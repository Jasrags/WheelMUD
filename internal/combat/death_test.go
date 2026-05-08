package combat

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

func TestXPValueForChallenge(t *testing.T) {
	cases := []struct {
		code creature.ChallengeCode
		want int64
	}{
		{'A', 100},
		{'B', 250},
		{'I', 38400},
		{0, 100},   // unknown defaults to A-tier
		{'Z', 100}, // out-of-range defaults to A-tier
		{'a', 100}, // lowercase is not in canon table; default
	}
	for _, tc := range cases {
		if got := xpValueForChallenge(tc.code); got != tc.want {
			t.Fatalf("code %q: got %d, want %d", tc.code, got, tc.want)
		}
	}
}

func TestAllocateXP_ProportionalSplit(t *testing.T) {
	a := ActorRef{Kind: ActorKindCharacter, ID: 1}
	b := ActorRef{Kind: ActorKindCharacter, ID: 2}
	tally := map[ActorRef]int32{
		a: 30,
		b: 10,
	}
	got := allocateXP(tally, 800, a)
	// 30/40 = 600, 10/40 = 200, no rounding remainder
	if got[a] != 600 || got[b] != 200 {
		t.Fatalf("split: got %v, want a=600 b=200", got)
	}
}

func TestAllocateXP_RemainderToKiller(t *testing.T) {
	a := ActorRef{Kind: ActorKindCharacter, ID: 1}
	b := ActorRef{Kind: ActorKindCharacter, ID: 2}
	tally := map[ActorRef]int32{a: 1, b: 2}
	// 100 * 1 / 3 = 33; 100 * 2 / 3 = 66; remainder 1 to killer (a).
	got := allocateXP(tally, 100, a)
	if got[a]+got[b] != 100 {
		t.Fatalf("totals don't sum to 100: %v", got)
	}
	if got[a] < 33 {
		t.Fatalf("killer should keep remainder: got %v", got)
	}
}

func TestAllocateXP_EmptyTallyAwardsKiller(t *testing.T) {
	killer := ActorRef{Kind: ActorKindCharacter, ID: 7}
	got := allocateXP(nil, 500, killer)
	if got[killer] != 500 {
		t.Fatalf("empty tally: got %v, want killer=500", got)
	}
}

func TestAllocateXP_ZeroTotalReturnsNil(t *testing.T) {
	if got := allocateXP(map[ActorRef]int32{{}: 5}, 0, ActorRef{}); got != nil {
		t.Fatalf("zero total: got %v, want nil", got)
	}
}

func TestPruneDead_RemovesAndClampsActiveIdx(t *testing.T) {
	a := ActorRef{Kind: ActorKindMob, ID: 1}
	b := ActorRef{Kind: ActorKindMob, ID: 2}
	c := ActorRef{Kind: ActorKindMob, ID: 3}
	f := &Fight{
		Order: []ActorEntry{
			{Ref: a, Initiative: 18},
			{Ref: b, Initiative: 15},
			{Ref: c, Initiative: 10},
		},
		ActiveIdx: 2,
		Dead:      map[ActorRef]struct{}{b: {}},
	}
	if !f.pruneDead() {
		t.Fatal("pruneDead should report a removal")
	}
	if len(f.Order) != 2 {
		t.Fatalf("len(Order) = %d, want 2", len(f.Order))
	}
	// b sat at index 1, before ActiveIdx=2; ActiveIdx walks back to 1.
	if f.ActiveIdx != 1 {
		t.Fatalf("ActiveIdx = %d, want 1", f.ActiveIdx)
	}
	if f.Order[f.ActiveIdx].Ref != c {
		t.Fatalf("active = %+v, want c", f.Order[f.ActiveIdx].Ref)
	}
	if f.Dead != nil {
		t.Fatalf("Dead should be cleared, got %v", f.Dead)
	}
}

