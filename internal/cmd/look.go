package cmd

import (
	"context"
	"errors"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// directionLongName maps the short direction codes stored on Exit
// (n/s/e/w/u/d/ne/nw/se/sw) to the words rendered in look output.
var directionLongName = map[string]string{
	repo.DirNorth:     "north",
	repo.DirSouth:     "south",
	repo.DirEast:      "east",
	repo.DirWest:      "west",
	repo.DirUp:        "up",
	repo.DirDown:      "down",
	repo.DirNortheast: "northeast",
	repo.DirNorthwest: "northwest",
	repo.DirSoutheast: "southeast",
	repo.DirSouthwest: "southwest",
}

// NewLook builds the look command. With no args it renders the current
// room (description, exits, items, mobs). With a noun it resolves the
// noun against the room's ExtraDescs map (e.g. `look fountain`); if no
// keyword matches, the player is told there's nothing special to see.
// Mob/item inspection is delegated to the `examine` command so each
// verb has a single concern.
func NewLook(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "look",
		Aliases: []string{"l"},
		Help:    "Look at your surroundings; `look <keyword>` for room details",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			if len(c.Args) == 0 {
				return RenderRoom(c.Ctx, c.Session, rooms, exits, items, mobs)
			}
			return lookKeyword(c, rooms, strings.Join(c.Args, " "))
		},
	}
}

// lookKeyword renders an entry from the room's ExtraDescs map. The
// noun is matched case-insensitively after trimming. Pitch-black rooms
// suppress the lookup so dark-room ambience stays consistent with the
// no-arg path.
func lookKeyword(c *telnet.Context, rooms repo.RoomRepo, noun string) error {
	noun = strings.ToLower(strings.TrimSpace(noun))
	if noun == "" {
		return RenderRoom(c.Ctx, c.Session, rooms, nil, nil, nil)
	}
	if c.Session.CurrentRoomID == 0 {
		return c.Session.WriteString("{{You are nowhere in particular.}}::red\r\n")
	}
	room, err := rooms.FindByID(c.Ctx, c.Session.CurrentRoomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return c.Session.WriteString("{{The room around you has gone missing. Tell an admin.}}::red\r\n")
		}
		return c.Session.WriteString("{{Could not look around right now.}}::red\r\n")
	}
	if room.Flags.Dark && room.LightLevel <= 0 {
		return c.Session.WriteString("{{It is pitch black — you can't see a thing.}}::gray\r\n")
	}
	if desc, ok := room.ExtraDescs[noun]; ok && desc != "" {
		return c.Session.WriteString(toCRLF(desc) + "\r\n")
	}
	return c.Session.WriteString("{{You see nothing special.}}::gray\r\n")
}

// RenderRoom produces the standard "you are here" view for the session's
// CurrentRoomID. Move commands call this after a successful move so the
// player sees the new room without typing `look`. Errors writing to the
// session bubble up; missing-data errors render as a graceful message.
//
// Output uses cfmt {{...}}::style tags via Session.WriteString — never
// pass untrusted input through this path. World text comes from the
// YAML loader, which is operator-controlled, so it's safe.
func RenderRoom(ctx context.Context, s *telnet.Session, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo) error {
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

	// Dark room with no light: render only the title-as-shadow plus
	// an exits hint. Items/mobs/desc stay hidden until something lights
	// the room (torch in inventory, daylight, weave). Builders who
	// want a "you see nothing" feel set dark=true and light_level=0.
	if room.Flags.Dark && room.LightLevel <= 0 {
		return s.WriteString("{{It is pitch black — you can't see a thing.}}::gray\r\n")
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
			b.WriteString(m.Core.Name)
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
