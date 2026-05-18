package main

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/metrics"
	"github.com/Jasrags/WheelMUD/internal/mode"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// buildCommandAuditFn returns the mode.Game audit closure when
// audit.commands_enabled is true. The closure resolves the verb from
// the first whitespace-separated token of the input line, skips
// guest / chargen sessions (no character bound), skips the configured
// exclude list, and records one character_audit row per dispatched
// line. Insert errors log at warn and are swallowed so a slow audit
// table can't tear down the dispatcher.
func buildCommandAuditFn(audits repo.CharacterAuditRepo, exclude []string) mode.CommandAuditFn {
	excludeSet := make(map[string]struct{}, len(exclude))
	for _, v := range exclude {
		excludeSet[strings.ToLower(strings.TrimSpace(v))] = struct{}{}
	}
	return func(ctx context.Context, s *telnet.Session, line string, _ error) {
		charID, charName, roomID := s.InWorld()
		if charID == 0 {
			return
		}
		verb := firstToken(line)
		if verb == "" {
			return
		}
		if _, skip := excludeSet[verb]; skip {
			return
		}
		if err := audits.Record(ctx, repo.CharacterAuditEntry{
			CharacterID:   charID,
			CharacterName: charName,
			RoomID:        roomID,
			Verb:          verb,
			Raw:           strings.TrimSpace(line),
		}); err != nil {
			slog.Warn("character_audit record failed",
				"character", charName, "verb", verb, "error", err)
		}
	}
}

// buildCommandMetricFn returns a Game metric hook that bumps the
// commands_total counter on every dispatch. The verb is the
// lowercased first whitespace-separated token; result is "ok" when
// dispatchErr is nil, "error" otherwise. Empty lines (whitespace-only
// input) are skipped so the counter doesn't accumulate empty-label
// noise.
func buildCommandMetricFn(m *metrics.Metrics) mode.CommandMetricFn {
	return func(line string, err error) {
		verb := firstToken(line)
		if verb == "" {
			return
		}
		m.ObserveCommand(verb, err == nil)
	}
}
