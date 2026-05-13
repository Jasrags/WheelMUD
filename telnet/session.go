package telnet

import (
	"bytes"
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

	// In-world session state. CharacterID / CharacterName /
	// CurrentRoomID are crossMu-guarded: written by the mode-
	// promotion helper (postauth.promoteToGame) and by movement
	// commands (move, teleport, admin movement). Same-goroutine
	// dispatch readers can still read the exported fields directly
	// — Go memory model + alignment make those reads benign because
	// they execute on the same goroutine that does the write — but
	// **any goroutine other than this session's dispatcher** must use
	// `Session.InWorld()` to take the crossMu-guarded snapshot.
	// Writes go through `SetInWorld` (postauth promotion) or
	// `SetCurrentRoom` (movement).
	//
	// CharacterID / CharacterName are zero / empty pre-character
	// (login or select); CurrentRoomID is zero pre-game (login,
	// select, create). Documented zero-value semantics survive the
	// snapshot path: an InWorld() call on a pre-game session
	// returns (0, "", 0).
	CharacterID   int64
	CharacterName string
	CurrentRoomID int64
	// Speed is the mover's movement-mode capability block. Populated
	// in postauth.promoteToGame from the loaded character's Core.Speed
	// and read by the move family for sector gating (FlyFt for air,
	// SwimFt for underwater). Refresh when an effect changes mode.
	Speed creature.Speed

	// lastNewsSeen mirrors characters.last_news_seen so the §18
	// `news` command can render unread markers without a per-render
	// repo round-trip. Stamped by promoteToGame at game entry and
	// bumped after a successful MarkNewsSeen. Dispatcher-owned —
	// read and written only on the dispatch path.
	lastNewsSeen time.Time

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
	// nextReady is the wall-clock at which this session's dispatcher
	// will accept a new lagged command. Zero = not lagged. Stamped on
	// success by Registry.dispatchOne for verbs with Command.Lag > 0;
	// read by the same dispatcher's gate before each segment. Lives
	// under crossMu so external (test) callers can ResetLag without a
	// race; the dispatcher itself only ever reads/writes from its own
	// goroutine, so the lock is conservative defense.
	nextReady time.Time
	// channelMuted holds the per-channel mute map keyed by lowercase
	// channel name; true = the player has the channel turned off and
	// should not receive broadcasts. Loaded from the character at game
	// promotion and written through to the repo on toggle. Read by
	// other dispatchers iterating Snapshot() during a broadcast.
	channelMuted map[string]bool
	// hidden is the wizinvis flag — when true, the session is filtered
	// out of `who` listings and tell-name completion for non-admin
	// viewers. Toggled by the §17 wizinvis command. Session-scoped:
	// not persisted across reconnect (intentional; no schema change).
	// Read in foreign goroutines (other dispatchers iterating
	// Snapshot()), so guarded by crossMu like channelMuted.
	hidden bool
	// followingID is the CharacterID of the player this session is
	// auto-following — set by the `follow` verb and consumed by the
	// move verb's chainFollowers helper. Zero when not following.
	// Cross-goroutine read (move-time iteration of Snapshot), so
	// guarded by crossMu.
	followingID int64
	// builderZones caches the set of zone IDs this session is
	// authorised to edit (Phase G #33). Loaded by postauth.promoteToGame
	// from builder_zones via BuilderZoneRepo.ListForCharacter, and
	// refreshed by the grant / revoke admin verbs when the target is
	// online. The OLC verbs (#34 redit / oedit / medit / zedit) read
	// this through IsBuilderFor as their permission gate. Nil = no
	// grants (the default for guest / pre-promote sessions). AuthAdmin
	// bypasses this check entirely — see cmd.CanEditZone.
	builderZones map[int64]struct{}

	// writeMu is the single serializer for everything visible on the
	// wire and for the line-edit state that drives async-write redraws.
	// It guards lastPrompt, Input.Buf, Input.Cursor, InPasswordMode,
	// and every Conn.Write. Read-goroutine keystroke handlers wrap
	// "mutate Input + echo" in EditAndWrite so a concurrent WriteAsync
	// cannot observe an already-mutated buffer and emit a redraw that
	// then races against the echo bytes (which would double-display
	// the just-typed character).
	writeMu sync.Mutex

	// lastPrompt caches the most recently emitted prompt bytes (cfmt
	// already resolved). WriteAsync replays them after async output so
	// the prompt isn't left half-overwritten by a mid-line broadcast.
	lastPrompt []byte

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

// LastNewsSeen returns the dispatcher-owned mirror of the player's
// last_news_seen watermark. Zero time means "never seen". Read and
// written only on the dispatch path; no lock needed.
func (s *Session) LastNewsSeen() time.Time { return s.lastNewsSeen }

// SetLastNewsSeen updates the dispatcher-owned watermark. Bumped by
// promoteToGame on game entry and by the `news` command after a
// successful MarkNewsSeen.
func (s *Session) SetLastNewsSeen(t time.Time) { s.lastNewsSeen = t }

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

// InWorld returns a crossMu-protected snapshot of the in-world
// session state. Foreign-goroutine readers (broadcast loops
// iterating session.Registry.Snapshot, the §10 phase-ambient ticker,
// etc.) MUST use this rather than reading the exported fields
// directly — direct reads from a non-dispatcher goroutine race
// against SetCurrentRoom / SetInWorld writes.
//
// Returned values are a frozen point-in-time copy: the player may
// have moved by the time the caller acts on the room id, but the
// triple is internally consistent (no torn read where charID and
// roomID disagree on the player's identity).
func (s *Session) InWorld() (charID int64, charName string, roomID int64) {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	return s.CharacterID, s.CharacterName, s.CurrentRoomID
}

// SetInWorld stamps the (CharacterID, CharacterName, CurrentRoomID)
// triple atomically under crossMu. Called by postauth.promoteToGame
// when a character is selected and entering the game world. The
// dispatcher goroutine still reads the exported fields directly;
// this setter exists so the write happens-before relation is
// established for any foreign reader that uses InWorld().
func (s *Session) SetInWorld(charID int64, charName string, roomID int64) {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	s.CharacterID = charID
	s.CharacterName = charName
	s.CurrentRoomID = roomID
}

// SetCurrentRoom updates the dispatcher's current room under
// crossMu. Called by the move family and teleport / admin movement.
// Use this rather than direct assignment so foreign-goroutine
// broadcast loops that read InWorld() observe the new room.
func (s *Session) SetCurrentRoom(roomID int64) {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	s.CurrentRoomID = roomID
}

// StampInput records the wall-clock at which this session's
// dispatcher received a command. Read by `who` in foreign
// goroutines via IdleSince.
func (s *Session) StampInput(t time.Time) {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	s.lastInputAt = t
}

// IsLagged reports whether the session is currently rate-limited by
// a prior Command.Lag stamp. remaining is zero when not lagged.
// Read by Registry.dispatchOne at segment entry; the dispatcher
// owns the only writer (StampLag) so the lock is defense in depth.
func (s *Session) IsLagged(now time.Time) (locked bool, remaining time.Duration) {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	if s.nextReady.IsZero() || !now.Before(s.nextReady) {
		return false, 0
	}
	return true, s.nextReady.Sub(now)
}

// StampLag extends nextReady by d. No-op when d <= 0. Only extends
// — a pending longer lag is not shortened by a subsequent shorter
// stamp (defensive: refuse-mode prevents stacking V1, but a future
// queue-mode promotion could reach this path).
func (s *Session) StampLag(d time.Duration) {
	if d <= 0 {
		return
	}
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	deadline := time.Now().Add(d)
	if deadline.After(s.nextReady) {
		s.nextReady = deadline
	}
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

// SetBuilderZones replaces this session's cached set of editable zone
// IDs. promoteToGame stamps it at login from builder_zones; the
// grant/revoke admin verbs refresh it on an online target.
//
// The caller's map is copied so post-call mutations cannot race the
// reader. Passing nil clears the set.
func (s *Session) SetBuilderZones(grants map[int64]struct{}) {
	var cp map[int64]struct{}
	if len(grants) > 0 {
		cp = make(map[int64]struct{}, len(grants))
		for k := range grants {
			cp[k] = struct{}{}
		}
	}
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	s.builderZones = cp
}

// IsBuilderFor reports whether this session may edit the given zone
// per its cached builder grants. False for a nil / empty grant set —
// AuthAdmin bypasses this check at the cmd.CanEditZone layer.
func (s *Session) IsBuilderFor(zoneID int64) bool {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	if s.builderZones == nil {
		return false
	}
	_, ok := s.builderZones[zoneID]
	return ok
}

// BuilderZonesSnapshot returns a copy of the cached grant set. Mirrors
// ChannelMutedSnapshot; suitable for the `grants` admin viewer and
// tests. Nil when no grants are cached.
func (s *Session) BuilderZonesSnapshot() map[int64]struct{} {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	if len(s.builderZones) == 0 {
		return nil
	}
	cp := make(map[int64]struct{}, len(s.builderZones))
	for k := range s.builderZones {
		cp[k] = struct{}{}
	}
	return cp
}

// SetHidden sets the wizinvis bit on this session. Safe from any
// goroutine; readers in `who` and tell-completion observe the new
// value on their next call.
func (s *Session) SetHidden(v bool) {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	s.hidden = v
}

// IsHidden reports whether the session is currently wizinvis. Safe
// from any goroutine.
func (s *Session) IsHidden() bool {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	return s.hidden
}

// ToggleHidden flips the wizinvis bit and returns the new value
// (true = now hidden). The wizinvis command uses this to render the
// "fade / return" feedback line.
func (s *Session) ToggleHidden() bool {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	s.hidden = !s.hidden
	return s.hidden
}

// Following returns the CharacterID of the player this session is
// auto-following, or 0 when not following.
func (s *Session) Following() int64 {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	return s.followingID
}

// SetFollowing replaces the auto-follow target. Pass 0 to stop.
func (s *Session) SetFollowing(id int64) {
	s.crossMu.Lock()
	defer s.crossMu.Unlock()
	s.followingID = id
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
	rendered := []byte(cfmt.Sprint(text))
	if s.ColorLevel == ColorLevelNone {
		rendered = StripANSI(rendered)
	}
	return s.WriteRaw(rendered)
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
	out := []byte(wrapped)
	if s.ColorLevel == ColorLevelNone {
		out = StripANSI(out)
	}
	return s.WriteRaw(out)
}

// WritePaged writes body, pushing a pager mode when body would
// overflow Session.Height. Body is taken verbatim — callers are
// responsible for cfmt rendering / wrapping. Pass already-rendered
// bytes (most callers come from a strings.Builder).
//
// Pagination is skipped when:
//   - Height is non-positive (NAWS never negotiated and default
//     cleared, or a test set Height=0 to opt out), or
//   - the body fits in Height-1 lines (one line reserved for the
//     `--More--` prompt).
//
// In both fall-through cases, the body is emitted with a single
// WriteRaw and no mode is pushed.
func (s *Session) WritePaged(body []byte) error {
	if s.Height <= 0 {
		return s.WriteRaw(body)
	}
	lines := splitCRLFLines(body)
	if len(lines) < s.Height {
		return s.WriteRaw(body)
	}
	return s.PushMode(newPagerMode(lines, s.Height))
}

// WritePagedWrapped is the cfmt+reflow companion to WritePaged. It
// renders cfmt tags, reflows to Session.Width, normalizes line
// endings to CRLF, and then hands the result to WritePaged. Mirrors
// WriteWrapped's preprocessing so callers can swap one for the
// other.
func (s *Session) WritePagedWrapped(text string) error {
	if s.Width <= 0 {
		// No width to wrap to — fall back to the cfmt-only path so
		// the caller still gets pagination.
		rendered := []byte(cfmt.Sprint(text))
		if s.ColorLevel == ColorLevelNone {
			rendered = StripANSI(rendered)
		}
		return s.WritePaged(rendered)
	}
	rendered := cfmt.Sprint(text)
	wrapped := WrapText(rendered, s.Width)
	wrapped = strings.ReplaceAll(wrapped, "\n", "\r\n")
	out := []byte(wrapped)
	if s.ColorLevel == ColorLevelNone {
		out = StripANSI(out)
	}
	return s.WritePaged(out)
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

// WritePrompt is the dispatcher's prompt-write path: it caches the
// rendered bytes for later WriteAsync replay and emits them, all under
// writeMu so a concurrent WriteAsync sees the new prompt or the old
// one but never half of each. Callers should pass cfmt-resolved bytes.
func (s *Session) WritePrompt(p []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.lastPrompt = append(s.lastPrompt[:0], p...)
	if len(p) == 0 {
		return nil
	}
	if _, err := s.Conn.Write(p); err != nil {
		return fmt.Errorf("session prompt write: %w", err)
	}
	return nil
}

// writeBareEnter is the bare-Enter redraw: emit CRLF, repaint the
// prompt, and update the cache — all under writeMu so a concurrent
// WriteAsync sees either the pre-Enter state or the post-Enter state,
// never an in-between split write.
func (s *Session) writeBareEnter(prompt []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	out := make([]byte, 0, 2+len(prompt))
	out = append(out, '\r', '\n')
	out = append(out, prompt...)
	s.lastPrompt = append(s.lastPrompt[:0], prompt...)
	if _, err := s.Conn.Write(out); err != nil {
		return fmt.Errorf("session bare-enter write: %w", err)
	}
	return nil
}

// CacheLastPrompt updates the cached prompt without writing it. Used
// by paths that emitted the prompt as part of a larger payload (tab
// completion's listing+prompt+buffer redraw) so subsequent WriteAsync
// calls still replay the correct bytes.
func (s *Session) CacheLastPrompt(p []byte) {
	s.writeMu.Lock()
	s.lastPrompt = append(s.lastPrompt[:0], p...)
	s.writeMu.Unlock()
}

// ClearLastPrompt drops the cached prompt so the next async write does
// not replay stale bytes from a prior mode. Called by mode-stack
// transitions; the next dispatcher cycle paints a fresh prompt.
func (s *Session) ClearLastPrompt() {
	s.writeMu.Lock()
	s.lastPrompt = s.lastPrompt[:0]
	s.writeMu.Unlock()
}

// EditAndWrite runs fn under writeMu and emits any bytes fn returns
// to the wire — all atomically against concurrent WriteAsync /
// WritePrompt callers. fn may mutate Input.Buf / Input.Cursor /
// InPasswordMode; the returned bytes are the echo the terminal needs.
// Returning nil/empty is fine; nothing is written.
//
// This is the keystroke-handler entry point: read goroutine paths
// that mutate Input use it instead of taking writeMu manually so the
// "decide-what-to-emit, mutate, emit" cycle is one critical section.
// Without this, a WriteAsync between mutation and echo would observe
// the new buffer state and repaint with the just-typed byte already
// in it, then the deferred echo would print the byte a second time.
func (s *Session) EditAndWrite(fn func() []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	out := fn()
	if len(out) == 0 {
		return nil
	}
	if _, err := s.Conn.Write(out); err != nil {
		return fmt.Errorf("session edit write: %w", err)
	}
	return nil
}

// SetPasswordMode flips InPasswordMode under writeMu so a concurrent
// WriteAsync's snapshot observes a consistent value.
func (s *Session) SetPasswordMode(on bool) {
	s.writeMu.Lock()
	s.InPasswordMode = on
	s.writeMu.Unlock()
}

// snapshotInputLocked copies Input.Buf and reads InPasswordMode. The
// caller must hold writeMu.
func (s *Session) snapshotInputLocked() (buf []byte, masked bool) {
	if n := len(s.Input.Buf); n > 0 {
		buf = make([]byte, n)
		copy(buf, s.Input.Buf)
	}
	return buf, s.InPasswordMode
}

// WriteAsync renders cfmt tags on text and emits it sandwiched between
// "erase the prompt line" and "repaint the prompt + in-progress
// input." Use it from any goroutine that is NOT the recipient session's
// dispatcher — broadcast helpers, tick subscribers, channel fanout,
// phase-ambient writers, mob arrival/departure broadcasts. Synchronous
// command output should keep using WriteString / WriteWrapped /
// WriteRaw because the dispatcher repaints the prompt right after
// Mode.Handle returns.
//
// Layout written in a single Conn.Write so the redraw is atomic:
//
//	[CR + erase-to-EOL]  text  CRLF  cached-prompt  echo-of-input
//
// In password mode the input echo is N asterisks; on no-color
// terminals (ColorLevelNone) the erase falls back to CR + Width
// spaces + CR.
func (s *Session) WriteAsync(text string) error {
	rendered := []byte(cfmt.Sprint(text))
	if s.ColorLevel == ColorLevelNone {
		rendered = StripANSI(rendered)
	}
	if !bytes.HasSuffix(rendered, []byte("\r\n")) {
		rendered = append(rendered, '\r', '\n')
	}

	var prefix []byte
	if s.ColorLevel == ColorLevelNone {
		// No-ANSI fallback: blank the line with spaces, then return to
		// column 0. Width is the negotiated terminal width or 80.
		w := s.Width
		if w <= 0 {
			w = 80
		}
		prefix = make([]byte, 0, 2+w)
		prefix = append(prefix, '\r')
		prefix = append(prefix, bytes.Repeat([]byte(" "), w)...)
		prefix = append(prefix, '\r')
	} else {
		prefix = []byte("\r\x1b[K")
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	inputBuf, masked := s.snapshotInputLocked()
	var inputEcho []byte
	switch {
	case len(inputBuf) == 0:
		inputEcho = nil
	case masked:
		inputEcho = bytes.Repeat([]byte("*"), len(inputBuf))
	default:
		inputEcho = inputBuf
	}
	out := make([]byte, 0, len(prefix)+len(rendered)+len(s.lastPrompt)+len(inputEcho))
	out = append(out, prefix...)
	out = append(out, rendered...)
	out = append(out, s.lastPrompt...)
	out = append(out, inputEcho...)
	if _, err := s.Conn.Write(out); err != nil {
		return fmt.Errorf("session async write: %w", err)
	}
	return nil
}

// PushMode adds m to the top of the mode stack and calls m.OnEnter. If
// OnEnter returns an error the push is rolled back so the stack stays
// consistent (the caller treats a failed push as "not on the stack").
//
// Partial-write caveat: if OnEnter wrote bytes to the conn before
// erroring (e.g. the pager's first page partially landed), those bytes
// are already on the wire — the rollback only unwinds the stack, not
// the I/O. Callers that care must surface the error to the player.
//
// The cached prompt is dropped on every transition: the new mode's
// Prompt() is called next dispatcher tick and refreshes the cache;
// during the gap an async write would otherwise replay a prompt from
// the previous mode (login banner bleeding into game, etc.).
func (s *Session) PushMode(m Mode) error {
	if m == nil {
		return errors.New("telnet: PushMode(nil)")
	}
	s.modeMu.Lock()
	s.modes = append(s.modes, m)
	s.modeMu.Unlock()
	s.ClearLastPrompt()
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
	s.ClearLastPrompt()
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
