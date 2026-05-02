// Package session holds the process-level registry that enforces the
// single-session-per-account policy: a successful login that finds an
// existing live session for the same account disconnects the old one.
//
// The registry is intentionally tiny and decoupled from the rest of
// the project — it knows about *telnet.Session pointers and account
// IDs, nothing else.
package session

import (
	"strings"
	"sync"

	"github.com/Jasrags/WheelMUD/telnet"
)

// Registry tracks at most one live *telnet.Session per account ID.
// Bind atomically swaps a new session in and returns the previous
// occupant (if any) so the caller can disconnect it. Unbind removes
// a session via compare-and-delete so a stale teardown defer cannot
// blow away the binding of a newer session that has since taken over.
type Registry struct {
	mu    sync.Mutex
	bound map[int64]*telnet.Session
}

func NewRegistry() *Registry {
	return &Registry{bound: make(map[int64]*telnet.Session)}
}

// Bind associates accountID with s. If another session was previously
// bound to the same account, it is returned (caller responsibility:
// notify + disconnect). Always sets the registry to the new session.
func (r *Registry) Bind(accountID int64, s *telnet.Session) (prev *telnet.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev = r.bound[accountID]
	r.bound[accountID] = s
	return prev
}

// Unbind removes accountID's binding only when the bound session is s.
// A no-op when the registry has been rebound to a different session
// (i.e., this session was the loser of a takeover race) or when there
// was no binding.
func (r *Registry) Unbind(accountID int64, s *telnet.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bound[accountID] == s {
		delete(r.bound, accountID)
	}
}

// Lookup returns the currently bound session for accountID, or nil.
// Holds the lock briefly; callers should not hold the returned
// pointer indefinitely.
func (r *Registry) Lookup(accountID int64) *telnet.Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bound[accountID]
}

// FindByCharacterName returns the bound session whose
// Session.CharacterName matches name (case-insensitive), or nil. The
// `tell` command uses this to resolve a recipient. O(n) over bound
// accounts — fine for hundreds of players, would want indexing for
// thousands.
func (r *Registry) FindByCharacterName(name string) *telnet.Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.bound {
		if strings.EqualFold(s.CharacterName, name) {
			return s
		}
	}
	return nil
}

// Snapshot returns a copy of the current bindings. Useful for the
// `who` command and for tests. The caller owns the map.
func (r *Registry) Snapshot() map[int64]*telnet.Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[int64]*telnet.Session, len(r.bound))
	for k, v := range r.bound {
		out[k] = v
	}
	return out
}
