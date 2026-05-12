package repo

import (
	"context"
	"time"
)

// CharacterAuditEntry is one row in the per-character command audit log
// (migration 0052). Append-only — no Update, no Delete. CharacterName
// and RoomID are snapshots at write time so renames / moves don't
// rewrite history.
type CharacterAuditEntry struct {
	ID            int64
	TS            time.Time
	CharacterID   int64
	CharacterName string
	RoomID        int64
	Verb          string
	Raw           string
}

// CharacterAuditFilter narrows CharacterAuditRepo.List. All fields
// zero-value to "no filter": empty Verbs lists everything, zero
// Character matches all characters, zero Limit applies the default cap.
type CharacterAuditFilter struct {
	Since     time.Time
	Verbs     []string
	Character int64
	Limit     int
}

// CharacterAuditRepo persists in-game command invocations.
// Append-only: no Update, no Delete.
type CharacterAuditRepo interface {
	Record(ctx context.Context, e CharacterAuditEntry) error
	List(ctx context.Context, f CharacterAuditFilter) ([]CharacterAuditEntry, error)
}

// DefaultCharacterAuditListLimit caps an unfiltered List so a forensic
// viewer can't accidentally page in the entire table.
const DefaultCharacterAuditListLimit = 100

// CharacterAuditRawCap is the maximum byte length of the Raw column at
// insert time. Longer lines are truncated so a malicious or buggy
// client can't fill the table with multi-megabyte rows. The cap is
// large enough that any realistic command (including alias-expanded
// semicolon chains) fits unchanged.
const CharacterAuditRawCap = 4096
