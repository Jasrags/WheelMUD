package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewWhereAmI builds the room-debug command. Useful for builders /
// admins to verify that flag, sector, light, and coord columns landed
// the way the YAML implies. AuthPlayer for now so it's available
// without a privileged role; promote to AuthAdmin alongside teleport
// when an admin tier exists.
func NewWhereAmI(rooms repo.RoomRepo) *telnet.Command {
	return &telnet.Command{
		Name: "whereami",
		Help: "Show the current room's id, sector, flags, light, and coords",
		Auth: telnet.AuthPlayer,
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
	fmt.Fprintf(&b, "{{Room %d}}::cyan|bold {{(%s)}}::gray\r\n", room.ID, room.ExternalID)
	fmt.Fprintf(&b, "  {{Name:}}::yellow %s\r\n", room.Name)
	sector := room.Sector
	if sector == "" {
		sector = repo.SectorCity
	}
	fmt.Fprintf(&b, "  {{Sector:}}::yellow %s    {{Light:}}::yellow %d\r\n", sector, room.LightLevel)
	fmt.Fprintf(&b, "  {{Coords:}}::yellow (%d, %d, %d)\r\n", room.CoordX, room.CoordY, room.CoordZ)
	flags := whereFlags(room.Flags)
	if flags == "" {
		flags = "(none)"
	}
	fmt.Fprintf(&b, "  {{Flags:}}::yellow %s\r\n", flags)
	if len(room.ExtraDescs) > 0 {
		keys := make([]string, 0, len(room.ExtraDescs))
		for k := range room.ExtraDescs {
			keys = append(keys, k)
		}
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
