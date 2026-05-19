package flow

import (
	"encoding/json"
	"strings"
	"testing"
)

// chargen6 is a representative point-buy config matching the
// hand-rolled WheelMUD chargen: 6 abilities, range 8–18, cumulative
// cost table, budget 25.
func chargen6() *PointBuyStep {
	return &PointBuyStep{
		ID:         "abil",
		PromptText: "Allocate ability scores:",
		Items: []PointBuyItem{
			{Key: "str", Label: "STR", Min: 8, Max: 18},
			{Key: "dex", Label: "DEX", Min: 8, Max: 18},
			{Key: "con", Label: "CON", Min: 8, Max: 18},
			{Key: "int", Label: "INT", Min: 8, Max: 18},
			{Key: "wis", Label: "WIS", Min: 8, Max: 18},
			{Key: "cha", Label: "CHA", Min: 8, Max: 18},
		},
		Budget: 25,
		// idx 0  1  2  3  4  5  6  7  8   9  10
		// score 8  9 10 11 12 13 14 15 16 17  18
		Costs:   []int{0, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16},
		StoreAs: "abilities",
		Next:    "review",
	}
}

func TestPointBuyStep_Prompt_ShowsScoresAndBudget(t *testing.T) {
	step := chargen6()
	state := &State{Values: map[string]string{}}
	got := step.Prompt(state)
	for _, want := range []string{"STR", "DEX", "CON", "INT", "WIS", "CHA", "Budget remaining: 25 / 25"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q in %q", want, got)
		}
	}
}

func TestPointBuyStep_AdjustRaiseAndLower(t *testing.T) {
	step := chargen6()
	state := &State{Values: map[string]string{}}
	if _, err := step.Handle(state, "+str"); err != nil {
		t.Fatalf("+str: %v", err)
	}
	if _, err := step.Handle(state, "+str"); err != nil {
		t.Fatalf("+str again: %v", err)
	}
	// Should have str=10 now.
	working := step.loadWorking(state)
	if working["str"] != 10 {
		t.Errorf("str = %d, want 10", working["str"])
	}
	if _, err := step.Handle(state, "-str"); err != nil {
		t.Fatalf("-str: %v", err)
	}
	working = step.loadWorking(state)
	if working["str"] != 9 {
		t.Errorf("after -str, str = %d, want 9", working["str"])
	}
}

func TestPointBuyStep_AdjustRespectsItemBounds(t *testing.T) {
	step := chargen6()
	state := &State{Values: map[string]string{}}
	_, err := step.Handle(state, "-str")
	if !IsValidationError(err) {
		t.Fatalf("-str at min: expected ValidationError, got %v", err)
	}
}

func TestPointBuyStep_RejectsRaiseBeyondBudget(t *testing.T) {
	step := chargen6()
	state := &State{Values: map[string]string{}}
	// Try to max str to 18 (cost 16) and dex to 17 (cost 13) — total 29 > 25.
	for i := 0; i < 10; i++ {
		if _, err := step.Handle(state, "+str"); err != nil {
			t.Fatalf("+str %d: %v", i, err)
		}
	}
	// str=18, dex=8 → spent 16. Now try to raise dex past budget.
	for i := 0; i < 5; i++ {
		if _, err := step.Handle(state, "+dex"); err != nil {
			t.Fatalf("+dex %d (under budget): %v", i, err)
		}
	}
	// 5 raises: dex went 8→13, cost 5. Total spent 16 + 5 = 21.
	// Raising dex to 14 costs 6 (+1), to 15 costs 8 (+2), to 16 (+2),
	// to 17 (+3) — over budget. Specifically dex=15 means spent 24,
	// dex=16 means spent 26 (over).
	if _, err := step.Handle(state, "+dex"); err != nil {
		t.Fatalf("dex 13→14 should fit: %v", err)
	}
	if _, err := step.Handle(state, "+dex"); err != nil {
		t.Fatalf("dex 14→15 should fit: %v", err)
	}
	// dex 15→16 puts spent at 26 — must reject.
	_, err := step.Handle(state, "+dex")
	if !IsValidationError(err) {
		t.Fatalf("over-budget raise: expected ValidationError, got %v", err)
	}
}

