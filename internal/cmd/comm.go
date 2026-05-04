package cmd

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// commMaxLen caps inbound chat payloads to keep one player from
// flooding everyone else's scrollback. 1024 covers any reasonable
// roleplay sentence; longer strings get truncated with an ellipsis.
const commMaxLen = 1024

// NewSay builds the room-scoped chat command. Strips control bytes,
// caps length, and broadcasts to every other session whose
// CurrentRoomID matches the speaker's room. Rooms with the `silent`
// flag swallow speech with a flavor message.
func NewSay(sessions *session.Registry, rooms repo.RoomRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "say",
		Aliases: []string{"'"}, // most MUDs alias the apostrophe to say
		Help:    "Speak to everyone in this room",
		MinArgs: 1,
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			text, ok := sanitizeChat(c.Raw)
			if !ok {
				return c.Session.WriteString("{{Say what?}}::yellow\r\n")
			}
			if c.Session.CurrentRoomID == 0 {
				return c.Session.WriteString("{{You are nowhere — no one will hear you.}}::yellow\r\n")
			}
			// rooms is guaranteed non-nil in production (buildRegistry).
			// A FindByID miss (ErrRoomNotFound) is a soft-deleted-room
			// race and is allowed to broadcast — the speech still
			// reaches peers in the same logical CurrentRoomID.
			if room, err := rooms.FindByID(c.Ctx, c.Session.CurrentRoomID); err == nil {
				if room.Flags.Silent {
					return c.Session.WriteString("{{The air smothers your words; nothing carries.}}::yellow\r\n")
				}
			} else if !errors.Is(err, repo.ErrRoomNotFound) {
				slog.Debug("say: room lookup failed", "room", c.Session.CurrentRoomID, "error", err)
			}
			speaker := c.Session.CharacterName
			if speaker == "" {
				speaker = "Someone"
			}
			selfMsg := "{{You say,}}::cyan \"{{" + text + "}}::white\"\r\n"
			otherMsg := "{{" + speaker + " says,}}::cyan \"{{" + text + "}}::white\"\r\n"
			for _, peer := range sessions.Snapshot() {
				if peer == c.Session {
					continue
				}
				if peer.CurrentRoomID != c.Session.CurrentRoomID {
					continue
				}
				if err := peer.WriteString(otherMsg); err != nil {
					slog.Debug("comm: peer write failed", "to", peer.CharacterName, "error", err)
				}
			}
			return c.Session.WriteString(selfMsg)
		},
	}
}

// NewTell builds the global private-message command. Resolves the
// target by character name through session.Registry, writes one
// line to each side, and updates the recipient's LastTellFrom so
// `reply` knows where to write back.
func NewTell(sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:    "tell",
		Help:    "Tell <name> <text> — send a private message",
		MinArgs: 2,
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			name := c.Args[0]
			rest := strings.TrimSpace(strings.TrimPrefix(c.Raw, c.Args[0]))
			text, ok := sanitizeChat(rest)
			if !ok {
				return c.Session.WriteString("{{Tell them what?}}::yellow\r\n")
			}
			peer := sessions.FindByCharacterName(name)
			if peer == nil {
				return c.Session.WriteString("{{There is no one by that name.}}::yellow\r\n")
			}
			if peer == c.Session {
				return c.Session.WriteString("{{You mutter to yourself.}}::yellow\r\n")
			}
			speaker := c.Session.CharacterName
			if speaker == "" {
				speaker = "Someone"
			}
			peer.SetLastTellFrom(speaker)
			if err := peer.WriteString("{{" + speaker + " tells you,}}::magenta \"{{" + text + "}}::white\"\r\n"); err != nil {
				slog.Debug("comm: peer write failed", "to", peer.CharacterName, "error", err)
			}
			return c.Session.WriteString("{{You tell " + peer.CharacterName + ",}}::magenta \"{{" + text + "}}::white\"\r\n")
		},
	}
}

// NewReply builds the reply-to-last-tell command.
func NewReply(sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:    "reply",
		Help:    "Reply to the last `tell` you received",
		MinArgs: 1,
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			text, ok := sanitizeChat(c.Raw)
			if !ok {
				return c.Session.WriteString("{{Reply what?}}::yellow\r\n")
			}
			to := c.Session.LastTellFrom()
			if to == "" {
				return c.Session.WriteString("{{You have no one to reply to.}}::yellow\r\n")
			}
			peer := sessions.FindByCharacterName(to)
			if peer == nil {
				return c.Session.WriteString("{{" + to + " is no longer connected.}}::yellow\r\n")
			}
			speaker := c.Session.CharacterName
			if speaker == "" {
				speaker = "Someone"
			}
			peer.SetLastTellFrom(speaker)
			if err := peer.WriteString("{{" + speaker + " tells you,}}::magenta \"{{" + text + "}}::white\"\r\n"); err != nil {
				slog.Debug("comm: peer write failed", "to", peer.CharacterName, "error", err)
			}
			return c.Session.WriteString("{{You tell " + peer.CharacterName + ",}}::magenta \"{{" + text + "}}::white\"\r\n")
		},
	}
}

// worldFieldDefanger neutralizes the cfmt close-and-style sequence
// (`}}::`) inside name-shaped world strings spliced into colored
// templates (room names, item names, mob names, ExtraDescs keywords).
// Standalone `}}` and `::` are left alone so legitimate prose like
// "Lv:: 5" or "the inn's roof tiles" survives. Description-shaped
// fields (LongDesc, ExtraDescs values) are NOT defanged — builders
// can intentionally color them with cfmt.
//
// Same shape as game.go::cfmtDefanger; consolidating both copies into
// a shared internal/display package is tracked in
// world_aggregates_followups.md.
var worldFieldDefanger = strings.NewReplacer("{{", "{ {", "}}::", "} }::")

// defangWorldField scrubs cfmt injection from a name-shaped world
// string before it is spliced into a colored template.
func defangWorldField(s string) string { return worldFieldDefanger.Replace(s) }

// sanitizeChat strips control bytes and trims, caps length, and
// returns ok=false if the result is empty. cfmt template syntax
// (`{{ }} ::style`) is escaped so a player can't inject styling
// or close someone else's tag.
func sanitizeChat(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return "", false
	}
	if len(out) > commMaxLen {
		out = out[:commMaxLen-1] + "…"
	}
	// Defang cfmt by replacing `{{` and `}}` and `::` so a player's
	// text can never close the surrounding template tag.
	out = strings.ReplaceAll(out, "{{", "{ {")
	out = strings.ReplaceAll(out, "}}", "} }")
	out = strings.ReplaceAll(out, "::", ": :")
	return out, true
}
