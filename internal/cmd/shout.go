package cmd

import (
	"errors"
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/visibility"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewShout builds the zone-scoped chat command. Reaches every session
// whose CurrentRoomID resolves to the same zones.id as the speaker.
// Distinct from `say` (room-only) and channel chatter (global, OOC).
func NewShout(sessions *session.Registry, rooms repo.RoomRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "shout",
		Help:    "shout <message> — broadcast to everyone in your zone",
		MinArgs: 1,
		Auth:    telnet.AuthPlayer,
		Lag:     2 * time.Second,
		Run: func(c *telnet.Context) error {
			return zoneBroadcast(c, sessions, rooms, "shout", "shouts", "yellow")
		},
	}
}

// NewYell builds the zone-scoped alarmed-broadcast command. Same fanout
// as `shout` but rendered in red so a yelp reads differently from a
// hail.
func NewYell(sessions *session.Registry, rooms repo.RoomRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "yell",
		Help:    "yell <message> — alarmed broadcast across your zone",
		MinArgs: 1,
		Auth:    telnet.AuthPlayer,
		Lag:     2 * time.Second,
		Run: func(c *telnet.Context) error {
			return zoneBroadcast(c, sessions, rooms, "yell", "yells", "red")
		},
	}
}

// zoneBroadcast is the shared fanout for shout/yell. selfVerb is the
// first-person verb ("shout"), thirdVerb is the third-person form
// ("shouts"), color is the cfmt color tag applied to the verb half.
func zoneBroadcast(c *telnet.Context, sessions *session.Registry, rooms repo.RoomRepo, selfVerb, thirdVerb, color string) error {
	text, ok := sanitizeChat(c.Raw)
	if !ok {
		return c.Session.WriteString("{{" + capFirst(selfVerb) + " what?}}::yellow\r\n")
	}
	if c.Session.CurrentRoomID == 0 {
		return c.Session.WriteString("{{You are nowhere — your voice goes nowhere.}}::yellow\r\n")
	}
	speakerRoom, err := rooms.FindByID(c.Ctx, c.Session.CurrentRoomID)
	if err != nil {
		if !errors.Is(err, repo.ErrRoomNotFound) {
			slog.Debug(selfVerb+": speaker room lookup failed", "room", c.Session.CurrentRoomID, "error", err)
		}
		return c.Session.WriteString("{{Your voice catches in the silence.}}::yellow\r\n")
	}
	if speakerRoom.ZoneID == 0 {
		return c.Session.WriteString("{{You are nowhere a " + selfVerb + " could carry.}}::yellow\r\n")
	}
	if speakerRoom.Flags.Silent {
		return c.Session.WriteString("{{The air smothers your voice; nothing carries.}}::yellow\r\n")
	}

	speaker := c.Session.CharacterName
	if speaker == "" {
		speaker = "Someone"
	}
	selfMsg := "{{You " + selfVerb + ",}}::" + color + " \"{{" + text + "}}::white\"\r\n"
	otherMsg := "{{" + speaker + " " + thirdVerb + ",}}::" + color + " \"{{" + text + "}}::white\"\r\n"

	for _, peer := range sessions.Snapshot() {
		if peer == c.Session {
			continue
		}
		peerCharID, peerName, peerRoomID := peer.InWorld()
		if peerCharID == 0 || peerRoomID == 0 {
			continue
		}
		peerRoom, err := rooms.FindByID(c.Ctx, peerRoomID)
		if err != nil {
			if !errors.Is(err, repo.ErrRoomNotFound) {
				slog.Debug(selfVerb+": peer room lookup failed", "to", peerName, "room", peerRoomID, "error", err)
			}
			continue
		}
		if peerRoom.ZoneID != speakerRoom.ZoneID {
			continue
		}
		// wizinvis: a hidden admin's shout/yell is silent to
		// non-admin peers. The speaker still sees their own self-line.
		if !visibility.CanSee(peer, c.Session) {
			continue
		}
		if err := peer.WriteAsync(otherMsg); err != nil {
			slog.Debug(selfVerb+": peer write failed", "to", peerName, "error", err)
		}
	}
	return c.Session.WriteString(selfMsg)
}

// capFirst uppercases the first ASCII byte. Used to title-case the
// "Shout what?" / "Yell what?" prompt without pulling in unicode.
func capFirst(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}
