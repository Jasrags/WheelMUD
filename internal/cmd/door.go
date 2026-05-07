package cmd

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// directionAliases maps every accepted spelling (long and short, mixed
// case) to its canonical short code. Door commands accept the same
// surface area as the move family so `open n` and `open north` agree.
var directionAliases = map[string]string{
	"n": repo.DirNorth, "north": repo.DirNorth,
	"s": repo.DirSouth, "south": repo.DirSouth,
	"e": repo.DirEast, "east": repo.DirEast,
	"w": repo.DirWest, "west": repo.DirWest,
	"u": repo.DirUp, "up": repo.DirUp,
	"d": repo.DirDown, "down": repo.DirDown,
	"ne": repo.DirNortheast, "northeast": repo.DirNortheast,
	"nw": repo.DirNorthwest, "northwest": repo.DirNorthwest,
	"se": repo.DirSoutheast, "southeast": repo.DirSoutheast,
	"sw": repo.DirSouthwest, "southwest": repo.DirSouthwest,
}

// resolveDir maps a player-supplied direction word to the short code
// the repo stores. Returns ok=false on unknown input.
func resolveDir(s string) (string, bool) {
	d, ok := directionAliases[strings.ToLower(strings.TrimSpace(s))]
	return d, ok
}

// dirLong is a tiny shim around repo.DirLong so the door verbs read
// naturally (`dirLong(dir)`); kept for consistency with the call
// sites below until they migrate too.
func dirLong(code string) string { return repo.DirLong(code) }

