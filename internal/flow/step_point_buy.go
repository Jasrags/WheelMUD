package flow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// PointBuyItem is one allocatable line in a PointBuyStep — typically
// an ability score in chargen, but the step is generic: any
// budget-constrained allocation across named items with per-item
// bounds and a shared cost table works.
type PointBuyItem struct {
	Key   string `yaml:"key"`
	Label string `yaml:"label"`
	Min   int    `yaml:"min"`
	Max   int    `yaml:"max"`
}

// PointBuyStep is a sticky-loop allocator. The player issues `+key` /
// `-key` commands to raise/lower individual items; the step re-renders
// after each tick showing the running totals and remaining budget.
// `done` commits when sum(cost) == Budget and advances to Next.
// `reset` re-sets every item to its Min.
//
// Costs is the cumulative cost lookup: cost of holding `Min+i` for an
// item is Costs[i]. Length must equal Max-Min+1 (with the same range
// across every item) — validated at first Handle.
//
// State is persisted in State.Values[StoreAs] as a JSON object
// {key: score}. Re-rendering the prompt reads this back, so a
// disconnect mid-allocation resumes with the same scores under §O.2.
//
// Mandatory: ID, PromptText, Items (≥1), Budget>0, Costs (≥1), StoreAs,
// Next. All items share the same Min/Max range (the simplest contract;
// chargen's 6 abilities all run 8–18). Per-item Min/Max differing from
// the cost-table range is a Validate-time error.
type PointBuyStep struct {
	ID         StepID         `yaml:"id"`
	PromptText string         `yaml:"prompt"`
	Items      []PointBuyItem `yaml:"items"`
	Budget     int            `yaml:"budget"`
	Costs      []int          `yaml:"costs"`

	StoreAs string `yaml:"store_as"`
	Next    StepID `yaml:"next"`
}

func (s *PointBuyStep) StepID() StepID       { return s.ID }
func (s *PointBuyStep) ValidatorRef() string { return "" }
func (s *PointBuyStep) ActionRef() string    { return "" }

// Prompt renders the static prompt plus a per-item readout with the
// current score and the remaining budget. Reads the working state out
// of State.Values[StoreAs] so the display survives a resume.
func (s *PointBuyStep) Prompt(state *State) string {
	working := s.loadWorking(state)
	var b strings.Builder
	b.WriteString(s.PromptText)
	b.WriteString("\r\n")
	spent := 0
	for _, item := range s.Items {
		score := working[item.Key]
		cost := s.costFor(score)
		spent += cost
		fmt.Fprintf(&b, "  %-12s %2d  (cost %d)\r\n", item.Label, score, cost)
	}
	remaining := s.Budget - spent
	fmt.Fprintf(&b, "Budget remaining: %d / %d\r\n", remaining, s.Budget)
	b.WriteString("(+key / -key to adjust, `done` to commit, `reset` to clear)\r\n")
	return b.String()
}

// Handle parses one of `+key`, `-key`, `done`, `reset`. Returns the
// same StepID for re-prompt on every command except `done` (success
// advances to Next).
func (s *PointBuyStep) Handle(state *State, input string) (StepID, error) {
	if err := s.validateShape(); err != nil {
		return s.ID, err
	}
	working := s.loadWorking(state)
	tok := strings.ToLower(strings.TrimSpace(input))
	switch {
	case tok == "":
		return s.ID, &ValidationError{Message: "Type +key, -key, done, or reset."}
	case tok == "done":
		spent := s.spent(working)
		if spent != s.Budget {
			return s.ID, &ValidationError{Message: fmt.Sprintf("Budget is %d; you've spent %d. Adjust before `done`.", s.Budget, spent)}
		}
		return s.commit(state, working)
	case tok == "reset":
		for _, item := range s.Items {
			working[item.Key] = item.Min
		}
		s.saveWorking(state, working)
		return s.ID, nil
	case strings.HasPrefix(tok, "+"):
		return s.adjust(state, working, tok[1:], +1)
	case strings.HasPrefix(tok, "-"):
		return s.adjust(state, working, tok[1:], -1)
	}
	return s.ID, &ValidationError{Message: fmt.Sprintf("Unknown command %q. Type +key, -key, done, or reset.", input)}
}

