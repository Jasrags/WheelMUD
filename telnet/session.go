package telnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/i582/cfmt/cmd/cfmt"
)

// inboxCap bounds the number of unprocessed input lines per session.
// Legitimate input never queues this deep; backpressure trips on flooding.
const inboxCap = 16

// ErrInputFlooded is returned by RunSession when the dispatcher cannot keep
// up with incoming lines.
var ErrInputFlooded = errors.New("telnet: input flooded")

// ErrSessionEnded is returned by Mode.Handle implementations that have
// closed the connection or otherwise want the dispatcher to terminate
// without writing a prompt afterward (e.g., a `quit` command).
var ErrSessionEnded = errors.New("telnet: session ended")

type Session struct {
	Conn          net.Conn
	RemoteAddress string
	TerminalType  string
	Width         int
	Height        int
	// Input is the cursor-aware line model. Buf accumulates raw printable
	// bytes until a line terminator arrives; Cursor tracks the insertion
	// point for in-line editing. It is owned by the read goroutine inside
	// RunSession and must not be mutated from anywhere else; reading from
	// another goroutine is also unsafe. The buffer retains its high-water
	// allocation for the session lifetime — acceptable at MUD scale.
	Input LineEdit
	// History is the per-session command-history ring used by ↑/↓
	// navigation. Entries that arrive in InPasswordMode are skipped.
	// Same goroutine-affinity rules as Input.
	History        History
	InPasswordMode bool
	ColorLevel     int
	// AuthLevel is the privilege the session has earned. Defaults to
	// AuthGuest until login mode promotes it. Registry.Dispatch checks
	// this against Command.Auth.
	AuthLevel AuthLevel

	// AccountID is the authenticated account, set by login mode after a
	// successful auth. Zero means "not authenticated." Used for the
	// session registry / multi-session policy.
	AccountID int64

	// Aliases holds the user-defined alias table consulted by
	// Registry.Dispatch before verb lookup. May be nil; expandAlias
	// tolerates that. Persistence across reconnects is deferred —
	// see ROADMAP §4 ("User-defined aliases stored on the character").
	Aliases *AliasTable

	// In-world session state. CharacterID, CharacterName, and
	// CurrentRoomID are owned by the dispatcher goroutine: written by
	// the mode-promotion helpers (postauth.promoteToGame) and by
	// movement commands during dispatch, read only by other commands
	// running on the same dispatch path. Code outside the dispatcher
	// (e.g. session.Registry consumers, future `who` implementations)
	// must treat these as snapshot values that can change underfoot;
	// add explicit synchronization here if/when a non-dispatcher reader
	// lands.
	//
	// CharacterID / CharacterName are zero / empty pre-character (login
	// or select); CurrentRoomID is zero pre-game (login, select, create).
	CharacterID   int64
	CharacterName string
	CurrentRoomID int64
	// Speed is the mover's movement-mode capability block. Populated
	// in postauth.promoteToGame from the loaded character's Core.Speed
	// and read by the move family for sector gating (FlyFt for air,
	// SwimFt for underwater). Refresh when an effect changes mode.
	Speed creature.Speed

	// crossMu guards the few session fields that are written by one
	// goroutine and read by another: lastTellFrom (set by senders'
	// dispatchers, read by this session's reply handler) and
	// lastInputAt (stamped by this session's dispatcher, read by
	// the `who` command running in any other dispatcher). Use the
	// SetLastTellFrom / LastTellFrom / StampInput / IdleSince
	// helpers; the fields themselves are unexported so callers
	// can't bypass the lock.
	crossMu      sync.Mutex
	lastTellFrom string
	lastInputAt  time.Time
	// channelMuted holds the per-channel mute map keyed by lowercase
	// channel name; true = the player has the channel turned off and
	// should not receive broadcasts. Loaded from the character at game
	// promotion and written through to the repo on toggle. Read by
	// other dispatchers iterating Snapshot() during a broadcast.
	channelMuted map[string]bool

	writeMu sync.Mutex

	modeMu sync.Mutex
	modes  []Mode

	inbox chan string

	// ctx is the per-session context, canceled when the read loop
	// returns (EOF / idle / flood). Set once by RunSession via
	// SetContext and read via Context() so prompt and helper paths
	// can honor cancellation without threading ctx through every
	// byte-handler signature. atomic.Pointer guards against the
	// race that would arise if a future caller read from a goroutine
	// that doesn't have a happens-before edge to the RunSession write
	// — per CLAUDE.md, cross-goroutine session fields must not be
	// touched as plain Go values.
	ctx atomic.Pointer[context.Context]
}

// SetContext stores the per-session context. RunSession calls this
// once during setup; tests that construct Session directly may call
// it to install a cancelable ctx. Subsequent reads via Context()
// observe the stored value.
func (s *Session) SetContext(ctx context.Context) {
	s.ctx.Store(&ctx)
}

// Context returns the per-session context. It is canceled when the
// read loop exits. Returns context.Background() if SetContext has
// not been called (test fixtures that construct Session directly).
func (s *Session) Context() context.Context {
	if p := s.ctx.Load(); p != nil {
		return *p
	}
	return context.Background()
}

// SetLastTellFrom records the name of the most recent `tell` sender
// so a follow-up `reply` can route back. Safe to call from any
// goroutine (the calling dispatcher writes the recipient's session).
func (s *Session) SetLastTellFrom(name string) {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	s.lastTellFrom = name
}

// LastTellFrom returns the name set by the most recent SetLastTellFrom,
// or the empty string. Safe from any goroutine.
func (s *Session) LastTellFrom() string {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	return s.lastTellFrom
}

