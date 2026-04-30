package cmd

import (
	"context"
	"errors"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// directionLongName maps the single-byte direction codes stored on Exit
// to the words rendered in look output. Mirrored by lookupDirection in
// move.go.
var directionLongName = map[string]string{
	repo.DirNorth: "north",
	repo.DirSouth: "south",
	repo.DirEast:  "east",
	repo.DirWest:  "west",
	repo.DirUp:    "up",
	repo.DirDown:  "down",
}

// NewLook builds the look command. It reads the room, exits, items, and
// mobs anchored at Session.CurrentRoomID and renders them. Empty
// subsections are omitted entirely.
func NewLook(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "look",
		Aliases: []string{"l"},
		Help:    "Look at your surroundings",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			return RenderRoom(c.Ctx, c.Session, rooms, exits, items, mobs)
		},
	}
}

// RenderRoom produces the standard "you are here" view for the session's
// CurrentRoomID. Move commands call this after a successful move so the
// player sees the new room without typing `look`. Errors writing to the
// session bubble up; missing-data errors render as a graceful message.
func RenderRoom(ctx context.Context, s *telnet.Session, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobRepo) error {
	if s.CurrentRoomID == 0 {
		return s.WriteRaw([]byte("You are nowhere in particular.\r\n"))
	}

	room, err := rooms.FindByID(ctx, s.CurrentRoomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return s.WriteRaw([]byte("The room around you has gone missing. Tell an admin.\r\n"))
		}
		return s.WriteRaw([]byte("Could not look around right now.\r\n"))
	}

	exitsList, err := exits.ListFrom(ctx, room.ID)
	if err != nil {
		return s.WriteRaw([]byte("Could not look around right now.\r\n"))
	}
	itemsList, err := items.ListInRoom(ctx, room.ID)
	if err != nil {
		return s.WriteRaw([]byte("Could not look around right now.\r\n"))
	}
	mobsList, err := mobs.ListInRoom(ctx, room.ID)
	if err != nil {
		return s.WriteRaw([]byte("Could not look around right now.\r\n"))
	}

	var b strings.Builder
	b.WriteString(room.Name)
	b.WriteString("\r\n")
	if room.LongDesc != "" {
		b.WriteString(room.LongDesc)
		b.WriteString("\r\n")
	}
	if len(exitsList) > 0 {
		b.WriteString("Exits: ")
		for i, e := range exitsList {
			if i > 0 {
				b.WriteString(", ")
			}
			name, ok := directionLongName[e.Direction]
			if !ok {
				name = e.Direction
			}
			b.WriteString(name)
		}
		b.WriteString("\r\n")
	} else {
		b.WriteString("Exits: none\r\n")
	}
	if len(itemsList) > 0 {
		b.WriteString("You see: ")
		for i, it := range itemsList {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(it.Name)
		}
		b.WriteString("\r\n")
	}
	if len(mobsList) > 0 {
		b.WriteString("Also here: ")
		for i, m := range mobsList {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(m.Name)
		}
		b.WriteString("\r\n")
	}
	return s.WriteRaw([]byte(b.String()))
}