func TestPointBuyStep_Reset(t *testing.T) {
	step := chargen6()
	state := &State{Values: map[string]string{}}
	for i := 0; i < 3; i++ {
		_, _ = step.Handle(state, "+str")
	}
	if _, err := step.Handle(state, "reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	working := step.loadWorking(state)
	if working["str"] != 8 {
		t.Errorf("after reset, str = %d, want 8", working["str"])
	}
}

func TestPointBuyStep_DoneEnforcesBudget(t *testing.T) {
	step := chargen6()
	state := &State{Values: map[string]string{}}
	// done without spending anything → reject.
	_, err := step.Handle(state, "done")
	if !IsValidationError(err) {
		t.Fatalf("done with 0 spent: expected ValidationError, got %v", err)
	}
}

func TestPointBuyStep_DoneAdvancesOnExactBudget(t *testing.T) {
	step := chargen6()
	state := &State{Values: map[string]string{}}
	// Spend exactly 25: str=18 (16) + dex=14 (6) + con=11 (3) = 25.
	for i := 0; i < 10; i++ {
		_, _ = step.Handle(state, "+str")
	}
	for i := 0; i < 6; i++ {
		_, _ = step.Handle(state, "+dex")
	}
	for i := 0; i < 3; i++ {
		_, _ = step.Handle(state, "+con")
	}
	got, err := step.Handle(state, "done")
	if err != nil {
		t.Fatalf("done at exact budget: %v", err)
	}
	if got != "review" {
		t.Errorf("next = %q, want review", got)
	}
	// Stored JSON parses back to the chosen scores.
	var parsed map[string]int
	if err := json.Unmarshal([]byte(state.Values["abilities"]), &parsed); err != nil {
		t.Fatalf("stored JSON: %v", err)
	}
	if parsed["str"] != 18 || parsed["dex"] != 14 || parsed["con"] != 11 {
		t.Errorf("scores not committed correctly: %+v", parsed)
	}
}

func TestPointBuyStep_LoadWorking_FromCorruptBlob(t *testing.T) {
	step := chargen6()
	state := &State{Values: map[string]string{"abilities": "{not json"}}
	working := step.loadWorking(state)
	for _, k := range []string{"str", "dex", "con", "int", "wis", "cha"} {
		if working[k] != 8 {
			t.Errorf("corrupt blob: %s = %d, want 8", k, working[k])
		}
	}
}

func TestPointBuyStep_LoadWorking_RoundTripFromResume(t *testing.T) {
	step := chargen6()
	state := &State{Values: map[string]string{"abilities": `{"str":15,"dex":14}`}}
	working := step.loadWorking(state)
	if working["str"] != 15 || working["dex"] != 14 {
		t.Errorf("resume scores not applied: %+v", working)
	}
	// Items not in the blob default to Min.
	if working["con"] != 8 {
		t.Errorf("con default = %d, want 8", working["con"])
	}
}

func TestPointBuyStep_ValidateShape(t *testing.T) {
	step := &PointBuyStep{
		ID:      "x",
		Items:   []PointBuyItem{{Key: "a", Label: "A", Min: 0, Max: 2}},
		Costs:   []int{0, 1}, // wrong length; want 3
		Budget:  5,
		StoreAs: "x",
	}
	_, err := step.Handle(&State{Values: map[string]string{}}, "+a")
	if err == nil || !strings.Contains(err.Error(), "Costs length") {
		t.Fatalf("expected Costs length error, got %v", err)
	}
}

func TestPointBuyStep_UnknownCommand(t *testing.T) {
	step := chargen6()
	state := &State{Values: map[string]string{}}
	_, err := step.Handle(state, "garbage")
	if !IsValidationError(err) {
		t.Fatalf("unknown command: expected ValidationError, got %v", err)
	}
}
