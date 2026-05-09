package quest

import (
	"context"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/world"
)

// fixtureEngine builds an Engine wired to in-memory repos and the
// goodCatalog test fixture. Used by every transition test below.
func fixtureEngine(t *testing.T) (*Engine, repo.CharacterRepo, *repo.MemoryAccountRepo, repo.RoomRepo) {
	t.Helper()
	chars := repo.NewMemoryCharacterRepo()
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 100, ExternalID: "tr.westwood.path_2", Name: "Westwood Path"})
	accounts := repo.NewMemoryAccountRepo()
	e := NewEngine(goodCatalog(), chars, rooms, nil, nil, nil)
	return e, chars, accounts, rooms
}

func makeChar(t *testing.T, chars repo.CharacterRepo, accounts *repo.MemoryAccountRepo, name string) repo.Character {
	t.Helper()
	acc, err := accounts.Create(context.Background(), repo.Account{Username: name, PasswordHash: "h"})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	c, err := chars.Create(context.Background(), repo.Character{AccountID: acc.ID, Name: name})
	if err != nil {
		t.Fatalf("create character: %v", err)
	}
	return c
}

func TestEngine_AcceptQuest(t *testing.T) {
	e, chars, accounts, _ := fixtureEngine(t)
	c := makeChar(t, chars, accounts, "Rand")

	if err := e.AcceptQuest(context.Background(), c.ID, "lost_lamb"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	got, _ := chars.GetByID(context.Background(), c.ID)
	if len(got.QuestLog) != 1 || got.QuestLog[0].QuestID != "lost_lamb" {
		t.Fatalf("quest log: %+v", got.QuestLog)
	}
	if got.QuestLog[0].StepIndex != 0 {
		t.Fatalf("StepIndex = %d, want 0", got.QuestLog[0].StepIndex)
	}
}

func TestEngine_AcceptQuest_Idempotent(t *testing.T) {
	e, chars, accounts, _ := fixtureEngine(t)
	c := makeChar(t, chars, accounts, "Rand")
	ctx := context.Background()

	_ = e.AcceptQuest(ctx, c.ID, "lost_lamb")
	_ = e.AcceptQuest(ctx, c.ID, "lost_lamb")
	got, _ := chars.GetByID(ctx, c.ID)
	if len(got.QuestLog) != 1 {
		t.Fatalf("re-accept duplicated entry: %d", len(got.QuestLog))
	}
}

func TestEngine_AcceptUnknownQuest_NoOp(t *testing.T) {
	e, chars, accounts, _ := fixtureEngine(t)
	c := makeChar(t, chars, accounts, "Rand")
	if err := e.AcceptQuest(context.Background(), c.ID, "ghost_quest"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := chars.GetByID(context.Background(), c.ID)
	if len(got.QuestLog) != 0 {
		t.Fatalf("unknown quest should not write a log entry: %+v", got.QuestLog)
	}
}

func TestEngine_ReachRoom_Advances(t *testing.T) {
	e, chars, accounts, _ := fixtureEngine(t)
	c := makeChar(t, chars, accounts, "Rand")
	ctx := context.Background()

	_ = e.AcceptQuest(ctx, c.ID, "lost_lamb")
	// Step 0 is reach_room tr.westwood.path_2.
	e.onPlayerEntered(ctx, world.PlayerEntered{
		CharacterID: c.ID,
		ToRoomID:    100,
	})
	got, _ := chars.GetByID(ctx, c.ID)
	if got.QuestLog[0].StepIndex != 1 {
		t.Fatalf("StepIndex after reach_room = %d, want 1", got.QuestLog[0].StepIndex)
	}
}

func TestEngine_ReachWrongRoom_NoAdvance(t *testing.T) {
	e, chars, accounts, rooms := fixtureEngine(t)
	rooms.(*repo.MemoryRoomRepo).Insert(repo.Room{ID: 200, ExternalID: "tr.elsewhere"})
	c := makeChar(t, chars, accounts, "Rand")
	ctx := context.Background()
	_ = e.AcceptQuest(ctx, c.ID, "lost_lamb")
	e.onPlayerEntered(ctx, world.PlayerEntered{CharacterID: c.ID, ToRoomID: 200})
	got, _ := chars.GetByID(ctx, c.ID)
	if got.QuestLog[0].StepIndex != 0 {
		t.Fatalf("wrong room advanced: StepIndex = %d", got.QuestLog[0].StepIndex)
	}
}

func TestEngine_KillN_DecrementsAndAdvances(t *testing.T) {
	e, chars, accounts, _ := fixtureEngine(t)
	c := makeChar(t, chars, accounts, "Rand")
	ctx := context.Background()

	_ = e.AcceptQuest(ctx, c.ID, "lost_lamb")
	// Skip to kill_n step (step 1) by advancing past reach_room.
	got, _ := chars.GetByID(ctx, c.ID)
	got.QuestLog[0].StepIndex = 1
	got.QuestLog[0].StateJSON = `{"remaining":3}`
	_ = chars.RecordQuestProgress(ctx, c.ID, got.QuestLog)

	kill := combat.CombatDeath{
		RoomID:                100,
		Victim:                combat.ActorRef{Kind: combat.ActorKindMob, ID: 99},
		Killer:                combat.ActorRef{Kind: combat.ActorKindCharacter, ID: c.ID},
		MobTemplateExternalID: "tr.wolf",
	}
	for i := 0; i < 2; i++ {
		e.onCombatDeath(ctx, kill)
	}
	got, _ = chars.GetByID(ctx, c.ID)
	if got.QuestLog[0].StepIndex != 1 {
		t.Fatalf("premature advance: StepIndex = %d", got.QuestLog[0].StepIndex)
	}
	if got.QuestLog[0].StateJSON != `{"remaining":1}` {
		t.Fatalf("StateJSON after 2 kills = %q", got.QuestLog[0].StateJSON)
	}
	// Third kill should advance to step 2.
	e.onCombatDeath(ctx, kill)
	got, _ = chars.GetByID(ctx, c.ID)
	if got.QuestLog[0].StepIndex != 2 {
		t.Fatalf("after 3 kills StepIndex = %d, want 2", got.QuestLog[0].StepIndex)
	}
}

func TestEngine_KillWrongMob_NoDecrement(t *testing.T) {
	e, chars, accounts, _ := fixtureEngine(t)
	c := makeChar(t, chars, accounts, "Rand")
	ctx := context.Background()
	_ = e.AcceptQuest(ctx, c.ID, "lost_lamb")
	got, _ := chars.GetByID(ctx, c.ID)
	got.QuestLog[0].StepIndex = 1
	got.QuestLog[0].StateJSON = `{"remaining":3}`
	_ = chars.RecordQuestProgress(ctx, c.ID, got.QuestLog)

	e.onCombatDeath(ctx, combat.CombatDeath{
		Killer: combat.ActorRef{Kind: combat.ActorKindCharacter, ID: c.ID},
		Victim: combat.ActorRef{Kind: combat.ActorKindMob, ID: 1},
		MobTemplateExternalID: "tr.bandit",
	})
	got, _ = chars.GetByID(ctx, c.ID)
	if got.QuestLog[0].StateJSON != `{"remaining":3}` {
		t.Fatalf("wrong mob decremented: %q", got.QuestLog[0].StateJSON)
	}
}

func TestEngine_TalkTo_AdvancesAndCompletes(t *testing.T) {
	e, chars, accounts, _ := fixtureEngine(t)
	c := makeChar(t, chars, accounts, "Rand")
	ctx := context.Background()

	_ = e.AcceptQuest(ctx, c.ID, "lost_lamb")
	got, _ := chars.GetByID(ctx, c.ID)
	got.QuestLog[0].StepIndex = 2 // talk_to tr.elder is step 2
	got.QuestLog[0].StateJSON = "{}"
	_ = chars.RecordQuestProgress(ctx, c.ID, got.QuestLog)

	if err := e.AdvanceTalkTo(ctx, c.ID, "lost_lamb", "tr.elder"); err != nil {
		t.Fatalf("advance: %v", err)
	}
	got, _ = chars.GetByID(ctx, c.ID)
	if got.QuestLog[0].CompletedAt.IsZero() {
		t.Fatalf("expected CompletedAt on final-step advance")
	}
	if got.XP != 200 {
		t.Fatalf("XP = %d, want 200 (reward applied)", got.XP)
	}
	if got.Coin != currency.Amount(5000) {
		t.Fatalf("Coin = %v, want 5000cp", got.Coin)
	}
}

func TestEngine_TalkTo_WrongNPC_NoAdvance(t *testing.T) {
	e, chars, accounts, _ := fixtureEngine(t)
	c := makeChar(t, chars, accounts, "Rand")
	ctx := context.Background()
	_ = e.AcceptQuest(ctx, c.ID, "lost_lamb")
	got, _ := chars.GetByID(ctx, c.ID)
	got.QuestLog[0].StepIndex = 2
	_ = chars.RecordQuestProgress(ctx, c.ID, got.QuestLog)

	if err := e.AdvanceTalkTo(ctx, c.ID, "lost_lamb", "tr.someone_else"); err != nil {
		t.Fatalf("advance: %v", err)
	}
	got, _ = chars.GetByID(ctx, c.ID)
	if !got.QuestLog[0].CompletedAt.IsZero() {
		t.Fatal("wrong NPC should not complete the quest")
	}
}

// TestEngine_Advance_ScriptStep covers the Phase F #32 slice 2
// `quest.advance` Lua entry point against a script-kind step. The
// engine treats StepScript identically to StepTalkTo for advance
// purposes: external code (a Lua catalog script) decides when the
// goal is met and calls Advance to transition.
func TestEngine_Advance_ScriptStep(t *testing.T) {
	chars := repo.NewMemoryCharacterRepo()
	rooms := repo.NewMemoryRoomRepo()
	cat := &Catalog{ByID: map[string]*Quest{
		"q": {
			ID: "q", Name: "Q",
			Steps: []Step{
				{Kind: StepScript, Prompt: "wait for hint", Script: "advance_q"},
				{Kind: StepReachRoom, Prompt: "go home", Room: "tr.start"},
			},
		},
	}}
	e := NewEngine(cat, chars, rooms, nil, nil, nil)
	accounts := repo.NewMemoryAccountRepo()
	c := makeChar(t, chars, accounts, "Mat")
	ctx := context.Background()

	if err := e.AcceptQuest(ctx, c.ID, "q"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := e.Advance(ctx, c.ID, "q"); err != nil {
		t.Fatalf("advance: %v", err)
	}
	got, _ := chars.GetByID(ctx, c.ID)
	if got.QuestLog[0].StepIndex != 1 {
		t.Fatalf("StepIndex = %d, want 1 after Advance on script step", got.QuestLog[0].StepIndex)
	}
}

// Advance on a counter-driven step kind (kill_n / reach_room) is a
// no-op — calling code is buggy, but we don't punish the player by
// auto-advancing past their kill quota. Engine logs and returns nil.
func TestEngine_Advance_OnKillNStep_NoOp(t *testing.T) {
	e, chars, accounts, _ := fixtureEngine(t)
	c := makeChar(t, chars, accounts, "Egwene")
	ctx := context.Background()

	_ = e.AcceptQuest(ctx, c.ID, "lost_lamb")
	// Advance past reach_room so step 1 (kill_n) is active.
	e.onPlayerEntered(ctx, world.PlayerEntered{CharacterID: c.ID, ToRoomID: 100})
	got, _ := chars.GetByID(ctx, c.ID)
	if got.QuestLog[0].StepIndex != 1 {
		t.Fatalf("setup precondition failed: %d", got.QuestLog[0].StepIndex)
	}

	if err := e.Advance(ctx, c.ID, "lost_lamb"); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	got, _ = chars.GetByID(ctx, c.ID)
	if got.QuestLog[0].StepIndex != 1 {
		t.Fatalf("StepIndex = %d after no-op Advance, want 1 (unchanged)", got.QuestLog[0].StepIndex)
	}
}

// Advance on a quest the character isn't on is a silent no-op, not
// an error — Lua scripts may be invoked in contexts where the
// character has dropped the quest already.
func TestEngine_Advance_NotOnQuest_NoOp(t *testing.T) {
	e, chars, accounts, _ := fixtureEngine(t)
	c := makeChar(t, chars, accounts, "Perrin")
	if err := e.Advance(context.Background(), c.ID, "lost_lamb"); err != nil {
		t.Fatalf("Advance: %v", err)
	}
}

func TestEngine_AbandonQuest(t *testing.T) {
	e, chars, accounts, _ := fixtureEngine(t)
	c := makeChar(t, chars, accounts, "Rand")
	ctx := context.Background()
	_ = e.AcceptQuest(ctx, c.ID, "lost_lamb")
	if err := e.AbandonQuest(ctx, c.ID, "lost_lamb"); err != nil {
		t.Fatalf("abandon: %v", err)
	}
	got, _ := chars.GetByID(ctx, c.ID)
	if len(got.QuestLog) != 0 {
		t.Fatalf("abandon should drop the entry: %+v", got.QuestLog)
	}
	// Idempotent: abandoning again is fine.
	if err := e.AbandonQuest(ctx, c.ID, "lost_lamb"); err != nil {
		t.Fatalf("re-abandon: %v", err)
	}
}

// suppress unused-variable warnings on imports we keep for clarity.
var _ = creature.QuestProgress{}