// TestPruneDead_DeadHeadDoesNotSkipNewHead covers the regression
// where ActiveIdx=0 with the index-0 actor dead used to wrap forward
// to the second new actor, silently skipping the new head's first
// turn. ActiveIdx is parked at -1 so the round-advance lands on 0.
func TestPruneDead_DeadHeadDoesNotSkipNewHead(t *testing.T) {
	a := ActorRef{Kind: ActorKindMob, ID: 1}
	b := ActorRef{Kind: ActorKindMob, ID: 2}
	c := ActorRef{Kind: ActorKindMob, ID: 3}
	f := &Fight{
		Order: []ActorEntry{
			{Ref: a, Initiative: 18},
			{Ref: b, Initiative: 15},
			{Ref: c, Initiative: 10},
		},
		ActiveIdx: 0,
		Dead:      map[ActorRef]struct{}{a: {}},
	}
	f.pruneDead()
	if len(f.Order) != 2 {
		t.Fatalf("len(Order) = %d, want 2", len(f.Order))
	}
	// Round-advance simulation: (-1 + 1) % 2 = 0 → new head (b) acts.
	next := (f.ActiveIdx + 1) % len(f.Order)
	if f.Order[next].Ref != b {
		t.Fatalf("next active = %+v, want b (the new head)", f.Order[next].Ref)
	}
	// Active() must not panic with ActiveIdx parked at -1.
	if got := f.Active(); got != (ActorRef{}) {
		t.Fatalf("Active() with parked idx = %+v, want zero ref", got)
	}
}

func TestPruneDead_AllRemovedClampsToZero(t *testing.T) {
	a := ActorRef{Kind: ActorKindMob, ID: 1}
	f := &Fight{
		Order:     []ActorEntry{{Ref: a, Initiative: 10}},
		ActiveIdx: 0,
		Dead:      map[ActorRef]struct{}{a: {}},
	}
	f.pruneDead()
	if len(f.Order) != 0 {
		t.Fatalf("Order should be empty: %+v", f.Order)
	}
	if f.ActiveIdx != 0 {
		t.Fatalf("ActiveIdx = %d, want 0", f.ActiveIdx)
	}
}

