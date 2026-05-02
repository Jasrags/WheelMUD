package repo

import "errors"

// Cross-cutting world-aggregate sentinels. Returned from the Create
// methods on RoomRepo / ItemRepo / MobRepo / ExitRepo when the
// caller-supplied identity is unusable. Aggregate-specific not-found
// errors live next to their interface.
var (
	ErrInvalidExternalID   = errors.New("repo: external_id must be non-empty")
	ErrDuplicateExternalID = errors.New("repo: external_id already taken")

	// Creature aggregates (mob templates / instances / channeling).
	ErrTemplateNotFound    = errors.New("repo: mob template not found")
	ErrInstanceNotFound    = errors.New("repo: mob instance not found")
	ErrChannelingNotFound  = errors.New("repo: channeling record not found")
	ErrInvalidOwnerKind    = errors.New("repo: channeling owner_kind must be 1 (character), 2 (template), or 3 (instance)")
)
