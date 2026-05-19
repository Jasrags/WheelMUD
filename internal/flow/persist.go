package flow

import "context"

// Persister is the optional storage hook the Runner calls on every
// successful step transition (Save) and on terminal exit — both
// normal Completed and user-initiated Cancelled (Delete).
//
// Implementations live outside this package: the production adapter
// in `cmd/server/main.go` wraps a `repo.FlowStateRepo`. The engine
// never imports `repo`, keeping the persistence layer pluggable
// (memory repo for tests, sqlite for prod, future cloud KV).
//
// The Runner treats nil Persisters as a no-op so the §O.0 in-memory
// path keeps working. State.AccountID == 0 also skips the Save —
// anonymous test flows have no row to write.
//
// Save errors abort the flow (a flow that can't persist mid-run is
// in a bad state). Delete errors do NOT abort — the flow already
// succeeded or cancelled, and a stuck row gets cleaned up by the
// next eviction cycle.
type Persister interface {
	Save(ctx context.Context, s *State) error
	Delete(ctx context.Context, accountID int64, flowID string) error
}