// TestDeath_E2E exercises the full pipeline: a killing hit drops a
// mob's HP to zero, the death handler spawns a corpse, despawns the
// mob, and awards XP to the character attacker.
func TestDeath_E2E(t *testing.T) {
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})
	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Alice", CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50, BAB: 50, Defense: 10,
			Abilities: creature.Abilities{
				Str: creature.AbilityScore{Current: 18}, // +4 mod
			},
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}

	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID:    "trolloc-grunt",
		ChallengeCode: 'B', // 250 XP
	})

	mobs := repo.NewMemoryMobInstanceRepo()
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "trolloc", HPCurrent: 1, HPMax: 30, Defense: 0,
			CurrentRoomID: 1,
		},
	})
	if err := mobs.UpdateRoom(ctx, mob.ID, 1); err != nil {
		t.Fatalf("place mob: %v", err)
	}

	items := repo.NewMemoryItemRepo()
	mgr := New(bus, chars, mobs, templates, items)

	var (
		mu        sync.Mutex
		gotDeath  *CombatDeath
		gotEnded  *CombatEnded
		gotAwards []CombatXPAwarded
	)
	eventbus.Subscribe[CombatDeath](bus, func(_ context.Context, ev CombatDeath) {
		mu.Lock()
		gotDeath = &ev
		mu.Unlock()
	})
	eventbus.Subscribe[CombatEnded](bus, func(_ context.Context, ev CombatEnded) {
		mu.Lock()
		gotEnded = &ev
		mu.Unlock()
	})
	eventbus.Subscribe[CombatXPAwarded](bus, func(_ context.Context, ev CombatXPAwarded) {
		mu.Lock()
		gotAwards = append(gotAwards, ev)
		mu.Unlock()
	})

	parts := []ActorRef{
		{Kind: ActorKindCharacter, ID: ch.ID},
		{Kind: ActorKindMob, ID: mob.ID},
	}
	if _, err := mgr.Start(ctx, 1, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := mgr.EnqueueAction(1, parts[0], Action{
		Kind: ActionAttack, Target: parts[1],
	}); err != nil {
		t.Fatalf("EnqueueAction: %v", err)
	}

	// First Tick: the head of Order acts. With BAB=50 the attacker
	// will hit regardless of d20; the trolloc has 1 HP so any damage
	// kills it. If Alice is not the head (mob rolled higher
	// initiative), Tick once more — the mob has no queued action so
	// it's a no-op, then we re-queue Alice and tick again.
	for i := 0; i < 4 && mgr.Active(1); i++ {
		mgr.Tick(ctx)
		// If mob dies, the next Tick prunes; bail when the fight ends.
		if !mgr.Active(1) {
			break
		}
		// Re-queue Alice's attack each round so we get her swing in.
		_ = mgr.EnqueueAction(1, parts[0], Action{
			Kind: ActionAttack, Target: parts[1],
		})
	}

	// Mob deleted from the world.
	if _, err := mobs.GetByID(ctx, mob.ID); !errors.Is(err, repo.ErrInstanceNotFound) {
		t.Fatalf("mob still alive: err=%v", err)
	}

	// Corpse spawned in room 1.
	roomItems, err := items.ListInRoom(ctx, 1)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	foundCorpse := false
	for _, it := range roomItems {
		if it.Type == repo.ItemTypeContainer && it.Name == "corpse of trolloc" {
			foundCorpse = true
			break
		}
	}
	if !foundCorpse {
		t.Fatalf("no corpse in room: %+v", roomItems)
	}

	// XP credited to Alice (250 — full B-tier reward, killer gets it
	// all because she's the only attacker in the tally).
	got, _ := chars.GetByID(ctx, ch.ID)
	if got.XP != 250 {
		t.Fatalf("Alice XP = %d, want 250", got.XP)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotDeath == nil {
		t.Fatal("CombatDeath not published")
	}
	if gotDeath.Victim.ID != mob.ID {
		t.Fatalf("Victim = %+v, want mob %d", gotDeath.Victim, mob.ID)
	}
	if len(gotAwards) != 1 || gotAwards[0].Awardee.ID != ch.ID || gotAwards[0].Amount != 250 {
		t.Fatalf("awards = %+v, want one to Alice for 250", gotAwards)
	}
	if gotEnded == nil {
		t.Fatal("CombatEnded not published after sole-mob death pruned Order")
	}
	if gotEnded.Reason != ReasonNoParticipants {
		t.Fatalf("CombatEnded reason = %q, want %q",
			gotEnded.Reason, ReasonNoParticipants)
	}
}

// killTrollocWithLoot runs the full kill pipeline using the same
// shape as TestDeath_E2E but accepts a template + a list of items to
// pre-seed as the mob's carried inventory. The mob.Inventory id list
// is built from the actual Create-returned ids so a future change in
// id-allocation order doesn't silently invalidate the assertion.
func killTrollocWithLoot(t *testing.T, tmpl creature.MobTemplate,
	preseedItems []repo.Item,
) (*repo.MemoryItemRepo, *repo.MemoryCharacterRepo, int64, []CombatXPAwarded) {
	t.Helper()
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})
	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Alice", CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50, BAB: 50, Defense: 10,
			Abilities: creature.Abilities{
				Str: creature.AbilityScore{Current: 18},
			},
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}

	templates := repo.NewMemoryMobTemplateRepo()
	tmpl.ExternalID = "trolloc-grunt"
	created, err := templates.Create(ctx, tmpl)
	if err != nil {
		t.Fatalf("seed template: %v", err)
	}

	items := repo.NewMemoryItemRepo()
	mobInv := make([]int64, 0, len(preseedItems))
	for _, it := range preseedItems {
		got, err := items.Create(ctx, it)
		if err != nil {
			t.Fatalf("seed item %q: %v", it.ExternalID, err)
		}
		mobInv = append(mobInv, got.ID)
	}

	mobs := repo.NewMemoryMobInstanceRepo()
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: created.ID,
		Inventory:  mobInv,
		Core: creature.Core{
			Name: "trolloc", HPCurrent: 1, HPMax: 30, Defense: 0,
			CurrentRoomID: 1,
		},
	})
	if err := mobs.UpdateRoom(ctx, mob.ID, 1); err != nil {
		t.Fatalf("place mob: %v", err)
	}

	mgr := New(bus, chars, mobs, templates, items)
	var awards []CombatXPAwarded
	var amu sync.Mutex
	eventbus.Subscribe[CombatXPAwarded](bus, func(_ context.Context, ev CombatXPAwarded) {
		amu.Lock()
		awards = append(awards, ev)
		amu.Unlock()
	})

	parts := []ActorRef{
		{Kind: ActorKindCharacter, ID: ch.ID},
		{Kind: ActorKindMob, ID: mob.ID},
	}
	if _, err := mgr.Start(ctx, 1, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := mgr.EnqueueAction(1, parts[0], Action{
		Kind: ActionAttack, Target: parts[1],
	}); err != nil {
		t.Fatalf("EnqueueAction: %v", err)
	}
	for i := 0; i < 4 && mgr.Active(1); i++ {
		mgr.Tick(ctx)
		if !mgr.Active(1) {
			break
		}
		_ = mgr.EnqueueAction(1, parts[0], Action{
			Kind: ActionAttack, Target: parts[1],
		})
	}

	amu.Lock()
	out := append([]CombatXPAwarded(nil), awards...)
	amu.Unlock()
	return items, chars, ch.ID, out
}

