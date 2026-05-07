package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

func TestFlee_RefusesWithoutFight(t *testing.T) {
	fx := newAttackFixture(t, false)
	_, alice, _, aOut, _ := commPair(t)
	c := NewFlee(fx.mgr)
	runCmd(t, c, alice, "")
	if !strings.Contains(aOut.String(), "aren't fighting") {
		t.Fatalf("expected refusal, got %q", aOut.String())
	}
	if fx.mgr.Active(alice.CurrentRoomID) {
		t.Fatal("flee outside fight must not start one")
	}
}

func TestFlee_QueuesActionFleeWhenFighting(t *testing.T) {
	fx := newAttackFixture(t, false)
	_, alice, _, aOut, _ := commPair(t)
	// Open a fight first via the attack verb so the queue plumbing is
	// the same as production.
	atk := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, fx.sessions, nil)
	runCmd(t, atk, alice, "trolloc")
	aOut.Reset()

	c := NewFlee(fx.mgr)
	runCmd(t, c, alice, "")

	if !strings.Contains(aOut.String(), "look for an opening") {
		t.Fatalf("missing self-echo: %q", aOut.String())
	}
	got, ok := fx.mgr.PendingAction(alice.CurrentRoomID,
		ActorRefForCharacter(alice.CharacterID))
	if !ok {
		t.Fatal("no queued action after flee")
	}
	if got.Kind != combat.ActionFlee {
		t.Fatalf("kind = %v, want ActionFlee", got.Kind)
	}
}

// TestFleeMover_NoExitsFails covers the no-eligible-exit failure
// path. Passes a room without any exits seeded and verifies the
// FleeMover returns Reason="no_exits".
func TestFleeMover_NoExitsFails(t *testing.T) {
	fx := newAttackFixture(t, false)
	exits := repo.NewMemoryExitRepo()
	mover := NewFleeMover(fx.rooms, exits, nil, fx.mobs, fx.characters, fx.sessions, nil, nil, nil)
	res := mover.AttemptFlee(context.Background(), 1,
		combat.ActorRef{Kind: combat.ActorKindMob, ID: fx.mob.ID})
	if res.Success {
		t.Fatalf("expected failure with no exits: %+v", res)
	}
	if res.Reason != "no_exits" {
		t.Fatalf("Reason = %q, want no_exits", res.Reason)
	}
}
