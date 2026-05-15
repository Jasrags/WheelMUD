package combat

import (
	"context"
	"math/rand"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/progression"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// killCharacterByMob runs the full mob-attacks-player flow until the
// character's HP hits 0 and the death pipeline fires. Returns the
// repos and event captures so individual tests can assert the
// outcomes they care about.
func killCharacterByMob(t *testing.T, charXP int64, charXPDebt int64,
	deathRoom, boundRoom int64,
) (
	chars *repo.MemoryCharacterRepo,
	mgr *Manager,
	charID, mobID int64,
	deaths []CharacterDied,
	respawns []CharacterRespawned,
) {
	t.Helper()
	ctx := context.Background()
	bus := eventbus.New()
	chars = repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "owner", PasswordHash: "h"})
	ch, err := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Alice",
		CurrentRoomID: deathRoom, BoundRoomID: boundRoom,
		XP: charXP, XPDebt: charXPDebt,
		Core: creature.Core{
			HPCurrent: 1, HPMax: 50, Defense: 0, BAB: 0,
			Abilities: creature.Abilities{
				Str: creature.AbilityScore{Current: 10},
			},
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}
	charID = ch.ID

	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "trolloc-grunt", ChallengeCode: 'A',
	})
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "trolloc", HPCurrent: 100, HPMax: 100,
			Defense: 0, BAB: 50,
			Abilities: creature.Abilities{
				Str: creature.AbilityScore{Current: 18},
			},
			CurrentRoomID: deathRoom,
		},
	})
	if err := mobs.UpdateRoom(ctx, mob.ID, deathRoom); err != nil {
		t.Fatalf("place mob: %v", err)
	}
	mobID = mob.ID

	items := repo.NewMemoryItemRepo()
	mgr = New(bus, chars, mobs, templates, items)
	// Pin the RNG so the kill-swing roll is deterministic — without
	// this, a nat-1 on the first swing leaves the character alive and
	// every subsequent Tick is blocked behind NextActAt (real wall
	// clock is unmocked here). CI surfaced this as a 1-in-20 flake.
	mgr.SetRNG(rand.New(rand.NewSource(1)))

	var dmu sync.Mutex
	eventbus.Subscribe[CharacterDied](bus, func(_ context.Context, ev CharacterDied) {
		dmu.Lock()
		deaths = append(deaths, ev)
		dmu.Unlock()
	})
	eventbus.Subscribe[CharacterRespawned](bus, func(_ context.Context, ev CharacterRespawned) {
		dmu.Lock()
		respawns = append(respawns, ev)
		dmu.Unlock()
	})

	parts := []ActorRef{
		{Kind: ActorKindCharacter, ID: ch.ID},
		{Kind: ActorKindMob, ID: mob.ID},
	}
	if _, err := mgr.Start(ctx, deathRoom, parts); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := mgr.EnqueueAction(deathRoom, parts[1], Action{
		Kind: ActionAttack, Target: parts[0],
	}); err != nil {
		t.Fatalf("EnqueueAction: %v", err)
	}
	// Tick until the death pipeline fires (CharacterDied published)
	// then one more tick so pruneDead clears Order. Bailing on the
	// first death keeps subsequent ticks from re-attacking the
	// already-respawned character if the test harness leaves a
	// stale enqueue lying around.
	for i := 0; i < 8; i++ {
		mgr.Tick(ctx)
		dmu.Lock()
		fired := len(deaths) > 0
		dmu.Unlock()
		if fired {
			break
		}
		if !mgr.Active(deathRoom) {
			break
		}
		_ = mgr.EnqueueAction(deathRoom, parts[1], Action{
			Kind: ActionAttack, Target: parts[0],
		})
	}
	// One extra tick to fire pruneDead so Fight.Dead translates into
	// an Order removal. The mob doesn't have a queued action so this
	// is a no-op aside from the prune sweep.
	if mgr.Active(deathRoom) {
		mgr.Tick(ctx)
	}
	dmu.Lock()
	defer dmu.Unlock()
	out := append([]CharacterDied(nil), deaths...)
	rsp := append([]CharacterRespawned(nil), respawns...)
	return chars, mgr, charID, mobID, out, rsp
}

