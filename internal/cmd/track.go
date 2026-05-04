package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// trackListLimit caps how many spawned mobs the verb scans when
// resolving the keyword. Aligns with the wander handler's
// DefaultWanderCap; if the world ever exceeds this, both will need
// pagination.
const trackListLimit = 200

// NewTrack builds the §10 admin tracking verb. Resolves a mob by
// keyword across every spawned mob in the world (`a guard`,
// `2.rat`), then reports the mob's current room and (when history
// is available) its most recent direction of travel + elapsed time.
//
// Gated at AuthAdmin until §12 ships the per-character skill check
// and staleness window. Until then, players see the same
// "Unknown command" they'd see for any other privileged verb.
//
// trackTimeSource defaults to time.Now when nil — see NewTrackAt for
// the testable seam.
func NewTrack(mobs repo.MobInstanceRepo, rooms repo.RoomRepo, exits repo.ExitRepo) *telnet.Command {
	return newTrackAt(mobs, rooms, exits, time.Now)
}

// newTrackAt is the test-friendly constructor that lets tests freeze
// the clock used to compute "X seconds ago" output.
func newTrackAt(mobs repo.MobInstanceRepo, rooms repo.RoomRepo, exits repo.ExitRepo, now func() time.Time) *telnet.Command {
	if now == nil {
		now = time.Now
	}
	return &telnet.Command{
		Name:    "track",
		Help:    "track <name> — report a mob's last-known room and heading",
		MinArgs: 1,
		Auth:    telnet.AuthAdmin,
		Run: func(c *telnet.Context) error {
			target := strings.TrimSpace(c.Args[0])
			if target == "" {
				return c.Session.WriteString("{{Track what?}}::yellow\r\n")
			}
			mob, ok, err := resolveTrackTarget(c.Ctx, mobs, target)
			if err != nil {
				slog.Warn("track: list spawned failed", "error", err)
				return c.Session.WriteString("{{Could not search for trails right now.}}::red\r\n")
			}
			if !ok {
				return c.Session.WriteString("{{You see no trail of that name.}}::yellow\r\n")
			}
			report, err := buildTrackReport(c.Ctx, mob, mobs, rooms, exits, now())
			if err != nil {
				slog.Warn("track: report build failed", "mob", mob.ID, "error", err)
				return c.Session.WriteString("{{Could not assemble that mob's trail right now.}}::red\r\n")
			}
			return c.Session.WriteString(report)
		},
	}
}

// resolveTrackTarget enumerates every spawned mob (capped at
// trackListLimit) and matches the keyword via the shared MatchMob
// helper, so ordinal disambiguation (`2.guard`) works the same way
// as look / examine / get.
func resolveTrackTarget(ctx context.Context, mobs repo.MobInstanceRepo, keyword string) (creature.MobInstance, bool, error) {
	list, err := mobs.ListSpawned(ctx, trackListLimit)
	if err != nil {
		return creature.MobInstance{}, false, err
	}
	mob, ok := MatchMob(keyword, list)
	return mob, ok, nil
}

// buildTrackReport renders the player-facing line(s) for a resolved
// mob. With ≥2 trail rows we can name the previous room and the
// direction of the most recent step; with 0–1 we just report the
// current location since direction inference needs two points.
func buildTrackReport(
	ctx context.Context,
	mob creature.MobInstance,
	mobs repo.MobInstanceRepo,
	rooms repo.RoomRepo,
	exits repo.ExitRepo,
	now time.Time,
) (string, error) {
	currentRoom, err := rooms.FindByID(ctx, mob.Core.CurrentRoomID)
	if err != nil && !errors.Is(err, repo.ErrRoomNotFound) {
		return "", fmt.Errorf("lookup current room: %w", err)
	}
	currentName := safeName(currentRoom.Name, "an unknown place")
	name := safeName(mob.Core.Name, "Something")

	trails, err := mobs.RecentTrails(ctx, mob.ID, 2)
	if err != nil {
		return "", fmt.Errorf("recent trails: %w", err)
	}
	if len(trails) < 2 {
		return fmt.Sprintf(
			"{{%s}}::cyan is in {{%s}}::yellow — no movement recorded yet.\r\n",
			name, currentName,
		), nil
	}

	newest, prev := trails[0], trails[1]
	dirText := inferStepDirection(ctx, exits, prev.RoomID, newest.RoomID)
	prevRoom, err := rooms.FindByID(ctx, prev.RoomID)
	if err != nil && !errors.Is(err, repo.ErrRoomNotFound) {
		return "", fmt.Errorf("lookup previous room: %w", err)
	}
	prevName := safeName(prevRoom.Name, "elsewhere")
	elapsed := now.Sub(newest.At).Round(time.Second)
	if elapsed < 0 {
		elapsed = 0
	}
	return fmt.Sprintf(
		"{{%s}}::cyan is in {{%s}}::yellow — last moved %s from {{%s}}::gray, about %s ago.\r\n",
		name, currentName, dirText, prevName, formatTrackElapsed(elapsed),
	), nil
}

// safeName scrubs a YAML-authored display string before it lands
// inside cfmt template tags. Strips control bytes and defangs the
// `{{` / `}}` / `::` triplets so a builder typo in mob/room names
// can't break tag rendering or leak terminal escapes. fallback is
// used when the input is empty or scrubs to empty.
func safeName(name, fallback string) string {
	if name == "" {
		return fallback
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return fallback
	}
	rep := strings.NewReplacer("{{", "{ {", "}}", "} }", "::", ": :")
	return rep.Replace(out)
}

// inferStepDirection returns the long-form direction name for the
// exit from prevRoomID to newestRoomID. Falls back to "onward" when
// no matching exit row exists — the move could have been a teleport,
// admin reseat, or a since-deleted exit.
func inferStepDirection(ctx context.Context, exits repo.ExitRepo, prevRoomID, newestRoomID int64) string {
	list, err := exits.ListFrom(ctx, prevRoomID)
	if err != nil {
		return "onward"
	}
	for _, e := range list {
		if e.ToRoomID == newestRoomID {
			return repo.DirLong(e.Direction)
		}
	}
	return "onward"
}

// formatTrackElapsed renders a duration as a short MUD-style string:
// "<1s" for sub-second, "23s" / "2m30s" / "1h05m" for longer spans.
// Defaults to seconds-only because the trail buffer caps at 16 hops
// which usually bounds reports to the minute range.
func formatTrackElapsed(d time.Duration) string {
	if d < time.Second {
		return "less than a second"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d / time.Minute)
		s := int((d % time.Minute) / time.Second)
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%dh%02dm", h, m)
}
