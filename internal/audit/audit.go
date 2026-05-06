// Package audit is the thin call-site wrapper privileged verbs use to
// land a row in admin_audit. It exists so callers don't repeat the
// "session may be nil, repo may be nil, log on failure" boilerplate.
package audit

import (
	"context"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// Record writes a single admin_audit row. Synchronous because we want
// the row flushed before side-effecting verbs return — `shutdown` in
// particular must commit its audit before the drain begins. SQLite
// inserts are sub-ms at MUD scale; if batching ever matters, the seam
// to add it lives here.
//
// Durability note: with sqlite synchronous=NORMAL (modernc default),
// the row reaches the OS page cache before this call returns. That
// covers a clean process exit (incl. shutdown / reboot drain) but
// not a hard power loss; switch the pragma if a stricter guarantee
// matters.
//
// Concurrency: this function reads s.CharacterID / s.CharacterName,
// which CLAUDE.md flags as dispatcher-owned. Callers must invoke from
// the session's dispatcher goroutine (every cmd Run handler today
// satisfies this). A future off-dispatcher caller (tick / event bus)
// must snapshot the actor scalars before calling.
//
// A nil repo is a no-op (memory-only test paths). A nil session yields
// a system-actor row (ActorCharacterID=0).
func Record(ctx context.Context, r repo.AdminAuditRepo, s *telnet.Session, verb, target, args string) {
	if r == nil {
		return
	}
	var (
		actorID   int64
		actorName string
	)
	if s != nil {
		actorID = s.CharacterID
		actorName = s.CharacterName
	}
	if err := r.Record(ctx, repo.AdminAuditEntry{
		ActorCharacterID: actorID,
		ActorType:        repo.ActorTypeCharacter,
		ActorName:        actorName,
		Verb:             verb,
		Target:           target,
		Args:             args,
	}); err != nil {
		slog.Warn("audit: record failed", "verb", verb, "actor", actorID, "err", err)
	}
}

// RecordAccount writes an admin_audit row attributed to an account
// rather than a character. Used by the post-login account menu, where
// the session has AccountID set but no CharacterID. accountName is a
// snapshot at write time (matches how Record snapshots ActorName).
//
// A nil repo is a no-op (memory-only test paths).
func RecordAccount(ctx context.Context, r repo.AdminAuditRepo, accountID int64, accountName, verb, target, args string) {
	if r == nil {
		return
	}
	if err := r.Record(ctx, repo.AdminAuditEntry{
		ActorAccountID: accountID,
		ActorType:      repo.ActorTypeAccount,
		ActorName:      accountName,
		Verb:           verb,
		Target:         target,
		Args:           args,
	}); err != nil {
		slog.Warn("audit: record failed", "verb", verb, "account", accountID, "err", err)
	}
}