func TestCharacterDeath_E2E(t *testing.T) {
	// Alice at XP=7000 → LevelForXP(7000)=4 (XPForLevel(4)=6000 ≤
	// 7000 < XPForLevel(5)=10000); xp-into-level = 1000 → debt = 100.
	startXP := progression.XPForLevel(4) + 1000
	chars, mgr, charID, _, deaths, respawns := killCharacterByMob(t,
		startXP, 0, 1, 99)

	if len(deaths) != 1 {
		t.Fatalf("CharacterDied = %d, want 1: %+v", len(deaths), deaths)
	}
	if len(respawns) != 1 {
		t.Fatalf("CharacterRespawned = %d, want 1", len(respawns))
	}
	if deaths[0].DeathRoomID != 1 || deaths[0].BoundRoomID != 99 {
		t.Fatalf("CharacterDied rooms = %d/%d, want 1/99",
			deaths[0].DeathRoomID, deaths[0].BoundRoomID)
	}
	if deaths[0].XPDebtAdded != 100 {
		t.Errorf("XPDebtAdded = %d, want 100", deaths[0].XPDebtAdded)
	}
	if respawns[0].PrevRoomID != 1 || respawns[0].RoomID != 99 {
		t.Errorf("CharacterRespawned rooms = %d/%d, want 1/99",
			respawns[0].PrevRoomID, respawns[0].RoomID)
	}

	got, err := chars.GetByID(context.Background(), charID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.CurrentRoomID != 99 {
		t.Errorf("CurrentRoomID = %d, want 99 (bound room)", got.CurrentRoomID)
	}
	if got.Core.HPCurrent != got.Core.HPMax {
		t.Errorf("HPCurrent = %d, want HPMax %d", got.Core.HPCurrent, got.Core.HPMax)
	}
	if got.Core.Conditions&creature.CondDying != 0 {
		t.Errorf("CondDying not cleared on respawn")
	}
	if got.Core.Conditions&creature.CondUnconscious != 0 {
		t.Errorf("CondUnconscious not cleared on respawn")
	}
	if got.XPDebt != 100 {
		t.Errorf("XPDebt = %d, want 100", got.XPDebt)
	}

	// pruneDead must have removed the dead character from the
	// fight's Order so they can't be re-targeted on subsequent
	// ticks. (The killCharacterByMob helper runs an extra tick
	// after the death event for exactly this prune sweep.)
	victimRef := ActorRef{Kind: ActorKindCharacter, ID: charID}
	if f, ok := mgr.Get(1); ok {
		for _, e := range f.Order {
			if e.Ref == victimRef {
				t.Errorf("dead character still in Fight.Order: %+v", e)
			}
		}
	}
}

func TestCharacterDeath_BoundRoomDefaultsToStarterRoom(t *testing.T) {
	// `BoundRoomID = 0` is normalized by `MemoryCharacterRepo.Create`
	// (and the sqlite Create) to `StarterRoomID = 1`, so a chargen
	// commit that didn't explicitly set the bound room still gets
	// a working respawn target. Verify the respawn lands at 1.
	chars, _, charID, _, _, respawns := killCharacterByMob(t,
		1000, 0, 7, 0)
	if len(respawns) != 1 || respawns[0].RoomID != repo.StarterRoomID {
		t.Fatalf("respawn room = %+v, want StarterRoomID(%d)",
			respawns, repo.StarterRoomID)
	}
	got, _ := chars.GetByID(context.Background(), charID)
	if got.CurrentRoomID != repo.StarterRoomID {
		t.Errorf("CurrentRoomID = %d, want %d (starter)",
			got.CurrentRoomID, repo.StarterRoomID)
	}
}

func TestDebt_OffsetsNextMobKillAward(t *testing.T) {
	// Seed Alice with XPDebt=200; kill a B-tier mob (250 XP).
	// 200 should drain to debt, 50 to her XP. CombatXPAwarded
	// should report Amount=50 + DebtTaken=200.
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "o", PasswordHash: "h"})
	ch, _ := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Alice", CurrentRoomID: 1,
		XPDebt: 200,
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50, BAB: 50, Defense: 10,
			Abilities: creature.Abilities{Str: creature.AbilityScore{Current: 18}},
		},
	})
	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "trolloc", ChallengeCode: 'B', // 250 XP
	})
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "trolloc", HPCurrent: 1, HPMax: 30, Defense: 0,
			CurrentRoomID: 1,
		},
	})
	_ = mobs.UpdateRoom(ctx, mob.ID, 1)
	items := repo.NewMemoryItemRepo()
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
	_ = mgr.EnqueueAction(1, parts[0], Action{Kind: ActionAttack, Target: parts[1]})
	for i := 0; i < 4 && mgr.Active(1); i++ {
		mgr.Tick(ctx)
		if !mgr.Active(1) {
			break
		}
		_ = mgr.EnqueueAction(1, parts[0], Action{Kind: ActionAttack, Target: parts[1]})
	}

	got, _ := chars.GetByID(ctx, ch.ID)
	if got.XP != 50 {
		t.Errorf("XP = %d, want 50 (250 award - 200 debt)", got.XP)
	}
	if got.XPDebt != 0 {
		t.Errorf("XPDebt = %d, want 0 (drained)", got.XPDebt)
	}
	amu.Lock()
	defer amu.Unlock()
	if len(awards) != 1 {
		t.Fatalf("awards = %d, want 1: %+v", len(awards), awards)
	}
	if awards[0].Amount != 50 || awards[0].DebtTaken != 200 {
		t.Errorf("award = {Amount:%d DebtTaken:%d}, want {50 200}",
			awards[0].Amount, awards[0].DebtTaken)
	}
}

