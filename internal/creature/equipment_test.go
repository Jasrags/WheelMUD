package creature

import "testing"

func TestEquipmentSetGetClear(t *testing.T) {
	t.Parallel()
	var eq Equipment

	if got := eq.Get(SlotArmor); got != 0 {
		t.Fatalf("empty Get: want 0, got %d", got)
	}
	if displaced := eq.Set(SlotArmor, 42); displaced != 0 {
		t.Fatalf("first Set displaced: want 0, got %d", displaced)
	}
	if got := eq.Get(SlotArmor); got != 42 {
		t.Fatalf("after Set: want 42, got %d", got)
	}
	if displaced := eq.Set(SlotArmor, 99); displaced != 42 {
		t.Fatalf("second Set displaced: want 42, got %d", displaced)
	}
	if cleared := eq.Clear(SlotArmor); cleared != 99 {
		t.Fatalf("Clear: want 99, got %d", cleared)
	}
	if got := eq.Get(SlotArmor); got != 0 {
		t.Fatalf("after Clear: want 0, got %d", got)
	}
}

func TestEquipmentFindByItem(t *testing.T) {
	t.Parallel()
	eq := Equipment{Armor: 10, PrimaryWield: 20, OffHand: 30}

	cases := []struct {
		id   int64
		slot Slot
		ok   bool
	}{
		{10, SlotArmor, true},
		{20, SlotPrimaryWield, true},
		{30, SlotOffHand, true},
		{0, 0, false},  // never report the empty sentinel
		{99, 0, false}, // unknown id
	}
	for _, tc := range cases {
		got, ok := eq.FindByItem(tc.id)
		if ok != tc.ok || (ok && got != tc.slot) {
			t.Errorf("FindByItem(%d) = (%v, %v); want (%v, %v)", tc.id, got, ok, tc.slot, tc.ok)
		}
	}
}

func TestEquipmentClearItem(t *testing.T) {
	t.Parallel()
	eq := Equipment{PrimaryWield: 7}

	slot, ok := eq.ClearItem(7)
	if !ok || slot != SlotPrimaryWield {
		t.Fatalf("ClearItem(7) = (%v, %v); want (PrimaryWield, true)", slot, ok)
	}
	if eq.PrimaryWield != 0 {
		t.Fatalf("PrimaryWield not cleared: %d", eq.PrimaryWield)
	}
	// Idempotent: clearing again returns false.
	if _, ok := eq.ClearItem(7); ok {
		t.Fatalf("ClearItem(7) second call: want false")
	}
}

func TestSlotLabels(t *testing.T) {
	t.Parallel()
	// Every defined slot must produce a non-empty label and slot name
	// so the inventory annotator and `equipment` never render blanks.
	slots := []Slot{
		SlotArmor, SlotShield, SlotPrimaryWield, SlotOffHand,
		SlotOutfit, SlotCloak, SlotBackpack, SlotHeldInHand, SlotMount,
	}
	for _, s := range slots {
		if s.Label() == "" {
			t.Errorf("slot %d: empty Label()", s)
		}
		if s.SlotName() == "" {
			t.Errorf("slot %d: empty SlotName()", s)
		}
	}
}