func (s *PointBuyStep) adjust(state *State, working map[string]int, key string, delta int) (StepID, error) {
	item, ok := s.findItem(key)
	if !ok {
		return s.ID, &ValidationError{Message: fmt.Sprintf("Unknown item %q.", key)}
	}
	cur := working[item.Key]
	next := cur + delta
	if next < item.Min || next > item.Max {
		return s.ID, &ValidationError{Message: fmt.Sprintf("%s is bounded to %d–%d.", item.Label, item.Min, item.Max)}
	}
	// Pre-check budget for raises so we don't allow an over-spend mid-allocation.
	if delta > 0 {
		hypothetical := working[item.Key]
		working[item.Key] = next
		spent := s.spent(working)
		working[item.Key] = hypothetical
		if spent > s.Budget {
			return s.ID, &ValidationError{Message: fmt.Sprintf("Not enough budget to raise %s.", item.Label)}
		}
	}
	working[item.Key] = next
	s.saveWorking(state, working)
	return s.ID, nil
}

func (s *PointBuyStep) commit(state *State, working map[string]int) (StepID, error) {
	s.saveWorking(state, working)
	return s.Next, nil
}

// findItem returns the item with matching Key (case-insensitive on
// the supplied token). Returns (zero, false) when not found.
func (s *PointBuyStep) findItem(key string) (PointBuyItem, bool) {
	lc := strings.ToLower(key)
	for _, item := range s.Items {
		if strings.ToLower(item.Key) == lc {
			return item, true
		}
	}
	return PointBuyItem{}, false
}

// costFor reads Costs at index (score - Items[0].Min). Returns 0 if
// the score sits outside the table — the bounds enforcement in adjust
// keeps this from happening on a healthy path.
func (s *PointBuyStep) costFor(score int) int {
	if len(s.Items) == 0 || len(s.Costs) == 0 {
		return 0
	}
	idx := score - s.Items[0].Min
	if idx < 0 || idx >= len(s.Costs) {
		return 0
	}
	return s.Costs[idx]
}

func (s *PointBuyStep) spent(working map[string]int) int {
	total := 0
	for _, item := range s.Items {
		total += s.costFor(working[item.Key])
	}
	return total
}

// validateShape catches catalog-authoring errors at first Handle:
// per-item ranges all match Costs's length. Once-per-Handle cost is
// cheap; doing it at YAML-load time would require a Validate hook on
// Step which we deliberately avoided in §O.0.
func (s *PointBuyStep) validateShape() error {
	if len(s.Items) == 0 {
		return fmt.Errorf("PointBuyStep %q: no Items", s.ID)
	}
	if len(s.Costs) == 0 {
		return fmt.Errorf("PointBuyStep %q: no Costs table", s.ID)
	}
	first := s.Items[0]
	wantLen := first.Max - first.Min + 1
	if wantLen != len(s.Costs) {
		return fmt.Errorf("PointBuyStep %q: Costs length %d does not match Item range %d..%d", s.ID, len(s.Costs), first.Min, first.Max)
	}
	for _, item := range s.Items[1:] {
		if item.Min != first.Min || item.Max != first.Max {
			return fmt.Errorf("PointBuyStep %q: item %q range %d..%d differs from %q range %d..%d", s.ID, item.Key, item.Min, item.Max, first.Key, first.Min, first.Max)
		}
	}
	if s.Budget <= 0 {
		return fmt.Errorf("PointBuyStep %q: Budget must be positive", s.ID)
	}
	if s.StoreAs == "" {
		return fmt.Errorf("PointBuyStep %q: StoreAs is required", s.ID)
	}
	return nil
}

// loadWorking reads the JSON-encoded scores out of State.Values; if
// absent (first-touch or reset), every item starts at its Min. Never
// returns nil — callers index freely.
func (s *PointBuyStep) loadWorking(state *State) map[string]int {
	out := make(map[string]int, len(s.Items))
	for _, item := range s.Items {
		out[item.Key] = item.Min
	}
	if state == nil || s.StoreAs == "" {
		return out
	}
	raw, ok := state.Values[s.StoreAs]
	if !ok || raw == "" {
		return out
	}
	stored := map[string]int{}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		// Corrupt blob — fall back to defaults. Conservatively reset
		// rather than abort the step; the player can re-allocate.
		return out
	}
	for k, v := range stored {
		if _, known := out[k]; known {
			out[k] = v
		}
	}
	return out
}

// saveWorking encodes scores under StoreAs. Sorted keys keep the
// persisted blob byte-stable across saves — easier to diff in
// forensic dumps.
func (s *PointBuyStep) saveWorking(state *State, working map[string]int) {
	keys := make([]string, 0, len(working))
	for k := range working {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Build the map in key-sorted order so json.Marshal emits keys
	// in the natural ability order (encoding/json's map output is
	// already sorted, but constructing from a fresh map for clarity).
	ordered := make(map[string]int, len(working))
	for _, k := range keys {
		ordered[k] = working[k]
	}
	encoded, err := json.Marshal(ordered)
	if err != nil {
		// Marshaling map[string]int never fails in practice.
		return
	}
	state.SetValue(s.StoreAs, string(encoded))
}
