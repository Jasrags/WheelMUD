package repo

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryFlowStateRepo is a map-backed FlowStateRepo for tests and
// non-persistent runs. Concurrent-safe.
type MemoryFlowStateRepo struct {
	mu   sync.RWMutex
	rows map[flowStateKey]FlowState
}

type flowStateKey struct {
	accountID int64
	flowID    string
}

func NewMemoryFlowStateRepo() *MemoryFlowStateRepo {
	return &MemoryFlowStateRepo{rows: map[flowStateKey]FlowState{}}
}

// Save writes or overwrites the row for (AccountID, FlowID). When
// inserting a new row would push the account over
// MaxFlowStatesPerAccount, the row with the oldest UpdatedAt for
// that account is evicted in the same critical section.
func (r *MemoryFlowStateRepo) Save(_ context.Context, fs FlowState) error {
	if fs.UpdatedAt.IsZero() {
		fs.UpdatedAt = time.Now().UTC()
	}
	if fs.StartedAt.IsZero() {
		fs.StartedAt = fs.UpdatedAt
	}
	if fs.Values == nil {
		fs.Values = map[string]string{}
	}
	key := flowStateKey{fs.AccountID, fs.FlowID}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Update-in-place: no cap check needed.
	if _, ok := r.rows[key]; ok {
		r.rows[key] = cloneFlowState(fs)
		return nil
	}

	// Insert path — enforce cap.
	var perAccount []FlowState
	for k, v := range r.rows {
		if k.accountID == fs.AccountID {
			perAccount = append(perAccount, v)
		}
	}
	if len(perAccount) >= MaxFlowStatesPerAccount {
		sort.Slice(perAccount, func(i, j int) bool {
			return perAccount[i].UpdatedAt.Before(perAccount[j].UpdatedAt)
		})
		evict := perAccount[0]
		delete(r.rows, flowStateKey{evict.AccountID, evict.FlowID})
	}
	r.rows[key] = cloneFlowState(fs)
	return nil
}

func (r *MemoryFlowStateRepo) Load(_ context.Context, accountID int64, flowID string) (FlowState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fs, ok := r.rows[flowStateKey{accountID, flowID}]
	if !ok {
		return FlowState{}, ErrFlowStateNotFound
	}
	return cloneFlowState(fs), nil
}

// Delete drops the row for (accountID, flowID). Idempotent.
func (r *MemoryFlowStateRepo) Delete(_ context.Context, accountID int64, flowID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, flowStateKey{accountID, flowID})
	return nil
}

func (r *MemoryFlowStateRepo) ListByAccount(_ context.Context, accountID int64) ([]FlowState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []FlowState
	for k, v := range r.rows {
		if k.accountID == accountID {
			out = append(out, cloneFlowState(v))
		}
	}
	// Newest first matches the sqlite impl's index order.
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// cloneFlowState defensively copies Values so callers can't mutate
// repo-owned state through a returned reference.
func cloneFlowState(fs FlowState) FlowState {
	if fs.Values == nil {
		return fs
	}
	clone := make(map[string]string, len(fs.Values))
	for k, v := range fs.Values {
		clone[k] = v
	}
	fs.Values = clone
	return fs
}
