package flow

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakePersister is a Persister stub that records every Save and
// Delete call. Optionally fails the next Save (or all Saves) so
// abort paths can be exercised.
type fakePersister struct {
	saves     []State    // deep-copied snapshots
	deletes   []deleteOp
	failSave  error
	failDelete error
}

type deleteOp struct {
	accountID int64
	flowID    string
}

func (p *fakePersister) Save(_ context.Context, s *State) error {
	if p.failSave != nil {
		return p.failSave
	}
	// Copy Values so we observe each call's snapshot, not the final
	// state pointer mutation.
	clone := *s
	clone.Values = map[string]string{}
	for k, v := range s.Values {
		clone.Values[k] = v
	}
	p.saves = append(p.saves, clone)
	return nil
}

func (p *fakePersister) Delete(_ context.Context, accountID int64, flowID string) error {
	if p.failDelete != nil {
		return p.failDelete
	}
	p.deletes = append(p.deletes, deleteOp{accountID, flowID})
	return nil
}

// twoStepFlow builds a minimal 2-step flow used across persist tests.
// step "one" stores input under "first" and advances to "two"; "two"
// stores input under "second" and completes.
func twoStepFlow(t *testing.T) *Flow {
	t.Helper()
	fl := &Flow{
		ID:    "wizdemo",
		Entry: "one",
		Steps: []Step{
			&TextStep{ID: "one", PromptText: "first?", StoreAs: "first", Next: "two"},
			&TextStep{ID: "two", PromptText: "second?", StoreAs: "second", Next: ""},
		},
	}
	if err := fl.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	return fl
}

func newPersistedRunner(t *testing.T, fl *Flow, accountID int64) (*Runner, *fakePersister, *BufferRenderer) {
	t.Helper()
	p := &fakePersister{}
	br := &BufferRenderer{}
	state := &State{FlowID: fl.ID, AccountID: accountID}
	r, err := NewRunner(fl, state, br, nil, nil)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	r.SetPersister(p)
	return r, p, br
}

func TestRunner_Persist_SaveOnStartAndEachTransition(t *testing.T) {
	fl := twoStepFlow(t)
	r, p, _ := newPersistedRunner(t, fl, 42)

	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(p.saves) != 1 {
		t.Fatalf("after Start saves=%d, want 1", len(p.saves))
	}
	if p.saves[0].Current != "one" {
		t.Errorf("after Start saved.Current=%q, want one", p.saves[0].Current)
	}
	if p.saves[0].StartedAt.IsZero() || p.saves[0].UpdatedAt.IsZero() {
		t.Errorf("StartedAt/UpdatedAt not stamped: %+v", p.saves[0])
	}

	done, err := r.Submit("alpha")
	if err != nil || done {
		t.Fatalf("submit one: done=%v err=%v", done, err)
	}
	if len(p.saves) != 2 {
		t.Fatalf("after first Submit saves=%d, want 2", len(p.saves))
	}
	if p.saves[1].Current != "two" {
		t.Errorf("after first Submit saved.Current=%q, want two", p.saves[1].Current)
	}
	if p.saves[1].Values["first"] != "alpha" {
		t.Errorf("first value not persisted: %+v", p.saves[1].Values)
	}
}

func TestRunner_Persist_DeleteOnCompletion(t *testing.T) {
	fl := twoStepFlow(t)
	r, p, _ := newPersistedRunner(t, fl, 42)

	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Submit("alpha"); err != nil {
		t.Fatal(err)
	}
	done, err := r.Submit("beta")
	if err != nil || !done {
		t.Fatalf("final submit: done=%v err=%v", done, err)
	}
	if len(p.deletes) != 1 {
		t.Fatalf("after completion deletes=%d, want 1", len(p.deletes))
	}
	if p.deletes[0].accountID != 42 || p.deletes[0].flowID != "wizdemo" {
		t.Errorf("delete op = %+v", p.deletes[0])
	}
}

func TestRunner_Persist_DeleteOnCancel(t *testing.T) {
	fl := twoStepFlow(t)
	r, p, _ := newPersistedRunner(t, fl, 42)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	r.Cancel()
	if len(p.deletes) != 1 {
		t.Fatalf("after Cancel deletes=%d, want 1", len(p.deletes))
	}
	// Idempotent — second Cancel should not double-delete.
	r.Cancel()
	if len(p.deletes) != 1 {
		t.Fatalf("after redundant Cancel deletes=%d, want still 1", len(p.deletes))
	}
}

