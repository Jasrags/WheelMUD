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
//
// Output uses cfmt {{...}}::style tags via Session.WriteString — never
// pass untrusted input through this path. World text comes from the
// YAML loader, which is operator-controlled, so it's safe.
func RenderRoom(ctx context.Context, s *telnet.Session, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobRepo) error {
	if s.CurrentRoomID == 0 {
		return s.WriteString("{{You are nowhere in particular.}}::red\r\n")
	}

	room, err := rooms.FindByID(ctx, s.CurrentRoomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return s.WriteString("{{The room around you has gone missing. Tell an admin.}}::red\r\n")
		}
		return s.WriteString("{{Could not look around right now.}}::red\r\n")
	}

	exitsList, err := exits.ListFrom(ctx, room.ID)
	if err != nil {
		return s.WriteString("{{Could not look around right now.}}::red\r\n")
	}
	itemsList, err := items.ListInRoom(ctx, room.ID)
	if err != nil {
		return s.WriteString("{{Could not look around right now.}}::red\r\n")
	}
	mobsList, err := mobs.ListInRoom(ctx, room.ID)
	if err != nil {
		return s.WriteString("{{Could not look around right now.}}::red\r\n")
	}

	var b strings.Builder
	// Room title: bright + bold so it stands out as the section header.
	b.WriteString("{{")
	b.WriteString(room.Name)
	b.WriteString("}}::cyan|bold\r\n")
	if room.LongDesc != "" {
		// Description stays uncoloured so the room title and the
		// labelled lists below it pop. Just normalize line endings.
		b.WriteString(toCRLF(room.LongDesc))
		b.WriteString("\r\n")
	}
	if len(exitsList) > 0 {
		b.WriteString("{{Exits:}}::yellow|bold ")
		for i, e := range exitsList {
			if i > 0 {
				b.WriteString(", ")
			}
			name, ok := directionLongName[e.Direction]
			if !ok {
				name = e.Direction
			}
			b.WriteString("{{")
			b.WriteString(name)
			b.WriteString("}}::yellow")
		}
		b.WriteString("\r\n")
	} else {
		b.WriteString("{{Exits:}}::yellow|bold {{none}}::gray\r\n")
	}
	if len(itemsList) > 0 {
		b.WriteString("{{You see:}}::green|bold ")
		for i, it := range itemsList {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("{{")
			b.WriteString(it.Name)
			b.WriteString("}}::green")
		}
		b.WriteString("\r\n")
	}
	if len(mobsList) > 0 {
		b.WriteString("{{Also here:}}::magenta|bold ")
		for i, m := range mobsList {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("{{")
			b.WriteString(m.Name)
			b.WriteString("}}::magenta")
		}
		b.WriteString("\r\n")
	}
	return s.WriteString(b.String())
}

// toCRLF normalizes line breaks for the telnet wire. World data
// authored in YAML uses bare LF (or, on Windows authoring, CRLF); the
// telnet client expects CRLF, and a stray LF leaves the cursor on the
// next row at the previous column. Strips bare CRs first so a CRLF
// input doesn't become CRCRLF.
func toCRLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}
