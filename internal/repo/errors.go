package repo

import "errors"

// Cross-cutting world-aggregate sentinels. Returned from the Create
// methods on RoomRepo / ItemRepo / MobRepo / ExitRepo when the
// caller-supplied identity is unusable. Aggregate-specific not-found
// errors live next to their interface.
var (
	ErrInvalidExternalID   = errors.New("repo: external_id must be non-empty")
	ErrDuplicateExternalID = errors.New("repo: external_id already taken")
)