func TestDebt_FullAwardWhenNoDebt(t *testing.T) {
	// Regression guard: zero-debt path should report DebtTaken=0
	// and credit the full gross share. Mirrors the original
	// TestDeath_E2E path that was passing before §19 player-death.
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "o", PasswordHash: "h"})
	ch, _ := chars.Create(ctx, repo.Character{
		AccountID: acc.ID, Name: "Alice", CurrentRoomID: 1,
		Core: creature.Core{
			HPCurrent: 50, HPMax: 50, BAB: 50, Defense: 10,
			Abilities: creature.Abilities{Str: creature.AbilityScore{Current: 18}},
		},
	})
	templates := repo.NewMemoryMobTemplateRepo()
	tmpl, _ := templates.Create(ctx, creature.MobTemplate{
		ExternalID: "trolloc", ChallengeCode: 'A',
	})
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, _ := mobs.Create(ctx, creature.MobInstance{
		TemplateID: tmpl.ID,
		Core: creature.Core{
			Name: "trolloc", HPCurrent: 1, HPMax: 30, Defense: 0,
			CurrentRoomID: 1,
		},
	})
	_ = mobs.UpdateRoom(ctx, mob.ID, 1)
	items := repo.NewMemoryItemRepo()
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
	_ = mgr.EnqueueAction(1, parts[0], Action{Kind: ActionAttack, Target: parts[1]})
	for i := 0; i < 4 && mgr.Active(1); i++ {
		mgr.Tick(ctx)
		if !mgr.Active(1) {
			break
		}
		_ = mgr.EnqueueAction(1, parts[0], Action{Kind: ActionAttack, Target: parts[1]})
	}

	got, _ := chars.GetByID(ctx, ch.ID)
	if got.XP != 100 {
		t.Errorf("XP = %d, want 100 (A-tier full)", got.XP)
	}
	amu.Lock()
	defer amu.Unlock()
	if len(awards) != 1 || awards[0].Amount != 100 || awards[0].DebtTaken != 0 {
		t.Errorf("award = %+v, want {Amount:100 DebtTaken:0}", awards)
	}
}

// pvpDeathSetup wires up two characters (attacker, victim) at the
// requested levels, both in roomID=1, builds a Manager, opens a Fight
// with the supplied DamageTally pre-seeded, and subscribes to
// CombatXPAwarded events. The test then drives the pipeline by
// calling mgr.handleCharacterDeath directly — this skips the full
// combat tick (no BAB/RNG/HP math) so the death-side wiring is
// isolated from the resolveAction surface.
//
// XP for each character is set from progression.XPForLevel(level) so
// LevelForXP(ch.XP) reads back the requested level exactly.
type pvpDeathFixture struct {
	chars             *repo.MemoryCharacterRepo
	mgr               *Manager
	attacker, victim  ActorRef
	awards            *[]CombatXPAwarded
	awardsMu          *sync.Mutex
}