// findCorpseID returns the id of the corpse container in the given
// room. Fails the test if not found.
func findCorpseID(t *testing.T, items *repo.MemoryItemRepo, roomID int64) int64 {
	t.Helper()
	all, err := items.ListInRoom(context.Background(), roomID)
	if err != nil {
		t.Fatalf("list room: %v", err)
	}
	for _, it := range all {
		if it.Type == repo.ItemTypeContainer {
			return it.ID
		}
	}
	t.Fatalf("no corpse in room %d: %+v", roomID, all)
	return 0
}

func TestDeath_TransfersInventoryToCorpse(t *testing.T) {
	// Two unattached items pre-created; mob.Inventory points at them
	// by id. After the kill, both should be parent_item_id = corpse.ID.
	preseed := []repo.Item{
		{ExternalID: "loot-helm", Name: "a rusted helm", Type: repo.ItemTypeClothing},
		{ExternalID: "loot-ring", Name: "a copper ring", Type: repo.ItemTypeClothing},
	}
	items, _, _, _ := killTrollocWithLoot(t,
		creature.MobTemplate{ChallengeCode: 'A'},
		preseed,
	)
	corpseID := findCorpseID(t, items, 1)
	kids, err := items.ListInContainer(context.Background(), corpseID)
	if err != nil {
		t.Fatalf("list container: %v", err)
	}
	if len(kids) != 2 {
		t.Fatalf("corpse children = %d, want 2: %+v", len(kids), kids)
	}
	for _, k := range kids {
		if k.ParentItemID != corpseID {
			t.Errorf("item %d ParentItemID = %d, want %d", k.ID, k.ParentItemID, corpseID)
		}
		if k.RoomID != 0 || k.OwnerCharacterID != 0 {
			t.Errorf("item %d location not exclusive: %+v", k.ID, k)
		}
	}
}

func TestDeath_RollsGoldDiceIntoCorpse(t *testing.T) {
	items, _, _, _ := killTrollocWithLoot(t,
		creature.MobTemplate{ChallengeCode: 'A', GoldDice: "2d4"},
		nil,
	)
	corpseID := findCorpseID(t, items, 1)
	kids, _ := items.ListInContainer(context.Background(), corpseID)
	if len(kids) != 1 {
		t.Fatalf("corpse children = %d, want 1 coin pile: %+v", len(kids), kids)
	}
	pile := kids[0]
	if pile.Type != repo.ItemTypeTradeGood {
		t.Errorf("pile.Type = %q, want trade_good", pile.Type)
	}
	if pile.Name != "a small pile of coins" {
		t.Errorf("pile.Name = %q", pile.Name)
	}
	// 2d4 → range [2, 8].
	if pile.Value < 2 || pile.Value > 8 {
		t.Errorf("pile.Value = %d, want [2..8]", pile.Value)
	}
	if pile.Flags&repo.FlagTradeGood == 0 {
		t.Errorf("pile missing FlagTradeGood: %v", pile.Flags)
	}
}

func TestDeath_NoCoinPileWhenGoldDiceEmpty(t *testing.T) {
	items, _, _, _ := killTrollocWithLoot(t,
		creature.MobTemplate{ChallengeCode: 'A'},
		nil,
	)
	corpseID := findCorpseID(t, items, 1)
	kids, _ := items.ListInContainer(context.Background(), corpseID)
	if len(kids) != 0 {
		t.Errorf("empty GoldDice still spawned children: %+v", kids)
	}
}

func TestDeath_XPValueOverridesChallengeCode(t *testing.T) {
	// XPValue = 777 must beat ChallengeCode 'A' (which would have
	// returned 100).
	_, chars, charID, awards := killTrollocWithLoot(t,
		creature.MobTemplate{ChallengeCode: 'A', XPValue: 777},
		nil,
	)
	got, _ := chars.GetByID(context.Background(), charID)
	if got.XP != 777 {
		t.Errorf("Alice XP = %d, want 777 (XPValue override)", got.XP)
	}
	if len(awards) != 1 || awards[0].Amount != 777 {
		t.Errorf("awards = %+v, want one for 777", awards)
	}
}

func TestDeath_XPValueZeroFallsBackToChallengeCode(t *testing.T) {
	// XPValue = 0 (unset) must use the old ChallengeCode table —
	// here ChallengeCode 'C' = 600.
	_, chars, charID, _ := killTrollocWithLoot(t,
		creature.MobTemplate{ChallengeCode: 'C', XPValue: 0},
		nil,
	)
	got, _ := chars.GetByID(context.Background(), charID)
	if got.XP != 600 {
		t.Errorf("Alice XP = %d, want 600 (ChallengeCode 'C' fallback)", got.XP)
	}
}
