package repo

import (
	"context"
	"errors"
	"time"
)

// FlowState is one row in flow_state — the persisted runtime snapshot
// of an in-flight `internal/flow` Runner. The row exists for the
// lifetime of a flow between Start and Completed/Cancelled; both
// terminal states trigger Delete.
//
// Values mirrors flow.State.Values verbatim; the sqlite impl
// JSON-marshals it through values_json.
type FlowState struct {
	AccountID   int64
	FlowID      string
	CurrentStep string
	Values      map[string]string
	StartedAt   time.Time
	UpdatedAt   time.Time
}

// FlowStateRepo persists per-(account, flow) runner state for the
// Phase O.2 resume-on-reconnect path. Single-writer semantics: the
// session that owns the flow is the only writer; concurrent sessions
// for the same account are blocked one layer up by
// `internal/session`.
//
// Save enforces MaxFlowStatesPerAccount via LRU eviction (oldest
// updated_at goes first) so a stuck wizard cannot pile up rows.
type FlowStateRepo interface {
	Save(ctx context.Context, fs FlowState) error
	Load(ctx context.Context, accountID int64, flowID string) (FlowState, error)
	Delete(ctx context.Context, accountID int64, flowID string) error
	ListByAccount(ctx context.Context, accountID int64) ([]FlowState, error)
}

// MaxFlowStatesPerAccount caps how many in-flight resumable flows a
// single account can accumulate. New Save at cap evicts the row with
// the oldest updated_at. Picked from the realistic-upper-bound side:
// chargen + one or two admin editors covers every plausible case.
const MaxFlowStatesPerAccount = 4

// ErrFlowStateNotFound is returned by Load when no row matches the
// (account_id, flow_id) pair. Delete is idempotent and never returns
// this — the caller is allowed to clear a row that was never written.
var ErrFlowStateNotFound = errors.New("repo: flow_state not found")
