package cmd

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewWhereAmI builds the room-debug command. Reveals internal ids,
// every flag, and the full ExtraDescs keyword set, so it is gated at
// AuthAdmin — exposing this to players bypasses look's deliberate
// per-keyword reveal. The command is registered today but unreachable
// until an admin role lands; promote a session via teleport-style
// elevation when that mechanism exists.
func NewWhereAmI(rooms repo.RoomRepo) *telnet.Command {
	return &telnet.Command{
		Name: "whereami",
		Help: "Show the current room's id, sector, flags, light, and coords",
		Auth: telnet.AuthAdmin,
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CurrentRoomID == 0 {
				return s.WriteString("{{You are nowhere in particular.}}::red\r\n")
			}
			room, err := rooms.FindByID(c.Ctx, s.CurrentRoomID)
			if err != nil {
				if errors.Is(err, repo.ErrRoomNotFound) {
					return s.WriteString("{{The room around you has gone missing. Tell an admin.}}::red\r\n")
				}
				return s.WriteString("{{Could not look up your location right now.}}::red\r\n")
			}
			return s.WriteString(formatWhereAmI(room))
		},
	}
}

func formatWhereAmI(room repo.Room) string {
	var b strings.Builder
	// ExternalID and Name are interpolated into cfmt-styled spans;
	// defang `}}::` in case a builder typo authored an injection.
	fmt.Fprintf(&b, "{{Room %d}}::cyan|bold {{(%s)}}::gray\r\n", room.ID, defangWorldField(room.ExternalID))
	fmt.Fprintf(&b, "  {{Name:}}::yellow %s\r\n", defangWorldField(room.Name))
	sector := room.Sector
	if sector == "" {
		sector = repo.SectorCity
	}
	fmt.Fprintf(&b, "  {{Sector:}}::yellow %s    {{Light:}}::yellow %d\r\n", defangWorldField(string(sector)), room.LightLevel)
	fmt.Fprintf(&b, "  {{Coords:}}::yellow (%d, %d, %d)\r\n", room.CoordX, room.CoordY, room.CoordZ)
	flags := whereFlags(room.Flags)
	if flags == "" {
		flags = "(none)"
	}
	fmt.Fprintf(&b, "  {{Flags:}}::yellow %s\r\n", flags)
	if len(room.ExtraDescs) > 0 {
		keys := make([]string, 0, len(room.ExtraDescs))
		for k := range room.ExtraDescs {
			keys = append(keys, defangWorldField(k))
		}
		sort.Strings(keys)
		fmt.Fprintf(&b, "  {{Keywords:}}::yellow %s\r\n", strings.Join(keys, ", "))
	}
	return b.String()
}

func whereFlags(f repo.RoomFlags) string {
	var on []string
	if f.Indoors {
		on = append(on, "indoors")
	}
	if f.NoPVP {
		on = append(on, "nopvp")
	}
	if f.NoTeleport {
		on = append(on, "noteleport")
	}
	if f.Dark {
		on = append(on, "dark")
	}
	if f.Silent {
		on = append(on, "silent")
	}
	if f.Peaceful {
		on = append(on, "peaceful")
	}
	return strings.Join(on, ", ")
}