// StampInput records the wall-clock at which this session's
// dispatcher received a command. Read by `who` in foreign
// goroutines via IdleSince.
func (s *Session) StampInput(t time.Time) {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	s.lastInputAt = t
}

// SetChannelMuted replaces the mute map. Pass nil to clear. The
// caller's map is copied so subsequent caller mutation doesn't race
// with broadcast readers.
func (s *Session) SetChannelMuted(m map[string]bool) {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	if len(m) == 0 {
		s.channelMuted = nil
		return
	}
	cp := make(map[string]bool, len(m))
	for k, v := range m {
		cp[k] = v
	}
	s.channelMuted = cp
}

// IsChannelMuted reports whether the named channel is muted on this
// session. Lookup is case-insensitive on the channel name; callers
// should already pass a normalized lowercase name.
func (s *Session) IsChannelMuted(name string) bool {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	return s.channelMuted[name]
}

// ToggleChannelMuted flips the mute bit for the named channel and
// returns the new value (true = now muted). The lazy allocation of
// the map keeps default-everything sessions cheap.
func (s *Session) ToggleChannelMuted(name string) bool {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	if s.channelMuted == nil {
		s.channelMuted = make(map[string]bool)
	}
	if s.channelMuted[name] {
		delete(s.channelMuted, name)
		return false
	}
	s.channelMuted[name] = true
	return true
}

// ChannelMutedSnapshot returns a copy of the mute map suitable for
// persisting. May be nil when nothing is muted.
func (s *Session) ChannelMutedSnapshot() map[string]bool {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	if len(s.channelMuted) == 0 {
		return nil
	}
	cp := make(map[string]bool, len(s.channelMuted))
	for k, v := range s.channelMuted {
		cp[k] = v
	}
	return cp
}

// IdleSince returns now - LastInputAt, or zero when no command has
// been processed yet. Safe from any goroutine.
func (s *Session) IdleSince(now time.Time) time.Duration {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	if s.lastInputAt.IsZero() {
		return 0
	}
	return now.Sub(s.lastInputAt)
}

func NewSession(conn net.Conn) *Session {
	if conn == nil {
		return nil
	}

	return &Session{
		Conn:          conn,
		RemoteAddress: conn.RemoteAddr().String(),
		Input:         LineEdit{Buf: make([]byte, 0)},
		Aliases:       NewAliasTable(),
		Width:         80,
		Height:        24,
		ColorLevel:    ColorLevel16,
		inbox:         make(chan string, inboxCap),
	}
}

// WriteString renders cfmt tags on `text` and writes the result to the
// connection. The write is serialized so concurrent callers do not interleave
// bytes on the wire. Any I/O error is returned so the caller can tear down the
// session.
//
// Note: cfmt interprets `{{...}}::style` tokens, so callers MUST NOT pass
// untrusted input directly. Use WriteRaw for client-derived strings.
func (s *Session) WriteString(text string) error {
	rendered := cfmt.Sprint(text)
	return s.WriteRaw([]byte(rendered))
}

// WriteWrapped renders cfmt tags and reflows the result to the session's
// current width before writing. Output newlines are emitted as CRLF so
// telnet clients render them correctly. A width of 0 falls back to
// WriteString without reflowing.
func (s *Session) WriteWrapped(text string) error {
	if s.Width <= 0 {
		return s.WriteString(text)
	}
	rendered := cfmt.Sprint(text)
	wrapped := WrapText(rendered, s.Width)
	// WrapText emits LF-only line breaks; convert to CRLF for the wire.
	wrapped = strings.ReplaceAll(wrapped, "\n", "\r\n")
	return s.WriteRaw([]byte(wrapped))
}

// WriteRaw writes the bytes verbatim, with no template rendering.
func (s *Session) WriteRaw(b []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.Conn.Write(b); err != nil {
		return fmt.Errorf("session write: %w", err)
	}
	return nil
}

// PushMode adds m to the top of the mode stack and calls m.OnEnter. If
// OnEnter returns an error the push is rolled back so the stack stays
// consistent (the caller treats a failed push as "not on the stack").
func (s *Session) PushMode(m Mode) error {
	if m == nil {
		return errors.New("telnet: PushMode(nil)")
	}
	s.modeMu.Lock()
	s.modes = append(s.modes, m)
	s.modeMu.Unlock()
	if err := m.OnEnter(s); err != nil {
		s.modeMu.Lock()
		// Defensive: only trim if our mode is still on top — a concurrent
		// Pop could have already removed it.
		if n := len(s.modes); n > 0 && s.modes[n-1] == m {
			s.modes = s.modes[:n-1]
		}
		s.modeMu.Unlock()
		return err
	}
	return nil
}

// PopMode removes the top mode and calls its OnExit. Returns ErrNoMode if
// the stack is empty.
func (s *Session) PopMode() error {
	s.modeMu.Lock()
	if len(s.modes) == 0 {
		s.modeMu.Unlock()
		return ErrNoMode
	}
	top := s.modes[len(s.modes)-1]
	s.modes = s.modes[:len(s.modes)-1]
	s.modeMu.Unlock()
	return top.OnExit(s)
}

// ReplaceMode pops the current top mode (if any) and pushes m. OnExit is
// called for the popped mode and OnEnter for m.
func (s *Session) ReplaceMode(m Mode) error {
	if m == nil {
		return errors.New("telnet: ReplaceMode(nil)")
	}
	if err := s.PopMode(); err != nil && !errors.Is(err, ErrNoMode) {
		return err
	}
	return s.PushMode(m)
}

// CurrentMode returns the top of the mode stack, or nil if empty.
func (s *Session) CurrentMode() Mode {
	s.modeMu.Lock()
	defer s.modeMu.Unlock()
	if len(s.modes) == 0 {
		return nil
	}
	return s.modes[len(s.modes)-1]
}