func TestRunner_Persist_SkipsAnonymous(t *testing.T) {
	fl := twoStepFlow(t)
	r, p, _ := newPersistedRunner(t, fl, 0) // anonymous
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Submit("alpha"); err != nil {
		t.Fatal(err)
	}
	if done, err := r.Submit("beta"); err != nil || !done {
		t.Fatal(err)
	}
	if len(p.saves) != 0 {
		t.Fatalf("anonymous flow saved %d times, want 0", len(p.saves))
	}
	if len(p.deletes) != 0 {
		t.Fatalf("anonymous flow deleted %d times, want 0", len(p.deletes))
	}
}

func TestRunner_Persist_NilPersister(t *testing.T) {
	// No SetPersister: behave like §O.0.
	fl := twoStepFlow(t)
	br := &BufferRenderer{}
	r, err := NewRunner(fl, &State{FlowID: fl.ID, AccountID: 42}, br, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := r.Submit("alpha"); err != nil {
		t.Fatal(err)
	}
	if done, err := r.Submit("beta"); err != nil || !done {
		t.Fatal(err)
	}
}

func TestRunner_Persist_SaveErrorAbortsFlow(t *testing.T) {
	fl := twoStepFlow(t)
	r, p, _ := newPersistedRunner(t, fl, 42)
	sentinel := errors.New("disk full")
	p.failSave = sentinel
	err := r.Start()
	if err == nil || !errors.Is(err, sentinel) {
		t.Fatalf("Start save-fail = %v, want wrapping %v", err, sentinel)
	}
}

func TestRunner_Persist_DeleteErrorSwallowed(t *testing.T) {
	fl := twoStepFlow(t)
	r, p, _ := newPersistedRunner(t, fl, 42)
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	p.failDelete = errors.New("disk full")
	if _, err := r.Submit("alpha"); err != nil {
		t.Fatalf("submit one: %v", err)
	}
	done, err := r.Submit("beta")
	// Completion should succeed even though the delete fails: the
	// runner already committed the state machine, and a stale row
	// will get evicted next time the account hits cap.
	if err != nil || !done {
		t.Fatalf("completion swallowed: done=%v err=%v", done, err)
	}
}

func TestRunner_Resume_HydratedState(t *testing.T) {
	fl := twoStepFlow(t)
	br := &BufferRenderer{}
	// Hand-build a State as if loaded from the repo: paused at "two".
	state := &State{
		FlowID:    fl.ID,
		AccountID: 42,
		Current:   "two",
		Values:    map[string]string{"first": "alpha"},
		StartedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now(),
	}
	r, err := NewRunner(fl, state, br, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Resume(); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if br.String() != "second?" {
		t.Errorf("resume rendered %q, want \"second?\"", br.String())
	}
	// Submitting now should hit "two" and complete the flow.
	done, err := r.Submit("beta")
	if err != nil || !done {
		t.Fatalf("post-resume submit: done=%v err=%v", done, err)
	}
	if r.State().Values["second"] != "beta" {
		t.Errorf("post-resume value not stored: %+v", r.State().Values)
	}
}

func TestRunner_Resume_RejectsNonHydratedOrTerminated(t *testing.T) {
	fl := twoStepFlow(t)
	br := &BufferRenderer{}

	// Empty current → reject.
	r1, _ := NewRunner(fl, &State{FlowID: fl.ID, AccountID: 1}, br, nil, nil)
	if err := r1.Resume(); err == nil {
		t.Fatal("Resume on empty Current should error")
	}

	// Completed → reject.
	r2, _ := NewRunner(fl, &State{FlowID: fl.ID, AccountID: 1, Current: "two", Completed: true}, br, nil, nil)
	if err := r2.Resume(); err == nil {
		t.Fatal("Resume on Completed should error")
	}

	// Dangling step (catalog drift) → reject.
	r3, _ := NewRunner(fl, &State{FlowID: fl.ID, AccountID: 1, Current: "ghost"}, br, nil, nil)
	if err := r3.Resume(); err == nil {
		t.Fatal("Resume on unknown step should error")
	}
}