func pvpDeathSetup(t *testing.T, attackerLevel, victimLevel int, victimXPDebt int64, tally map[int64]int32) pvpDeathFixture {
	t.Helper()
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	accs := repo.NewMemoryAccountRepo()

	accA, _ := accs.Create(ctx, repo.Account{Username: "alice-acc", PasswordHash: "h"})
	accB, _ := accs.Create(ctx, repo.Account{Username: "bob-acc", PasswordHash: "h"})

	alice, err := chars.Create(ctx, repo.Character{
		AccountID: accA.ID, Name: "Alice", CurrentRoomID: 1, BoundRoomID: 1,
		XP: progression.XPForLevel(attackerLevel),
		Core: creature.Core{HPCurrent: 50, HPMax: 50},
	})
	if err != nil {
		t.Fatalf("seed alice: %v", err)
	}
	bob, err := chars.Create(ctx, repo.Character{
		AccountID: accB.ID, Name: "Bob", CurrentRoomID: 1, BoundRoomID: 1,
		XP: progression.XPForLevel(victimLevel), XPDebt: victimXPDebt,
		Core: creature.Core{HPCurrent: 1, HPMax: 50},
	})
	if err != nil {
		t.Fatalf("seed bob: %v", err)
	}

	mobs := repo.NewMemoryMobInstanceRepo()
	templates := repo.NewMemoryMobTemplateRepo()
	items := repo.NewMemoryItemRepo()
	mgr := New(bus, chars, mobs, templates, items)

	attacker := ActorRef{Kind: ActorKindCharacter, ID: alice.ID}
	victim := ActorRef{Kind: ActorKindCharacter, ID: bob.ID}
	if _, err := mgr.Start(ctx, 1, []ActorRef{attacker, victim}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Seed the fight's DamageTally directly. Tests that want the
	// default (attacker = sole damage source) pass nil and we stamp
	// a single-entry tally; tests that want richer shapes (self-
	// damage, secondary attackers) pass an explicit map keyed by
	// character ID.
	mgr.mu.Lock()
	f := mgr.fights[1]
	if f.DamageTally == nil {
		f.DamageTally = make(map[ActorRef]int32)
	}
	if tally == nil {
		f.DamageTally[attacker] = 100
	} else {
		for id, dmg := range tally {
			f.DamageTally[ActorRef{Kind: ActorKindCharacter, ID: id}] = dmg
		}
	}
	mgr.mu.Unlock()

	awards := &[]CombatXPAwarded{}
	awardsMu := &sync.Mutex{}
	eventbus.Subscribe[CombatXPAwarded](bus, func(_ context.Context, ev CombatXPAwarded) {
		awardsMu.Lock()
		*awards = append(*awards, ev)
		awardsMu.Unlock()
	})

	return pvpDeathFixture{
		chars:    chars,
		mgr:      mgr,
		attacker: attacker,
		victim:   victim,
		awards:   awards,
		awardsMu: awardsMu,
	}
}

func TestCharacterDeath_PvP_AwardsAttackerXP(t *testing.T) {
	f := pvpDeathSetup(t, 12, 12, 0, nil)
	f.mgr.handleCharacterDeath(context.Background(), f.attacker, f.victim)

	want := int64(PvPXPPerVictimLevel * 12) // 50 * 12 = 600
	got, _ := f.chars.GetByID(context.Background(), f.attacker.ID)
	expectedXP := progression.XPForLevel(12) + want
	if got.XP != expectedXP {
		t.Errorf("attacker.XP = %d, want %d", got.XP, expectedXP)
	}

	f.awardsMu.Lock()
	defer f.awardsMu.Unlock()
	if len(*f.awards) != 1 {
		t.Fatalf("CombatXPAwarded count = %d, want 1: %+v", len(*f.awards), *f.awards)
	}
	ev := (*f.awards)[0]
	if ev.Amount != want || ev.DebtTaken != 0 {
		t.Errorf("event = %+v, want Amount=%d DebtTaken=0", ev, want)
	}
	if ev.Awardee != f.attacker || ev.Killed != f.victim {
		t.Errorf("event refs = %+v, want awardee=%+v killed=%+v", ev, f.attacker, f.victim)
	}
}

func TestCharacterDeath_PvP_LevelDiffSkipsAward(t *testing.T) {
	// Attacker level 20 vs victim level 10: diff = 10 > PvPLevelDiffCap.
	f := pvpDeathSetup(t, 20, 10, 0, nil)
	beforeXP := progression.XPForLevel(20)

	f.mgr.handleCharacterDeath(context.Background(), f.attacker, f.victim)

	got, _ := f.chars.GetByID(context.Background(), f.attacker.ID)
	if got.XP != beforeXP {
		t.Errorf("attacker.XP = %d, want %d (no award)", got.XP, beforeXP)
	}
	f.awardsMu.Lock()
	defer f.awardsMu.Unlock()
	if len(*f.awards) != 0 {
		t.Errorf("CombatXPAwarded fired despite level diff: %+v", *f.awards)
	}
}

func TestCharacterDeath_PvP_XPDebtDrains(t *testing.T) {
	// Attacker has XPDebt=400; level-12 kill yields totalXP=600.
	// 400 drains to debt, 200 credits to XP.
	f := pvpDeathSetup(t, 12, 12, 0, nil)
	ctx := context.Background()
	// Set attacker's debt directly via the repo's RecordXPDebt.
	if err := f.chars.RecordXPDebt(ctx, f.attacker.ID, 400); err != nil {
		t.Fatalf("seed debt: %v", err)
	}

	f.mgr.handleCharacterDeath(ctx, f.attacker, f.victim)

	got, _ := f.chars.GetByID(ctx, f.attacker.ID)
	wantGain := int64(200)
	wantXP := progression.XPForLevel(12) + wantGain
	if got.XP != wantXP {
		t.Errorf("attacker.XP = %d, want %d", got.XP, wantXP)
	}
	if got.XPDebt != 0 {
		t.Errorf("attacker.XPDebt = %d, want 0 (fully drained)", got.XPDebt)
	}

	f.awardsMu.Lock()
	defer f.awardsMu.Unlock()
	if len(*f.awards) != 1 {
		t.Fatalf("award count = %d, want 1", len(*f.awards))
	}
	ev := (*f.awards)[0]
	if ev.Amount != wantGain || ev.DebtTaken != 400 {
		t.Errorf("event = %+v, want Amount=200 DebtTaken=400", ev)
	}
}

func TestCharacterDeath_PvP_NonCombatDeathSkipsAward(t *testing.T) {
	f := pvpDeathSetup(t, 12, 12, 0, nil)

	// HandleAffectDeath passes ActorRef{} as killer — non-combat death
	// (DoT, environmental). Must not award XP regardless of tally.
	f.mgr.HandleAffectDeath(context.Background(), f.victim.ID)

	got, _ := f.chars.GetByID(context.Background(), f.attacker.ID)
	if got.XP != progression.XPForLevel(12) {
		t.Errorf("attacker.XP changed on non-combat death: got %d", got.XP)
	}
	f.awardsMu.Lock()
	defer f.awardsMu.Unlock()
	if len(*f.awards) != 0 {
		t.Errorf("CombatXPAwarded fired on non-combat death: %+v", *f.awards)
	}
}

func TestCharacterDeath_PvP_VictimNotInTally(t *testing.T) {
	// Seed a tally where the victim self-damaged (10) and the
	// attacker dealt the rest (50). Victim's contribution must be
	// stripped before allocateXP, so attacker gets the full 600.
	f := pvpDeathSetup(t, 12, 12, 0, nil)
	ctx := context.Background()
	f.mgr.mu.Lock()
	f.mgr.fights[1].DamageTally = map[ActorRef]int32{
		f.attacker: 50,
		f.victim:   10,
	}
	f.mgr.mu.Unlock()

	f.mgr.handleCharacterDeath(ctx, f.attacker, f.victim)

	got, _ := f.chars.GetByID(ctx, f.attacker.ID)
	want := progression.XPForLevel(12) + 600
	if got.XP != want {
		t.Errorf("attacker.XP = %d, want %d (full pool, victim share stripped)", got.XP, want)
	}
	// Victim's row must be unchanged aside from the existing death
	// pipeline (HP heal, room move, debt). Specifically, no XP
	// gain — they died, they don't credit themselves for their
	// own demise.
	victim, _ := f.chars.GetByID(ctx, f.victim.ID)
	if victim.XP != progression.XPForLevel(12) {
		t.Errorf("victim.XP changed: got %d", victim.XP)
	}
}

// dropOnDeathFixture sets up a character with carried coin + bank
// coin + an equipped wielded item + a top-level container holding two
// nested items, then constructs a Manager wired to a memory item repo.
// Tests pick the toggle state via the returned (mgr, ...) handle.
type dropOnDeathFixture struct {
	mgr   *Manager
	chars *repo.MemoryCharacterRepo
	items *repo.MemoryItemRepo
	bus   *eventbus.Bus

	deathRoom int64
	boundRoom int64

	charID    int64
	swordID   int64
	packID    int64
	nested1ID int64
	nested2ID int64

	carriedCoin currency.Amount
	bankCoin    currency.Amount
}

func setupDropOnDeath(t *testing.T) dropOnDeathFixture {
	t.Helper()
	ctx := context.Background()
	bus := eventbus.New()
	chars := repo.NewMemoryCharacterRepo()
	items := repo.NewMemoryItemRepo()

	accs := repo.NewMemoryAccountRepo()
	acc, _ := accs.Create(ctx, repo.Account{Username: "alice-acct", PasswordHash: "h"})
	ch, err := chars.Create(ctx, repo.Character{
		AccountID:     acc.ID,
		Name:          "Alice",
		CurrentRoomID: 101,
		BoundRoomID:   200,
		// Sit mid-level so DeathDebt produces a non-zero delta and the
		// disabled-toggle test can observe debt accrual.
		XP:            progression.XPForLevel(5) + 1000,
		Coin:          50,
		BankBalance:   100,
		Core: creature.Core{
			HPCurrent: 0, HPMax: 40,
		},
	})
	if err != nil {
		t.Fatalf("seed char: %v", err)
	}

	mkItem := func(extID, name string, ownerCharID, parentItemID int64) int64 {
		it, err := items.Create(ctx, repo.Item{
			ExternalID:   extID,
			Name:         name,
			Type:         repo.ItemTypeTrash,
			OwnerCharacterID:  ownerCharID,
			ParentItemID: parentItemID,
		})
		if err != nil {
			t.Fatalf("seed item %s: %v", extID, err)
		}
		return it.ID
	}

	// Container has different stats-type requirement; build it
	// explicitly so the type matcher in Create is happy.
	pack, err := items.Create(ctx, repo.Item{
		ExternalID:  "alice-pack",
		Name:        "a leather backpack",
		Type:        repo.ItemTypeContainer,
		OwnerCharacterID: ch.ID,
		Stats:       &repo.ContainerStats{CapacityLbs: 30, CapacityCuFt: 5},
	})
	if err != nil {
		t.Fatalf("seed pack: %v", err)
	}

	swordID := mkItem("alice-sword", "a steel sword", ch.ID, 0)
	nested1 := mkItem("alice-rag", "a rag", 0, pack.ID)
	nested2 := mkItem("alice-flint", "a flint", 0, pack.ID)

	// Equip the sword in the primary wield slot (slot map metadata).
	if err := chars.RecordEquipment(ctx, ch.ID, creature.Equipment{
		PrimaryWield: swordID,
	}); err != nil {
		t.Fatalf("equip sword: %v", err)
	}

	mgr := New(bus, chars, nil, nil, items)

	return dropOnDeathFixture{
		mgr: mgr, chars: chars, items: items, bus: bus,
		deathRoom: 101, boundRoom: 200,
		charID:      ch.ID,
		swordID:     swordID,
		packID:      pack.ID,
		nested1ID:   nested1, nested2ID: nested2,
		carriedCoin: 50, bankCoin: 100,
	}
}

func TestCharacterDeath_DropOnDeath_Disabled_Default(t *testing.T) {
	ctx := context.Background()
	f := setupDropOnDeath(t)

	pre, _ := f.chars.GetByID(ctx, f.charID)
	startDebt := pre.XPDebt

	f.mgr.handleCharacterDeath(ctx,
		ActorRef{},
		ActorRef{Kind: ActorKindCharacter, ID: f.charID},
	)

	post, _ := f.chars.GetByID(ctx, f.charID)
	if post.Coin != f.carriedCoin {
		t.Errorf("Coin = %d, want %d (drop disabled)", post.Coin, f.carriedCoin)
	}
	if post.BankBalance != f.bankCoin {
		t.Errorf("BankBalance = %d, want %d", post.BankBalance, f.bankCoin)
	}
	if post.Equipment.PrimaryWield != f.swordID {
		t.Errorf("Equipment cleared: PrimaryWield = %d", post.Equipment.PrimaryWield)
	}
	if post.XPDebt <= startDebt {
		t.Errorf("XPDebt = %d, want > %d (10%% delta should apply)", post.XPDebt, startDebt)
	}
	// No corpse in death room.
	roomItems, _ := f.items.ListInRoom(ctx, f.deathRoom)
	for _, it := range roomItems {
		if it.Type == repo.ItemTypeContainer && strings.HasPrefix(it.ExternalID, "pcorpse-") {
			t.Errorf("player corpse spawned with drop-on-death disabled: %+v", it)
		}
	}
}

func TestCharacterDeath_DropOnDeath_Enabled_DumpsLootAndWaivesDebt(t *testing.T) {
	ctx := context.Background()
	f := setupDropOnDeath(t)
	f.mgr.SetDropOnDeath(true)

	pre, _ := f.chars.GetByID(ctx, f.charID)
	startDebt := pre.XPDebt

	f.mgr.handleCharacterDeath(ctx,
		ActorRef{},
		ActorRef{Kind: ActorKindCharacter, ID: f.charID},
	)

	// Character: carried coin zeroed, bank preserved, equipment cleared.
	post, _ := f.chars.GetByID(ctx, f.charID)
	if post.Coin != 0 {
		t.Errorf("Coin = %d, want 0 (drop enabled)", post.Coin)
	}
	if post.BankBalance != f.bankCoin {
		t.Errorf("BankBalance = %d, want %d (bank preserved)", post.BankBalance, f.bankCoin)
	}
	if post.Equipment.PrimaryWield != 0 ||
		len(post.Equipment.BeltPouches) != 0 ||
		len(post.Equipment.WornMisc) != 0 {
		t.Errorf("Equipment not cleared: %+v", post.Equipment)
	}
	if post.XPDebt != startDebt {
		t.Errorf("XPDebt = %d, want %d (drop should waive delta)", post.XPDebt, startDebt)
	}

	// Find the corpse.
	roomItems, _ := f.items.ListInRoom(ctx, f.deathRoom)
	var corpse *repo.Item
	for i := range roomItems {
		it := &roomItems[i]
		if it.Type == repo.ItemTypeContainer && strings.HasPrefix(it.ExternalID, "pcorpse-") {
			corpse = it
			break
		}
	}
	if corpse == nil {
		t.Fatal("player corpse not spawned")
	}
	if corpse.DecayExpiresAt == nil {
		t.Errorf("corpse missing DecayExpiresAt")
	}
	if !strings.Contains(corpse.Name, "Alice") {
		t.Errorf("corpse name = %q, want it to contain Alice", corpse.Name)
	}

	// Corpse should contain: sword, pack, coin pile. Nested items
	// stay inside pack (not direct children of corpse).
	contents, err := f.items.ListInContainer(ctx, corpse.ID)
	if err != nil {
		t.Fatalf("ListInContainer corpse: %v", err)
	}
	gotSword, gotPack, gotCoin := false, false, false
	for _, it := range contents {
		switch it.ID {
		case f.swordID:
			gotSword = true
		case f.packID:
			gotPack = true
		}
		if it.Type == repo.ItemTypeTradeGood && (it.Flags&repo.FlagTradeGood) != 0 {
			if it.Value != f.carriedCoin {
				t.Errorf("coin pile value = %d, want %d", it.Value, f.carriedCoin)
			}
			gotCoin = true
		}
	}
	if !gotSword {
		t.Errorf("sword not in corpse: %+v", contents)
	}
	if !gotPack {
		t.Errorf("pack not in corpse: %+v", contents)
	}
	if !gotCoin {
		t.Errorf("coin pile not in corpse: %+v", contents)
	}

	// Nested items should still be inside the pack (which is now
	// inside the corpse). They are reachable via ListInContainer(pack).
	nested, _ := f.items.ListInContainer(ctx, f.packID)
	gotRag, gotFlint := false, false
	for _, it := range nested {
		if it.ID == f.nested1ID {
			gotRag = true
		}
		if it.ID == f.nested2ID {
			gotFlint = true
		}
	}
	if !gotRag || !gotFlint {
		t.Errorf("nested items lost: rag=%v flint=%v", gotRag, gotFlint)
	}
}

func TestCharacterDeath_DropOnDeath_AffectDeathPath(t *testing.T) {
	// HandleAffectDeath flows through handleCharacterDeath with empty
	// killer; drop-on-death should still fire.
	ctx := context.Background()
	f := setupDropOnDeath(t)
	f.mgr.SetDropOnDeath(true)

	f.mgr.HandleAffectDeath(ctx, f.charID)

	post, _ := f.chars.GetByID(ctx, f.charID)
	if post.Coin != 0 {
		t.Errorf("Coin = %d, want 0 (drop fired for affect-death)", post.Coin)
	}
	if post.Equipment.PrimaryWield != 0 {
		t.Errorf("Equipment not cleared on affect-death drop: %+v", post.Equipment)
	}
}
