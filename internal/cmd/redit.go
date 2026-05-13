package cmd

import (
	"errors"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// PushREditFn is the closure cmd/server/main.go installs so the redit
// verb can push the editor mode without internal/cmd importing
// internal/mode. Same shape as PushDialogueFn (§F #30): keeps cmd ↔
// mode dependency direction one-way.
//
// The closure receives the session and the target room; it returns
// an error from Session.PushMode if OnEnter fails.
type PushREditFn func(s *telnet.Session, room repo.Room) error

// NewREdit builds the `redit [<id>]` admin verb. With no arg, edits
// the room the caller is currently in; with an arg, edits the room
// resolved via internal/cmd.resolveRoom (numeric id or external_id).
// Permission is gated by CanEditZone — AuthAdmin bypasses, builders
// must hold a grant on the room's zone.
//
// Auth: telnet.AuthPlayer. The verb itself is dispatchable by any
// player but refuses with the same NoPermission message that
// CanEditZone returns false for, so a non-authorised player learns
// nothing about the room's edit-ability beyond "you can't edit that."
func NewREdit(rooms repo.RoomRepo, push PushREditFn) *telnet.Command {
	return &telnet.Command{
		Name: "redit",
		Help: "redit [<id>] — edit the current room (or a named one); builder+",
		Long: "Usage: redit             - edit the room you're standing in\n" +
			"       redit <id>        - edit a numeric room id\n" +
			"       redit <external>  - edit by stable external_id (e.g. plaza.fountain)\n\n" +
			"Requires AuthAdmin or a per-zone builder grant on the target's zone.",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			return runREdit(c, rooms, push)
		},
	}
}

func runREdit(c *telnet.Context, rooms repo.RoomRepo, push PushREditFn) error {
	var (
		room repo.Room
		err  error
	)
	if len(c.Args) == 0 {
		if c.Session.CurrentRoomID == 0 {
			return c.Session.WriteString("{{You are nowhere — there is nothing to edit.}}::yellow\r\n")
		}
		room, err = rooms.FindByID(c.Ctx, c.Session.CurrentRoomID)
	} else {
		room, err = resolveRoom(c.Ctx, rooms, c.Args[0])
	}
	// System-level lookup failure (DB driver hiccup) gets its own line
	// because there's no security value in masking it and the operator
	// needs to know to retry.
	if err != nil && !errors.Is(err, repo.ErrRoomNotFound) {
		return c.Session.WriteString("{{Could not look up that room right now.}}::red\r\n")
	}
	// Admin can already enumerate rooms via `zones show` / `whereami`,
	// so giving them the precise "no such room" line costs no security
	// and is more useful when debugging an external_id typo. Non-admin
	// builders see the same refusal whether the room is missing or
	// outside their grants, so they cannot use redit as a side channel
	// to probe for valid external_ids.
	if errors.Is(err, repo.ErrRoomNotFound) {
		if c.Session.AuthLevel >= telnet.AuthAdmin {
			target := "the current room"
			if len(c.Args) > 0 {
				target = c.Args[0]
			}
			return c.Session.WriteString("{{No such room:}}::red " + defangCfmt(target) + "\r\n")
		}
		return c.Session.WriteString("{{You do not have permission to edit that room.}}::red\r\n")
	}
	if !CanEditZone(c.Session, room.ZoneID) {
		return c.Session.WriteString("{{You do not have permission to edit that room.}}::red\r\n")
	}
	if push == nil {
		// Defensive: the verb wired without a push closure indicates a
		// boot-order bug, not a user-facing condition. Log + refuse.
		return c.Session.WriteString("{{Editor not available right now.}}::red\r\n")
	}
	return push(c.Session, room)
}
