package cmd

import (
	"context"
	"errors"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewLook builds the look command. With no args it renders the current
// room (description, exits, items, mobs). With a noun it resolves the
// noun against the room's ExtraDescs map (e.g. `look fountain`); if no
// keyword matches, the player is told there's nothing special to see.
// Mob/item inspection is delegated to the `examine` command so each
// verb has a single concern.
//
// clock drives the §9 day/night cycle and must be non-nil. Tests that
// don't care about lighting can pass a frozen-noon clock built via
// world.NewClock(675, world.WithNow(...)).
func NewLook(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, clock *world.Clock) *telnet.Command {
	return &telnet.Command{
		Name:    "look",
		Aliases: []string{"l"},
		Help:    "Look at your surroundings; `look <keyword>` for room details",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			if len(c.Args) == 0 {
				return RenderRoom(c.Ctx, c.Session, rooms, exits, items, mobs, clock)
			}
			return lookKeyword(c, rooms, strings.Join(c.Args, " "), clock)
		},
	}
}

// lookKeyword renders an entry from the room's ExtraDescs map. The
// noun is matched case-insensitively after trimming. Pitch-black rooms
// suppress the lookup so dark-room ambience stays consistent with the
// no-arg path.
func lookKeyword(c *telnet.Context, rooms repo.RoomRepo, noun string, clock *world.Clock) error {
	noun = strings.ToLower(strings.TrimSpace(noun))
	if noun == "" {
		return RenderRoom(c.Ctx, c.Session, rooms, nil, nil, nil, clock)
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
	if pitchBlack(room, clock) {
		return c.Session.WriteString("{{It is pitch black — you can't see a thing.}}::gray\r\n")
	}
	if desc, ok := room.ExtraDescs[noun]; ok && desc != "" {
		return c.Session.WriteString(toCRLF(desc) + "\r\n")
	}
	return c.Session.WriteString("{{You see nothing special.}}::gray\r\n")
}

// pitchBlack centralizes the "render nothing" gate so both the no-arg
// and `look <noun>` paths agree. The clock is required — production
// always wires a real one; tests pass a frozen-noon clock when they
// don't care about the cycle.
func pitchBlack(room repo.Room, clock *world.Clock) bool {
	return clock.EffectiveLight(room) <= 0
}

// RenderRoom produces the standard "you are here" view for the session's
// CurrentRoomID. Move commands call this after a successful move so the
// player sees the new room without typing `look`. Errors writing to the
// session bubble up; missing-data errors render as a graceful message.
//
// Output uses cfmt {{...}}::style tags via Session.WriteString — never
// pass untrusted input through this path. World text comes from the
// YAML loader, which is operator-controlled, so it's safe.
//
// clock must be non-nil; tests pass a frozen-noon clock when lighting
// isn't under test.
func RenderRoom(ctx context.Context, s *telnet.Session, rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, clock *world.Clock) error {
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

	// Pitch-black: render only the suppressed message. Items/mobs/desc
	// stay hidden until something lights the room (torch in inventory,
	// daylight curve, weave). Day/night cycle drives this when a clock
	// is wired; without a clock the legacy Dark+0 rule applies.
	if pitchBlack(room, clock) {
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
	// Defang interpolated names so a stray `}}::` in a builder-authored
	// name can't recolor everything below; LongDesc stays verbatim
	// because builders may intentionally cfmt-color description prose.
	b.WriteString("{{")
	b.WriteString(defangWorldField(room.Name))
	b.WriteString("}}::cyan|bold\r\n")
	if room.LongDesc != "" {
		// Description stays uncoloured so the room title and the
		// labelled lists below it pop. Just normalize line endings.
		b.WriteString(toCRLF(room.LongDesc))
		b.WriteString("\r\n")
	}
	visible := visibleExits(exitsList)
	if len(visible) > 0 {
		b.WriteString("{{Exits:}}::yellow|bold ")
		for i, e := range visible {
			if i > 0 {
				b.WriteString(", ")
			}
			name := repo.DirLong(e.Direction)
			// Closed (and especially locked) doors are dimmed and
			// annotated so a player can tell what blocks them at a
			// glance instead of bumping into the door on `north`.
			switch {
			case e.Flags.Locked:
				b.WriteString("{{")
				b.WriteString(name)
				b.WriteString(" (locked)}}::gray")
			case e.Flags.Closed:
				b.WriteString("{{")
				b.WriteString(name)
				b.WriteString(" (closed)}}::gray")
			default:
				b.WriteString("{{")
				b.WriteString(name)
				b.WriteString("}}::yellow")
			}
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
			b.WriteString(defangWorldField(it.Name))
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
			b.WriteString(defangWorldField(m.Core.Name))
			b.WriteString("}}::magenta")
		}
		b.WriteString("\r\n")
	}
	return s.WriteString(b.String())
}

// visibleExits drops Hidden exits from the list so look never reveals
// secret passages. Move treats Hidden the same as a missing exit.
func visibleExits(all []repo.Exit) []repo.Exit {
	out := make([]repo.Exit, 0, len(all))
	for _, e := range all {
		if e.Flags.Hidden {
			continue
		}
		out = append(out, e)
	}
	return out
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
