package flow

import (
	"fmt"
	"sort"
	"sync"
)

// ActionFn runs after a Step.Handle succeeds. It receives the
// post-Handle State (Values populated, Current still pointing at the
// step that just ran) and may have arbitrary side effects: write to
// a repo, emit an event, etc. A non-nil error aborts the flow —
// validation belongs in Step.Handle / ValidatorFn, not here.
//
// Actions execute synchronously on the dispatcher goroutine of
// whatever code drove Runner.Submit, mirroring how other cmd-layer
// side effects flow today. Long-running work should be offloaded by
// the action implementation; the Runner does not spawn goroutines.
type ActionFn func(state *State) error

// ValidatorFn checks raw input before a Step accepts it. Returns
// nil on success; *ValidationError on a user-visible rejection;
// other errors bubble up to abort the flow.
//
// Steps that need to inspect prior State.Values during validation
// (e.g. "this name is unique among already-loaded characters") see
// the populated state. Validators must not mutate state — that's
// the Step.Handle's job, after validation passes.
type ValidatorFn func(state *State, input string) error

// ActionRegistry maps catalog string keys to ActionFn instances.
// Concurrent-safe; built once at boot via Register calls and read
// repeatedly during flow execution. Adding actions at runtime is
// supported but discouraged — the registry is meant to be locked in
// before the first flow runs.
type ActionRegistry struct {
	mu  sync.RWMutex
	fns map[string]ActionFn
}

// NewActionRegistry returns an empty registry.
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{fns: map[string]ActionFn{}}
}

// Register installs an action under `key`. Returns an error on
// duplicate key (loud boot failure instead of silent overwrite) or
// blank key. Nil fn is rejected up front so a malformed call doesn't
// silently break a future Lookup.
func (r *ActionRegistry) Register(key string, fn ActionFn) error {
	if key == "" {
		return fmt.Errorf("flow: action key is blank")
	}
	if fn == nil {
		return fmt.Errorf("flow: action %q is nil", key)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.fns[key]; dup {
		return fmt.Errorf("flow: duplicate action key %q", key)
	}
	r.fns[key] = fn
	return nil
}

// Lookup returns the action fn and a presence bool. Concurrent with
// Register.
func (r *ActionRegistry) Lookup(key string) (ActionFn, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.fns[key]
	return fn, ok
}

// Keys returns every registered action key sorted alphabetically.
// Useful for admin verbs that introspect the registry; not used by
// the runner itself.
func (r *ActionRegistry) Keys() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.fns))
	for k := range r.fns {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ValidatorRegistry mirrors ActionRegistry for ValidatorFn entries.
// Kept distinct from actions so a step can't accidentally reference
// a validator as its post-success action (different signatures, but
// the typed registries also catch the intent error at registration
// time).
type ValidatorRegistry struct {
	mu  sync.RWMutex
	fns map[string]ValidatorFn
}

// NewValidatorRegistry returns an empty registry.
func NewValidatorRegistry() *ValidatorRegistry {
	return &ValidatorRegistry{fns: map[string]ValidatorFn{}}
}

// Register installs a validator under `key`. Same blank/nil/dup
// rules as ActionRegistry.Register.
func (r *ValidatorRegistry) Register(key string, fn ValidatorFn) error {
	if key == "" {
		return fmt.Errorf("flow: validator key is blank")
	}
	if fn == nil {
		return fmt.Errorf("flow: validator %q is nil", key)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.fns[key]; dup {
		return fmt.Errorf("flow: duplicate validator key %q", key)
	}
	r.fns[key] = fn
	return nil
}

// Lookup returns the validator fn and a presence bool. Concurrent
// with Register.
func (r *ValidatorRegistry) Lookup(key string) (ValidatorFn, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.fns[key]
	return fn, ok
}

// Keys returns every registered validator key sorted alphabetically.
func (r *ValidatorRegistry) Keys() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.fns))
	for k := range r.fns {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
