package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

func TestParry_RefusesWithoutFight(t *testing.T) {
	fx := newAttackFixture(t, false)
	_, alice, _, aOut, _ := commPair(t)
	c := NewParry(fx.mgr, fx.characters, fx.sessions)
	runCmd(t, c, alice, "")
	if !strings.Contains(aOut.String(), "aren't fighting") {
		t.Fatalf("expected refusal, got %q", aOut.String())
	}
}

func TestParry_RefusesWithoutWeapon(t *testing.T) {
	fx := newAttackFixture(t, false)
	_, alice, _, aOut, _ := commPair(t)
	atk := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, fx.sessions)
	runCmd(t, atk, alice, "trolloc")
	aOut.Reset()

	c := NewParry(fx.mgr, fx.characters, fx.sessions)
	runCmd(t, c, alice, "")
	if !strings.Contains(aOut.String(), "need a weapon") {
		t.Fatalf("expected weapon refusal, got %q", aOut.String())
	}
}

func TestParry_QueuesActionParryWithWeapon(t *testing.T) {
	fx := newAttackFixture(t, false)
	sessions, alice, _, aOut, bOut := commPair(t)

	// Equip Alice with a weapon.
	items := repo.NewMemoryItemRepo()
	weapon, err := items.Create(context.Background(), repo.Item{
		ExternalID: "test-sword",
		Name:       "test sword",
		Type:       repo.ItemTypeWeapon,
		Stats:      &repo.WeaponStats{Damage: "1d6", DamageType: []string{"S"}},
	})
	if err != nil {
		t.Fatalf("create weapon: %v", err)
	}
	ch, err := fx.characters.FindByName(context.Background(), alice.CharacterName)
	if err != nil {
		t.Fatalf("find char: %v", err)
	}
	ch.Equipment.Set(creature.SlotPrimaryWield, weapon.ID)
	if err := fx.characters.RecordEquipment(context.Background(), ch.ID, ch.Equipment); err != nil {
		t.Fatalf("record equipment: %v", err)
	}

	// Open a fight via attack so Manager.Active(roomID) is true.
	atk := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, sessions)
	runCmd(t, atk, alice, "trolloc")
	aOut.Reset()
	bOut.Reset()

	c := NewParry(fx.mgr, fx.characters, sessions)
	runCmd(t, c, alice, "")

	if !strings.Contains(aOut.String(), "raise your weapon") {
		t.Fatalf("missing self-echo: %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "raises a weapon to parry") {
		t.Fatalf("missing peer broadcast: %q", bOut.String())
	}
	got, ok := fx.mgr.PendingAction(alice.CurrentRoomID,
		ActorRefForCharacter(alice.CharacterID))
	if !ok {
		t.Fatal("no queued action")
	}
	if got.Kind != combat.ActionParry {
		t.Fatalf("kind = %v, want ActionParry", got.Kind)
	}
	if got.WeaponID != weapon.ID {
		t.Fatalf("WeaponID = %d, want %d", got.WeaponID, weapon.ID)
	}
}
