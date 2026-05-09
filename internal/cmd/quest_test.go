package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/quest"
	"github.com/Jasrags/WheelMUD/internal/repo"
)

// questFixture returns a session bound to a memory CharacterRepo +
// a quest catalog containing a one-step talk_to quest.
func questFixture(t *testing.T) (*repo.MemoryCharacterRepo, *quest.Catalog, repo.Character) {
	t.Helper()
	chars := repo.NewMemoryCharacterRepo()
	accounts := repo.NewMemoryAccountRepo()
	acc, _ := accounts.Create(context.Background(), repo.Account{Username: "p", PasswordHash: "h"})
	c, err := chars.Create(context.Background(), repo.Character{AccountID: acc.ID, Name: "Hero"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	cat := &quest.Catalog{ByID: map[string]*quest.Quest{
		"smoke": {
			ID:   "smoke",
			Name: "Smoke Test",
			Steps: []quest.Step{
				{Kind: quest.StepKillN, Mob: "tr.wolf", Count: 3, Prompt: "Kill 3 wolves."},
				{Kind: quest.StepTalkTo, Mob: "tr.elder", Prompt: "Return to the elder."},
			},
		},
	}}
	return chars, cat, c
}

func TestQuest_BareList_Empty(t *testing.T) {
	chars, cat, c := questFixture(t)
	_, alice, _, aOut, _ := commPair(t)
	alice.CharacterID = c.ID
	alice.CharacterName = c.Name

	v := NewQuest(chars, cat, nil, nil)
	runCmd(t, v, alice, "")
	if !strings.Contains(aOut.String(), "no active quests") {
		t.Fatalf("expected empty hint, got %q", aOut.String())
	}
}

func TestQuest_ListShowsActiveAndCompleted(t *testing.T) {
	chars, cat, c := questFixture(t)
	ctx := context.Background()
	_ = chars.RecordQuestProgress(ctx, c.ID, []creature.QuestProgress{
		{QuestID: "smoke", StepIndex: 0, StateJSON: `{"remaining":2}`},
	})

	_, alice, _, aOut, _ := commPair(t)
	alice.CharacterID = c.ID
	alice.CharacterName = c.Name

	v := NewQuest(chars, cat, nil, nil)
	runCmd(t, v, alice, "")
	out := aOut.String()
	if !strings.Contains(out, "Smoke Test") || !strings.Contains(out, "Kill 3 wolves") {
		t.Fatalf("active quest not rendered: %q", out)
	}
	if !strings.Contains(out, "2 remaining") {
		t.Fatalf("kill_n remaining count not rendered: %q", out)
	}
}

func TestQuest_Info_RendersStepProgress(t *testing.T) {
	chars, cat, c := questFixture(t)
	ctx := context.Background()
	_ = chars.RecordQuestProgress(ctx, c.ID, []creature.QuestProgress{
		{QuestID: "smoke", StepIndex: 1, StateJSON: "{}"},
	})

	_, alice, _, aOut, _ := commPair(t)
	alice.CharacterID = c.ID
	alice.CharacterName = c.Name

	v := NewQuest(chars, cat, nil, nil)
	runCmd(t, v, alice, "info smoke")
	out := aOut.String()
	if !strings.Contains(out, "Smoke Test") {
		t.Fatalf("info: missing name: %q", out)
	}
	// Step 0 should show ✓ (advanced past), step 1 should show ▸.
	if !strings.Contains(out, "✓") || !strings.Contains(out, "▸") {
		t.Fatalf("info: missing step markers: %q", out)
	}
}

func TestQuest_Info_UnknownQuest(t *testing.T) {
	chars, cat, c := questFixture(t)
	_, alice, _, aOut, _ := commPair(t)
	alice.CharacterID = c.ID
	alice.CharacterName = c.Name
	v := NewQuest(chars, cat, nil, nil)
	runCmd(t, v, alice, "info ghost")
	if !strings.Contains(aOut.String(), "No such quest") {
		t.Fatalf("expected unknown-quest refusal: %q", aOut.String())
	}
}

func TestQuest_Abandon_NotOnQuest(t *testing.T) {
	chars, cat, c := questFixture(t)
	_, alice, _, aOut, _ := commPair(t)
	alice.CharacterID = c.ID
	alice.CharacterName = c.Name

	engine := quest.NewEngine(cat, chars, repo.NewMemoryRoomRepo(), nil, nil, nil)
	v := NewQuest(chars, cat, engine, nil)
	runCmd(t, v, alice, "abandon smoke")
	if !strings.Contains(aOut.String(), "aren't on") {
		t.Fatalf("expected not-on-quest refusal: %q", aOut.String())
	}
}

func TestQuest_Abandon_RemovesEntry(t *testing.T) {
	chars, cat, c := questFixture(t)
	ctx := context.Background()
	_ = chars.RecordQuestProgress(ctx, c.ID, []creature.QuestProgress{
		{QuestID: "smoke", StepIndex: 0, StateJSON: `{"remaining":3}`},
	})

	_, alice, _, aOut, _ := commPair(t)
	alice.CharacterID = c.ID
	alice.CharacterName = c.Name

	engine := quest.NewEngine(cat, chars, repo.NewMemoryRoomRepo(), nil, nil, nil)
	v := NewQuest(chars, cat, engine, nil)
	runCmd(t, v, alice, "abandon smoke")
	if !strings.Contains(aOut.String(), "abandon") {
		t.Fatalf("expected abandon ack: %q", aOut.String())
	}
	got, _ := chars.GetByID(ctx, c.ID)
	if len(got.QuestLog) != 0 {
		t.Fatalf("abandon should empty log: %+v", got.QuestLog)
	}
}

func TestQuest_UnknownSubcommand(t *testing.T) {
	chars, cat, c := questFixture(t)
	_, alice, _, aOut, _ := commPair(t)
	alice.CharacterID = c.ID
	alice.CharacterName = c.Name
	v := NewQuest(chars, cat, nil, nil)
	runCmd(t, v, alice, "snapfingers")
	if !strings.Contains(aOut.String(), "Unknown subcommand") {
		t.Fatalf("expected unknown subcmd refusal: %q", aOut.String())
	}
}
