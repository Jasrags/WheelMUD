package cmd

import (
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewBind returns the `bind` command — anchor the character's death
// respawn point to the room they are standing in. The room must carry
// the `bindable` flag (inns, shrines, etc.); otherwise the verb
// refuses without mutating any state.
//
// Phase D #19 closer: until this verb existed, every player respawned
// at repo.StarterRoomID for the life of the character. The death
// pipeline in combat.handleCharacterDeath already reads
// Character.BoundRoomID; this verb is the only producer.
func NewBind(characters repo.CharacterRepo, rooms repo.RoomRepo) *telnet.Command {
	return &telnet.Command{
		Name: "bind",
		Help: "Anchor your respawn point to this room (requires a bindable room)",
		Long: "Usage: bind\r\n" +
			"       Sets your respawn point to the room you are standing\r\n" +
			"       in. Only rooms flagged `bindable` (inns, shrines,\r\n" +
			"       waystones) accept a binding; everywhere else, the\r\n" +
			"       attempt fails harmlessly.\r\n",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CharacterID == 0 || s.CurrentRoomID == 0 {
				return s.WriteString("{{You must be in the world to bind.}}::yellow\r\n")
			}
			room, err := rooms.FindByID(c.Ctx, s.CurrentRoomID)
			if err != nil {
				slog.Warn("bind: room lookup failed", "char", s.CharacterID, "room", s.CurrentRoomID, "error", err)
				return s.WriteString("{{You cannot bind here.}}::red\r\n")
			}
			if !room.Flags.Bindable {
				return s.WriteString("{{You feel no connection to this place. Try an inn or shrine.}}::yellow\r\n")
			}
			ch, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Warn("bind: char lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{Could not load your character.}}::red\r\n")
			}
			if ch.BoundRoomID == s.CurrentRoomID {
				return s.WriteString("{{You are already bound to this place.}}::cyan\r\n")
			}
			if err := characters.RecordBoundRoom(c.Ctx, ch.ID, s.CurrentRoomID); err != nil {
				slog.Warn("bind: record failed", "char", ch.ID, "room", s.CurrentRoomID, "error", err)
				return s.WriteString("{{Could not save your binding.}}::red\r\n")
			}
			return s.WriteString("{{You bind your spirit to this place.}}::cyan\r\n")
		},
	}
}
