package trigger

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// Registry is the in-memory index of triggers, keyed by
// (owner_kind, owner_id, event). Built once at boot via Reload from
// repo.TriggerRepo; Dispatcher consults it on every relevant bus
// event.
//
// All fields are copy-on-read — the dispatcher never holds the lock
// while invoking handlers, so an action handler that itself triggers
// another event will not deadlock.
type Registry struct {
	mu sync.RWMutex
	// byOwner: kind -> ownerID -> event -> []Trigger (priority-ordered)
	byOwner map[OwnerKind]map[int64]map[Event][]repo.Trigger
	// byEvent: event -> []Trigger (used by on_tick fan-out)
	byEvent map[Event][]repo.Trigger
}

// NewRegistry returns an empty Registry. Use Reload to populate from
// a TriggerRepo.
func NewRegistry() *Registry {
	return &Registry{
		byOwner: make(map[OwnerKind]map[int64]map[Event][]repo.Trigger),
		byEvent: make(map[Event][]repo.Trigger),
	}
}

// Reload rebuilds the in-memory index from r. Triggers whose Action
// is not registered in actions are kept (so a typo can be diagnosed
// at fire time with a clear "unknown action" warning rather than
// silent drop at boot). Returns the count of triggers indexed.
func (g *Registry) Reload(ctx context.Context, r repo.TriggerRepo) (int, error) {
	if r == nil {
		return 0, nil
	}
	all, err := r.ListAll(ctx)
	if err != nil {
		return 0, err
	}
	g.Replace(all)
	return len(all), nil
}

// Replace swaps the index for the given trigger set. Used by Reload
// and by tests that build a Registry from a literal slice.
func (g *Registry) Replace(triggers []repo.Trigger) {
	byOwner := make(map[OwnerKind]map[int64]map[Event][]repo.Trigger)
	byEvent := make(map[Event][]repo.Trigger)
	for _, t := range triggers {
		if !repo.ValidTriggerOwnerKind(t.OwnerKind) || !repo.ValidTriggerEvent(t.Event) {
			continue
		}
		owners, ok := byOwner[t.OwnerKind]
		if !ok {
			owners = make(map[int64]map[Event][]repo.Trigger)
			byOwner[t.OwnerKind] = owners
		}
		events, ok := owners[t.OwnerID]
		if !ok {
			events = make(map[Event][]repo.Trigger)
			owners[t.OwnerID] = events
		}
		events[t.Event] = append(events[t.Event], t)
		byEvent[t.Event] = append(byEvent[t.Event], t)
	}
	// Stable priority-DESC ordering within each (owner, event).
	for _, owners := range byOwner {
		for _, events := range owners {
			for ev, list := range events {
				sortByPriority(list)
				events[ev] = list
			}
		}
	}
	for ev, list := range byEvent {
		sortByPriority(list)
		byEvent[ev] = list
	}
	g.mu.Lock()
	g.byOwner = byOwner
	g.byEvent = byEvent
	g.mu.Unlock()
}

// UpdateFault patches the in-memory index entries for the given
// trigger ID. Phase F #32 slice 1 — runner.recordFault calls this
// after each Lua fault so the next dispatch sees the new
// consecutive_faults / disabled state without round-tripping the
// repo. The repo write is the source of truth across restarts;
// this index update is the same-process fast path.
func (g *Registry) UpdateFault(triggerID int64, faults int, disabled bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, owners := range g.byOwner {
		for _, events := range owners {
			for ev, list := range events {
				for i := range list {
					if list[i].ID == triggerID {
						list[i].ConsecutiveFaults = faults
						list[i].Disabled = disabled
					}
				}
				events[ev] = list
			}
		}
	}
	for ev, list := range g.byEvent {
		for i := range list {
			if list[i].ID == triggerID {
				list[i].ConsecutiveFaults = faults
				list[i].Disabled = disabled
			}
		}
		g.byEvent[ev] = list
	}
}

// ForOwnerEvent returns triggers attached to (kind, ownerID) for
// the given event, priority-DESC. Returns a copy so callers can
// iterate without holding the lock.
func (g *Registry) ForOwnerEvent(kind OwnerKind, ownerID int64, ev Event) []repo.Trigger {
	g.mu.RLock()
	defer g.mu.RUnlock()
	owners, ok := g.byOwner[kind]
	if !ok {
		return nil
	}
	events, ok := owners[ownerID]
	if !ok {
		return nil
	}
	src := events[ev]
	if len(src) == 0 {
		return nil
	}
	out := make([]repo.Trigger, len(src))
	copy(out, src)
	return out
}

// AllByEvent returns every trigger registered for ev, priority-DESC.
// Used by the on_tick fan-out.
func (g *Registry) AllByEvent(ev Event) []repo.Trigger {
	g.mu.RLock()
	defer g.mu.RUnlock()
	src := g.byEvent[ev]
	if len(src) == 0 {
		return nil
	}
	out := make([]repo.Trigger, len(src))
	copy(out, src)
	return out
}

// HasOwnerKindEvent reports whether any trigger is registered for the
// given (kind, event) pair. Used by Dispatcher to skip room mob
// expansion when no on_say / on_enter triggers exist anywhere.
func (g *Registry) HasOwnerKindEvent(kind OwnerKind, ev Event) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	owners, ok := g.byOwner[kind]
	if !ok {
		return false
	}
	for _, events := range owners {
		if len(events[ev]) > 0 {
			return true
		}
	}
	return false
}

// MatchSay reports whether the trigger's Match keyword (case-
// insensitive substring) is in text. Empty Match always matches.
func MatchSay(t repo.Trigger, text string) bool {
	if t.Match == "" {
		return true
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(t.Match))
}

func sortByPriority(list []repo.Trigger) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Priority != list[j].Priority {
			return list[i].Priority > list[j].Priority
		}
		return list[i].ID < list[j].ID
	})
}