// completeExits returns a Completer that suggests the visible exits
// leaving the actor's current room. The first arg slot completes
// short-codes (n/ne/u/...); subsequent slots return nil so the bell
// fires instead. Hidden exits are filtered the same way resolveDoor
// hides them — confirming a passage by tab-completing it would defeat
// the Hidden flag's purpose.
func completeExits(exits repo.ExitRepo) func(s *telnet.Session, args string) []telnet.Candidate {
	return func(s *telnet.Session, args string) []telnet.Candidate {
		slot, partial := completerSlot(args)
		if slot != 0 || s.CurrentRoomID == 0 {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		list, err := exits.ListFrom(ctx, s.CurrentRoomID)
		if err != nil {
			return nil
		}
		partial = strings.ToLower(partial)
		out := make([]telnet.Candidate, 0, len(list))
		for _, e := range list {
			if e.Flags.Hidden {
				continue
			}
			if !strings.HasPrefix(e.Direction, partial) {
				continue
			}
			out = append(out, telnet.Candidate{Text: e.Direction, Help: dirLong(e.Direction)})
		}
		return out
	}
}

// playerHasKey reports whether the player can satisfy a key requirement
// for an exit. The key must be in the player's inventory (§14). AuthAdmin
// bypasses the check entirely. exit.KeyExternalID == "" means no key is
// configured: only Admin can lock/unlock such an exit at runtime.
func playerHasKey(ctx context.Context, s *telnet.Session, exit repo.Exit, items repo.ItemRepo) bool {
	if s.AuthLevel >= telnet.AuthAdmin {
		return true
	}
	if exit.KeyExternalID == "" {
		return false
	}
	if items == nil || s.CharacterID == 0 {
		return false
	}
	held, err := items.ListInInventory(ctx, s.CharacterID)
	if err != nil {
		slog.Debug("door: key lookup failed", "char", s.CharacterID, "error", err)
		return false
	}
	for _, it := range held {
		// Prefer the typed KeyStats.KeyID — that's the contract once
		// item taxonomy (§9) lands. Fall through to ExternalID match
		// so legacy items authored before migration 0015 still work.
		if it.Type == repo.ItemTypeKey {
			if ks, ok := it.Stats.(*repo.KeyStats); ok && ks.KeyID == exit.KeyExternalID {
				return true
			}
		}
		if it.ExternalID == exit.KeyExternalID {
			return true
		}
	}
	return false
}

// broadcastRoom sends msg to every other session in roomID. Failures
// are logged at debug — one peer's broken pipe must not stop the
// announcement from reaching the rest of the room.
func broadcastRoom(sessions *session.Registry, roomID int64, except *telnet.Session, msg string) {
	if sessions == nil || roomID == 0 {
		return
	}
	for _, peer := range sessions.Snapshot() {
		_, peerName, peerRoom := peer.InWorld()
		if peer == except || peerRoom != roomID {
			continue
		}
		if err := peer.WriteAsync(msg); err != nil {
			slog.Debug("door: peer write failed", "to", peerName, "error", err)
		}
	}
}

// safeActor returns the actor's character name scrubbed for safe
// inclusion in a cfmt-formatted broadcast (control bytes stripped,
// `{{` / `}}` / `::` neutralised). Delegates to display.Defang —
// see internal/display/defang.go. character_create already restricts
// names to `[A-Za-z0-9_-]`, so this is defensive against a future
// loosening of the name policy.
func safeActor(s *telnet.Session) string {
	if s == nil {
		return "Someone"
	}
	return display.Defang(s.CharacterName, "Someone")
}

// resolveDoor pulls the exit named by c.Args[0] from the session's
// current room. Returns the exit plus the canonical short code on
// success, or writes the appropriate refusal and returns ok=false.
//
// The verb is non-atomic: resolveDoor returns a snapshot of exit
// state, the caller's guard checks (Closed / Locked / NoPass) read
// that snapshot, and UpdateFlags later commits unconditionally. Two
// players issuing conflicting verbs in the same instant can interleave
// — the second write wins and neither command is rejected. For a MUD
// the contention model makes this acceptable: the worst case is a
// door appearing to flicker between states for one render cycle, and
// the next look/move re-reads canonical state.
func resolveDoor(c *telnet.Context, exits repo.ExitRepo) (repo.Exit, string, bool) {
	s := c.Session
	if s.CurrentRoomID == 0 {
		_ = s.WriteString("{{You are nowhere — there is no door here.}}::yellow\r\n")
		return repo.Exit{}, "", false
	}
	if len(c.Args) == 0 {
		_ = s.WriteString("{{Usage: " + c.Name + " <direction>}}::yellow\r\n")
		return repo.Exit{}, "", false
	}
	dir, ok := resolveDir(c.Args[0])
	if !ok {
		_ = s.WriteString("{{That isn't a direction.}}::yellow\r\n")
		return repo.Exit{}, "", false
	}
	exit, err := exits.FindByDirection(c.Ctx, s.CurrentRoomID, dir)
	if err != nil {
		if !errors.Is(err, repo.ErrExitNotFound) {
			slog.Warn("door: exit lookup failed", "char", s.CharacterID, "room", s.CurrentRoomID, "dir", dir, "error", err)
		}
		_ = s.WriteString("{{There is no door that way.}}::yellow\r\n")
		return repo.Exit{}, "", false
	}
	// Hidden exits stay invisible to door verbs too — the player
	// shouldn't be able to confirm a passage by trying to open it.
	if exit.Flags.Hidden {
		_ = s.WriteString("{{There is no door that way.}}::yellow\r\n")
		return repo.Exit{}, "", false
	}
	return exit, dir, true
}

// reverseDir returns the opposite cardinal/diagonal/vertical so the
// door state-change can be announced to the room on the far side.
// Returns "" when no canonical reverse exists.
func reverseDir(d string) string {
	switch d {
	case repo.DirNorth:
		return repo.DirSouth
	case repo.DirSouth:
		return repo.DirNorth
	case repo.DirEast:
		return repo.DirWest
	case repo.DirWest:
		return repo.DirEast
	case repo.DirUp:
		return repo.DirDown
	case repo.DirDown:
		return repo.DirUp
	case repo.DirNortheast:
		return repo.DirSouthwest
	case repo.DirSouthwest:
		return repo.DirNortheast
	case repo.DirNorthwest:
		return repo.DirSoutheast
	case repo.DirSoutheast:
		return repo.DirNorthwest
	}
	return ""
}

// announce broadcasts the verb to the player's current room and (when
// the far side has a matching reverse exit) to the destination room.
// nearMsg includes the direction the actor used; farMsg uses the
// reverse direction so the far room sees a consistent picture.
func announce(c *telnet.Context, exits repo.ExitRepo, sessions *session.Registry, exit repo.Exit, dir, nearMsg, farMsg string) {
	broadcastRoom(sessions, c.Session.CurrentRoomID, c.Session, nearMsg)
	rev := reverseDir(dir)
	if rev == "" || exit.ToRoomID == 0 {
		return
	}
	farExit, err := exits.FindByDirection(c.Ctx, exit.ToRoomID, rev)
	if err != nil {
		// Stale far-side exit (deleted or the zone was reloaded
		// between the verb's UpdateFlags and this lookup). Drop the
		// far broadcast — the actor's room already saw the change —
		// but leave a debug breadcrumb so the gap is traceable.
		if !errors.Is(err, repo.ErrExitNotFound) {
			slog.Debug("door: far-exit lookup failed", "to", exit.ToRoomID, "rev", rev, "error", err)
		}
		return
	}
	if farExit.Flags.Hidden {
		return
	}
	broadcastRoom(sessions, exit.ToRoomID, nil, farMsg)
}

// NewOpen builds the open command. Refuses on locked, NoPass, or
// already-open doors; otherwise clears Closed and broadcasts.
func NewOpen(exits repo.ExitRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:      "open",
		Help:      "open <direction> — open a closed door (e.g. open north)",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeExits(exits),
		Run: func(c *telnet.Context) error {
			exit, dir, ok := resolveDoor(c, exits)
			if !ok {
				return nil
			}
			s := c.Session
			if !exit.Flags.Closed {
				return s.WriteString("{{It is already open.}}::yellow\r\n")
			}
			if exit.Flags.Locked {
				return s.WriteString("{{It is locked.}}::yellow\r\n")
			}
			if err := exits.UpdateFlags(c.Ctx, exit.ID, false, false); err != nil {
				slog.Warn("open: update failed", "exit", exit.ID, "error", err)
				return s.WriteString("{{It refuses to budge right now.}}::red\r\n")
			}
			actor := safeActor(s)
			long := dirLong(dir)
			rev := dirLong(reverseDir(dir))
			announce(c, exits, sessions, exit, dir,
				"{{"+actor+" opens the "+long+" door.}}::cyan\r\n",
				"{{The "+rev+" door swings open.}}::cyan\r\n",
			)
			return s.WriteString("{{You open the " + long + " door.}}::cyan\r\n")
		},
	}
}

