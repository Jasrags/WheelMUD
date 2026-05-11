package combat

import (
	"context"
	"math/rand"
	"sync"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// fumbleFixture seeds a character + mob in a room with the manager
// pinned to a nat-1 RNG seed. The character has a wielded weapon
// in `weaponID` (0 = unarmed). Returns the fired CombatMiss events.
func fumbleFixture(t *testing.T, equipWeapon bool) (
	chars *repo.MemoryCharacterRepo,
	items *repo.MemoryItemRepo,
	charID, weaponID, roomID int64,
	misses []CombatMiss,
) {
	t.Helper()
	ctx := context.Background()
	bus := eventbus.New()
	chars = repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})
	roomID = int64(7)

	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Alice",
		CurrentRoomID: roomID,
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50, Defense: 0, BAB: 0,
			Abilities: creature.Abilities{Str: creature.AbilityScore{Current: 10}},
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}
	charID = ch.ID

	items = repo.NewMemoryItemRepo()
	if equipWeapon {
		it, err := items.Create(ctx, repo.Item{
			ExternalID: "test.fumble.sword", Name: "a longsword",
			OwnerCharacterID: charID, Type: repo.ItemTypeWeapon,
			Stats: &repo.WeaponStats{Damage: "1d8", DamageType: []string{"S"}, Range: "melee"},
		})
		if err != nil {
			t.Fatalf("seed item: %v", err)
		}
		weaponID = it.ID
		eq := ch.Equipment
		eq.Set(creature.SlotPrimaryWield, weaponID)
		if err := chars.RecordEquipment(ctx, charID, eq); err != nil {
			t.Fatalf("seed equip: %v", err)
		}
	}

	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "dummy", ChallengeCode: 'A',
	})
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "dummy", HPCurrent: 100, HPMax: 100,
			Defense: 0, BAB: 0, CurrentRoomID: roomID,
		},
	})
	if err := mobs.UpdateRoom(ctx, mob.ID, roomID); err != nil {
		t.Fatalf("place mob: %v", err)
	}

	mgr := New(bus, chars, mobs, templates, items)

	var mu sync.Mutex
	eventbus.Subscribe[CombatMiss](bus, func(_ context.Context, ev CombatMiss) {
		mu.Lock()
		misses = append(misses, ev)
		mu.Unlock()
	})
	var hits []CombatHit
	eventbus.Subscribe[CombatHit](bus, func(_ context.Context, ev CombatHit) {
		mu.Lock()
		hits = append(hits, ev)
		mu.Unlock()
	})

	parts := []ActorRef{
		{Kind: ActorKindCharacter, ID: charID},
		{Kind: ActorKindMob, ID: mob.ID},
	}
	if _, err := mgr.Start(ctx, roomID, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Pin RNG AFTER Start so initiative rolls don't consume the seed.
	// The next d20 read will be the attack roll → nat-1.
	mgr.SetRNG(rand.New(rand.NewSource(stubSeedForRoll(1))))
	// Enqueue against the character ref directly (initiative order
	// might rearrange Order, but the action is keyed by ref).
	charRef := ActorRef{Kind: ActorKindCharacter, ID: charID}
	if err := mgr.EnqueueAction(roomID, charRef, Action{
		Kind: ActionAttack, Target: parts[1], WeaponID: weaponID,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Tick until the char has acted (initiative order may place it
	// later than the mob; mob has no queued action so its slot just
	// advances).
	for i := 0; i < 8; i++ {
		mgr.Tick(ctx)
		mu.Lock()
		acted := len(misses)+len(hits) > 0
		mu.Unlock()
		if acted {
			break
		}
		if !mgr.Active(roomID) {
			break
		}
		_ = mgr.EnqueueAction(roomID, charRef, Action{
			Kind: ActionAttack, Target: parts[1], WeaponID: weaponID,
		})
	}

	mu.Lock()
	defer mu.Unlock()
	return chars, items, charID, weaponID, roomID, append([]CombatMiss(nil), misses...)
}

func TestFumble_CharacterDropsWieldedWeapon(t *testing.T) {
	chars, items, charID, weaponID, roomID, misses := fumbleFixture(t, true)
	if len(misses) != 1 {
		t.Fatalf("CombatMiss = %d, want 1", len(misses))
	}
	ev := misses[0]
	if !ev.Fumble {
		t.Fatalf("Fumble = false, want true: %+v", ev)
	}
	if ev.WeaponDroppedID != weaponID {
		t.Errorf("WeaponDroppedID = %d, want %d", ev.WeaponDroppedID, weaponID)
	}

	// Equipment slot cleared.
	ch, err := chars.GetByID(context.Background(), charID)
	if err != nil {
		t.Fatalf("reload char: %v", err)
	}
	if got := ch.Equipment.Get(creature.SlotPrimaryWield); got != 0 {
		t.Errorf("SlotPrimaryWield = %d, want 0", got)
	}

	// Item now on the floor of the attacker's room.
	it, err := items.GetByID(context.Background(), weaponID)
	if err != nil {
		t.Fatalf("reload item: %v", err)
	}
	if it.OwnerCharacterID != 0 {
		t.Errorf("OwnerCharacterID = %d, want 0", it.OwnerCharacterID)
	}
	if it.RoomID != roomID {
		t.Errorf("RoomID = %d, want %d", it.RoomID, roomID)
	}
}

func TestFumble_UnarmedCharacter_NoDrop(t *testing.T) {
	_, _, _, _, _, misses := fumbleFixture(t, false)
	if len(misses) != 1 {
		t.Fatalf("CombatMiss = %d, want 1", len(misses))
	}
	ev := misses[0]
	if !ev.Fumble {
		t.Errorf("Fumble = false, want true: %+v", ev)
	}
	if ev.WeaponDroppedID != 0 {
		t.Errorf("WeaponDroppedID = %d, want 0", ev.WeaponDroppedID)
	}
}

func TestFumble_MobAttacker_NoSideEffect(t *testing.T) {
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})
	roomID := int64(7)

	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Bob",
		CurrentRoomID: roomID,
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50, Defense: 0, BAB: 0,
			Abilities: creature.Abilities{Str: creature.AbilityScore{Current: 10}},
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}

	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{ExternalID: "dummy", ChallengeCode: 'A'})
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "dummy", HPCurrent: 100, HPMax: 100,
			Defense: 0, BAB: 0, CurrentRoomID: roomID,
		},
	})
	if err := mobs.UpdateRoom(ctx, mob.ID, roomID); err != nil {
		t.Fatalf("place mob: %v", err)
	}
	items := repo.NewMemoryItemRepo()

	mgr := New(bus, chars, mobs, templates, items)

	var mu sync.Mutex
	var misses []CombatMiss
	var hits []CombatHit
	eventbus.Subscribe[CombatMiss](bus, func(_ context.Context, ev CombatMiss) {
		mu.Lock()
		misses = append(misses, ev)
		mu.Unlock()
	})
	eventbus.Subscribe[CombatHit](bus, func(_ context.Context, ev CombatHit) {
		mu.Lock()
		hits = append(hits, ev)
		mu.Unlock()
	})

	parts := []ActorRef{
		{Kind: ActorKindMob, ID: mob.ID},
		{Kind: ActorKindCharacter, ID: ch.ID},
	}
	if _, err := mgr.Start(ctx, roomID, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Pin RNG AFTER Start so the next d20 is the attack roll (nat-1).
	mgr.SetRNG(rand.New(rand.NewSource(stubSeedForRoll(1))))
	mobRef := ActorRef{Kind: ActorKindMob, ID: mob.ID}
	if err := mgr.EnqueueAction(roomID, mobRef, Action{
		Kind: ActionAttack, Target: parts[1],
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	for i := 0; i < 8; i++ {
		mgr.Tick(ctx)
		mu.Lock()
		acted := len(misses)+len(hits) > 0
		mu.Unlock()
		if acted {
			break
		}
		if !mgr.Active(roomID) {
			break
		}
		_ = mgr.EnqueueAction(roomID, mobRef, Action{Kind: ActionAttack, Target: parts[1]})
	}

	mu.Lock()
	defer mu.Unlock()
	if len(misses) != 1 {
		t.Fatalf("CombatMiss = %d, want 1", len(misses))
	}
	if !misses[0].Fumble {
		t.Errorf("Fumble = false, want true: %+v", misses[0])
	}
	if misses[0].WeaponDroppedID != 0 {
		t.Errorf("WeaponDroppedID = %d, want 0 (mob attacker)", misses[0].WeaponDroppedID)
	}
}
