package repo

import (
	"context"
	"time"
)

// AdminAuditEntry is one row in the append-only privileged-verb log.
// ActorCharacterID is 0 for system-actor rows; ActorName is a snapshot
// at write time so renames don't rewrite history.
type AdminAuditEntry struct {
	ID               int64
	TS               time.Time
	ActorCharacterID int64
	ActorName        string
	Verb             string
	Target           string
	Args             string
}

// AdminAuditFilter narrows AdminAuditRepo.List. All fields zero-value
// to "no filter" semantics: empty Verbs lists everything, zero Actor
// matches all actors, zero Limit applies the impl's default cap.
type AdminAuditFilter struct {
	Since time.Time
	Verbs []string
	Actor int64
	Limit int
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