// NewClose builds the close command. Refuses on already-closed or
// NoPass exits; otherwise sets Closed and broadcasts.
func NewClose(exits repo.ExitRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:      "close",
		Help:      "close <direction> — close an open door (e.g. close north)",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeExits(exits),
		Run: func(c *telnet.Context) error {
			exit, dir, ok := resolveDoor(c, exits)
			if !ok {
				return nil
			}
			s := c.Session
			if exit.Flags.Closed {
				return s.WriteString("{{It is already closed.}}::yellow\r\n")
			}
			if exit.Flags.NoPass {
				return s.WriteString("{{There is no door there to close.}}::yellow\r\n")
			}
			if err := exits.UpdateFlags(c.Ctx, exit.ID, true, false); err != nil {
				slog.Warn("close: update failed", "exit", exit.ID, "error", err)
				return s.WriteString("{{It refuses to budge right now.}}::red\r\n")
			}
			actor := safeActor(s)
			long := dirLong(dir)
			rev := dirLong(reverseDir(dir))
			announce(c, exits, sessions, exit, dir,
				"{{"+actor+" closes the "+long+" door.}}::cyan\r\n",
				"{{The "+rev+" door swings shut.}}::cyan\r\n",
			)
			return s.WriteString("{{You close the " + long + " door.}}::cyan\r\n")
		},
	}
}

