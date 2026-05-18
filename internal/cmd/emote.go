package cmd

import (
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewEmote builds the freeform `emote <text>` verb. Unlike socials,
// emote has no targeted form — the player composes the entire action
// string. The verb is the classic MUD escape hatch for any action
// the catalog does not cover.
//
// Sanitization, length cap, and cfmt defang are inherited from
// sanitizeChat (same path as say). Visibility and silent-room rules
// match socials: wizinvis hides the actor from non-admin peers, the
// silent flag does NOT gag emotes (text-gag covers speech only).
func NewEmote(sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:    "emote",
		Aliases: []string{":"},
		Help:    "emote <text> — describe a freeform action",
		Long:    "emote <text>\nAlias: : (colon)\n\nDescribe a physical action in the third person. Example: `emote tilts his head, considering the question.` renders to peers as `Yourname tilts his head, ...`",
		MinArgs: 1,
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			text, ok := sanitizeChat(c.Raw)
			if !ok {
				return c.Session.WriteString("{{Emote what?}}::yellow\r\n")
			}
			actor := c.Session.CharacterName
			if actor == "" {
				actor = "Someone"
			}
			actor = defangSocialName(actor)
			selfMsg := "{{You " + text + "}}::magenta\r\n"
			otherMsg := "{{" + actor + " " + text + "}}::magenta\r\n"
			if c.Session.CurrentRoomID != 0 {
				broadcastSocial(sessions, c.Session, otherMsg)
			}
			return c.Session.WriteString(selfMsg)
		},
	}
}
