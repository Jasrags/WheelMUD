package repo

import (
	"context"

	"github.com/Jasrags/WheelMUD/internal/creature"
)

// OwnerKind discriminates which aggregate a channeling row belongs
// to. Values match the CHECK constraint in 0008_create_creatures.sql.
type OwnerKind int8

const (
	OwnerKindCharacter   OwnerKind = 1
	OwnerKindMobTemplate OwnerKind = 2
	OwnerKindMobInstance OwnerKind = 3
)

// Valid reports whether k is a known owner kind.
func (k OwnerKind) Valid() bool {
	return k == OwnerKindCharacter || k == OwnerKindMobTemplate || k == OwnerKindMobInstance
}

// ChannelingRepo persists the polymorphic channeling sub-record
// (channeling table). Attached to characters, mob templates, or
// mob instances via (owner_kind, owner_id). Non-channelers have no
// row; GetByOwner returns ErrChannelingNotFound rather than a zero
// value so callers can distinguish "not a channeler" from "freshly
// initialized".
type ChannelingRepo interface {
	// Upsert creates or replaces the channeling row for the given
	// owner. Returns ErrInvalidOwnerKind if kind is unknown.
	Upsert(ctx context.Context, kind OwnerKind, ownerID int64, c creature.Channeling) error
	// GetByOwner returns the channeling row for the given owner, or
	// ErrChannelingNotFound if there is none.
	GetByOwner(ctx context.Context, kind OwnerKind, ownerID int64) (creature.Channeling, error)
	// DeleteByOwner removes the row. Returns ErrChannelingNotFound
	// if the owner has no row.
	DeleteByOwner(ctx context.Context, kind OwnerKind, ownerID int64) error
}