// NewLock builds the lock command. The exit must be Closed and the
// player must have the matching key (placeholder: in the same room
// until §14 inventory lands; AuthAdmin always bypasses).
func NewLock(exits repo.ExitRepo, items repo.ItemRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:      "lock",
		Help:      "lock <direction> — lock a closed door (requires the matching key)",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeExits(exits),
		Run: func(c *telnet.Context) error {
			exit, dir, ok := resolveDoor(c, exits)
			if !ok {
				return nil
			}
			s := c.Session
			if exit.Flags.Locked {
				return s.WriteString("{{It is already locked.}}::yellow\r\n")
			}
			if !exit.Flags.Closed {
				return s.WriteString("{{You must close it first.}}::yellow\r\n")
			}
			if !playerHasKey(c.Ctx, s, exit, items) {
				return s.WriteString("{{You don't have the key.}}::yellow\r\n")
			}
			if err := exits.UpdateFlags(c.Ctx, exit.ID, true, true); err != nil {
				slog.Warn("lock: update failed", "exit", exit.ID, "error", err)
				return s.WriteString("{{The lock won't turn right now.}}::red\r\n")
			}
			actor := safeActor(s)
			long := dirLong(dir)
			rev := dirLong(reverseDir(dir))
			announce(c, exits, sessions, exit, dir,
				"{{"+actor+" locks the "+long+" door.}}::cyan\r\n",
				"{{You hear the "+rev+" door lock.}}::cyan\r\n",
			)
			return s.WriteString("{{You lock the " + long + " door.}}::cyan\r\n")
		},
	}
}

// NewUnlock builds the unlock command. Mirrors lock: exit must be
// Locked and the player must have the matching key.
func NewUnlock(exits repo.ExitRepo, items repo.ItemRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:      "unlock",
		Help:      "unlock <direction> — unlock a locked door (requires the matching key)",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeExits(exits),
		Run: func(c *telnet.Context) error {
			exit, dir, ok := resolveDoor(c, exits)
			if !ok {
				return nil
			}
			s := c.Session
			if !exit.Flags.Locked {
				return s.WriteString("{{It is not locked.}}::yellow\r\n")
			}
			if !playerHasKey(c.Ctx, s, exit, items) {
				return s.WriteString("{{You don't have the key.}}::yellow\r\n")
			}
			// Unlocking leaves the door closed but unlocked; the
			// player can then `open` to pass through. Mirrors most
			// codebases and lets the lock state be observed.
			if err := exits.UpdateFlags(c.Ctx, exit.ID, true, false); err != nil {
				slog.Warn("unlock: update failed", "exit", exit.ID, "error", err)
				return s.WriteString("{{The lock won't turn right now.}}::red\r\n")
			}
			actor := safeActor(s)
			long := dirLong(dir)
			rev := dirLong(reverseDir(dir))
			announce(c, exits, sessions, exit, dir,
				"{{"+actor+" unlocks the "+long+" door.}}::cyan\r\n",
				"{{You hear the "+rev+" door unlock.}}::cyan\r\n",
			)
			return s.WriteString("{{You unlock the " + long + " door.}}::cyan\r\n")
		},
	}
}

// NewPick builds the pick command. The §12 skill check is not yet
// implemented, so the verb is gated to AuthAdmin: builders/admins can
// shake locks loose for testing, players get the standard "you lack
// the skill" refusal until lockpicking lands.
func NewPick(exits repo.ExitRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:      "pick",
		Help:      "pick <direction> — attempt to pick a locked door",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeExits(exits),
		Run: func(c *telnet.Context) error {
			exit, dir, ok := resolveDoor(c, exits)
			if !ok {
				return nil
			}
			s := c.Session
			if !exit.Flags.Locked {
				return s.WriteString("{{It is not locked.}}::yellow\r\n")
			}
			if !exit.Flags.Pickable {
				return s.WriteString("{{This lock cannot be picked.}}::yellow\r\n")
			}
			// §12 skill check is not landed yet. Until it is, only
			// AuthAdmin succeeds — players see the same flavor message
			// they will see on a failed roll once skills are wired in.
			if s.AuthLevel < telnet.AuthAdmin {
				return s.WriteString("{{You fumble with the lock but lack the skill.}}::yellow\r\n")
			}
			if err := exits.UpdateFlags(c.Ctx, exit.ID, true, false); err != nil {
				slog.Warn("pick: update failed", "exit", exit.ID, "error", err)
				return s.WriteString("{{The lock won't yield right now.}}::red\r\n")
			}
			actor := safeActor(s)
			long := dirLong(dir)
			rev := dirLong(reverseDir(dir))
			announce(c, exits, sessions, exit, dir,
				"{{"+actor+" picks the "+long+" lock.}}::cyan\r\n",
				"{{You hear the "+rev+" lock click open.}}::cyan\r\n",
			)
			return s.WriteString("{{The " + long + " lock yields to your tools.}}::cyan\r\n")
		},
	}
}
