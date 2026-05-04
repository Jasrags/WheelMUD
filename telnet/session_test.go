package telnet

import (
	"context"
	"errors"
	"testing"
)

// failingEnterMode returns sentinelEnterErr from OnEnter and records whether
// OnExit ran (it should NOT, since the push was rolled back).
type failingEnterMode struct {
	exitCalled bool
}

var sentinelEnterErr = errors.New("on-enter failed")

func (m *failingEnterMode) Handle(_ context.Context, _ *Session, _ string) error { return nil }
func (m *failingEnterMode) Prompt(_ context.Context, _ *Session) string                             { return "" }
func (m *failingEnterMode) OnEnter(_ *Session) error                             { return sentinelEnterErr }
func (m *failingEnterMode) OnExit(_ *Session) error                              { m.exitCalled = true; return nil }

func TestPushMode_RollsBackOnOnEnterError(t *testing.T) {
	s, _ := newPipeSession(t)
	base := &scriptedMode{}
	if err := s.PushMode(base); err != nil {
		t.Fatalf("seed push: %v", err)
	}

	failing := &failingEnterMode{}
	err := s.PushMode(failing)
	if !errors.Is(err, sentinelEnterErr) {
		t.Fatalf("PushMode err = %v, want sentinelEnterErr", err)
	}
	if got := s.CurrentMode(); got != base {
		t.Fatalf("CurrentMode = %v, want base scripted mode (failing mode leaked onto stack)", got)
	}
	if failing.exitCalled {
		t.Fatal("OnExit was called for a mode whose push failed")
	}
}

func TestReplaceMode_RollsBackOnOnEnterError(t *testing.T) {
	s, _ := newPipeSession(t)
	base := &scriptedMode{}
	if err := s.PushMode(base); err != nil {
		t.Fatalf("seed push: %v", err)
	}

	failing := &failingEnterMode{}
	err := s.ReplaceMode(failing)
	if !errors.Is(err, sentinelEnterErr) {
		t.Fatalf("ReplaceMode err = %v, want sentinelEnterErr", err)
	}
	// ReplaceMode pops the prior mode before pushing; on OnEnter failure the
	// stack ends up empty, not retaining either mode.
	if got := s.CurrentMode(); got != nil {
		t.Fatalf("CurrentMode = %v, want nil (failing mode must not leak)", got)
	}
	if failing.exitCalled {
		t.Fatal("OnExit was called for a mode whose push failed")
	}
}
