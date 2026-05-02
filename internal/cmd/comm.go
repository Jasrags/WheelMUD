package cmd

import (
	"strings"

	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// commMaxLen caps inbound chat payloads to keep one player from
// flooding everyone else's scrollback. 1024 covers any reasonable
// roleplay sentence; longer strings get truncated with an ellipsis.
const commMaxLen = 1024

// NewSay builds the room-scoped chat command. Strips control bytes,
// caps length, and broadcasts to every other session whose
// CurrentRoomID matches the speaker's room.
func NewSay(sessions *session.Registry) *telnet.Command {
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
				_ = peer.WriteString(otherMsg)
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
			peer.LastTellFrom = speaker
			_ = peer.WriteString("{{" + speaker + " tells you,}}::magenta \"{{" + text + "}}::white\"\r\n")
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
			to := c.Session.LastTellFrom
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
			peer.LastTellFrom = speaker
			_ = peer.WriteString("{{" + speaker + " tells you,}}::magenta \"{{" + text + "}}::white\"\r\n")
			return c.Session.WriteString("{{You tell " + peer.CharacterName + ",}}::magenta \"{{" + text + "}}::white\"\r\n")
		},
	}
}

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
