package repo

import (
	"context"
	"time"
)

// AdminAuditEntry is one row in the append-only privileged-verb log.
// ActorCharacterID is 0 for system-actor rows and account-mode rows;
// ActorAccountID is 0 for character-mode rows and system rows. ActorType
// disambiguates: "character" for in-game admin verbs (spawn, teleport,
// goto, …), "account" for post-login account-menu actions
// (delete-character, password change, settings — slice 1b onward).
// ActorName is a snapshot at write time so renames don't rewrite history.
type AdminAuditEntry struct {
	ID               int64
	TS               time.Time
	ActorCharacterID int64
	ActorAccountID   int64
	ActorType        string // "character" (default) | "account"
	ActorName        string
	Verb             string
	Target           string
	Args             string
}

// ActorTypeCharacter / ActorTypeAccount are the two values
// AdminAuditEntry.ActorType takes today. Stored as TEXT for forensic
// legibility when reading the audit table directly.
const (
	ActorTypeCharacter = "character"
	ActorTypeAccount   = "account"
)

// AdminAuditFilter narrows AdminAuditRepo.List. All fields zero-value
// to "no filter" semantics: empty Verbs lists everything, zero Actor
// matches all character-mode actors, zero ActorAccount matches all
// account-mode actors, zero Limit applies the impl's default cap.
//
// Actor and ActorAccount are independent filters mapping to
// actor_character_id and actor_account_id respectively (migration
// 0034). A row matches if its corresponding column equals the filter;
// zero on a filter means "don't constrain that column."
type AdminAuditFilter struct {
	Since        time.Time
	Verbs        []string
	Actor        int64
	ActorAccount int64
	Limit        int
}

// AdminAuditRepo persists privileged-verb invocations. Append-only:
// no Update, no Delete. Read paths go through List.
type AdminAuditRepo interface {
	Record(ctx context.Context, e AdminAuditEntry) error
	List(ctx context.Context, f AdminAuditFilter) ([]AdminAuditEntry, error)
}

// DefaultAdminAuditListLimit caps an unfiltered List so an admin
// viewer verb can't accidentally page in the whole table.
const DefaultAdminAuditListLimit = 100
