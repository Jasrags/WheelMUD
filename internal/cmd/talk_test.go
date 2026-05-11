package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/dialogue"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// makeNPCWithDialogue creates an in-memory mob template + spawned
// instance in roomID. Returns the populated repos and the spawned
// instance for assertion.
func makeNPCWithDialogue(t *testing.T, roomID int64, name string, tree *dialogue.Tree) (
	repo.MobInstanceRepo, repo.MobTemplateRepo, creature.MobInstance,
) {
	t.Helper()
	templates := repo.NewMemoryMobTemplateRepo()
	mobs := repo.NewMemoryMobInstanceRepo()

	var djson []byte
	if tree != nil {
		var err error
		djson, err = json.Marshal(tree)
		if err != nil {
			t.Fatalf("marshal tree: %v", err)
		}
	}
	tpl := creature.MobTemplate{
		ExternalID:   "test." + name,
		Core:         creature.Core{Name: name, HPMax: 1},
		DialogueJSON: djson,
	}
	created, err := templates.Create(context.Background(), tpl)
	if err != nil {
		t.Fatalf("template create: %v", err)
	}
	inst := creature.NewInstanceFromTemplate(created, roomID, 0)
	// NewInstanceFromTemplate doesn't copy Core.Name (production reads
	// it via SQL JOIN on the spawn name column); the memory repo
	// roundtrip needs it set so MatchMob can resolve the keyword.
	inst.Core.Name = name
	spawned, err := mobs.Create(context.Background(), inst)
	if err != nil {
		t.Fatalf("instance create: %v", err)
	}
	return mobs, templates, spawned
}

func TestTalk_RefusesWhenNoMob(t *testing.T) {
	_, alice, _, aOut, _ := commPair(t)
	mobs := repo.NewMemoryMobInstanceRepo()
	templates := repo.NewMemoryMobTemplateRepo()

	talk := NewTalk(mobs, templates, func(_ *telnet.Session, _, _ string, _ *dialogue.Tree) error {
		t.Fatal("pushDialogue should not be called")
		return nil
	})
	runCmd(t, talk, alice, "elder")
	if !strings.Contains(aOut.String(), "isn't here") {
		t.Fatalf("expected refusal, got %q", aOut.String())
	}
}

func TestTalk_RefusesWhenMobHasNoDialogue(t *testing.T) {
	_, alice, _, aOut, _ := commPair(t)
	mobs, templates, _ := makeNPCWithDialogue(t, 1, "guard", nil)

	called := false
	talk := NewTalk(mobs, templates, func(_ *telnet.Session, _, _ string, _ *dialogue.Tree) error {
		called = true
		return nil
	})
	runCmd(t, talk, alice, "guard")
	if called {
		t.Fatal("pushDialogue should not be called for empty dialogue")
	}
	if !strings.Contains(aOut.String(), "nothing to say") {
		t.Fatalf("expected refusal, got %q", aOut.String())
	}
}

func TestTalk_PushesDialogueOnMatch(t *testing.T) {
	_, alice, _, _, _ := commPair(t)
	tree := &dialogue.Tree{
		Root: "root",
		Nodes: map[dialogue.NodeID]dialogue.Node{
			"root": {ID: "root", Prompt: "hi", Responses: []dialogue.Response{
				{Match: []string{"bye"}},
			}},
		},
	}
	mobs, templates, _ := makeNPCWithDialogue(t, 1, "elder", tree)

	pushed := struct {
		called     bool
		name       string
		externalID string
		root       dialogue.NodeID
	}{}
	talk := NewTalk(mobs, templates, func(_ *telnet.Session, npcName, npcExternalID string, t *dialogue.Tree) error {
		pushed.called = true
		pushed.name = npcName
		pushed.externalID = npcExternalID
		pushed.root = t.Root
		return nil
	})
	runCmd(t, talk, alice, "elder")
	if !pushed.called {
		t.Fatal("pushDialogue not called")
	}
	if pushed.name != "elder" {
		t.Fatalf("npcName = %q, want elder", pushed.name)
	}
	if pushed.externalID != "test.elder" {
		t.Fatalf("externalID = %q, want test.elder", pushed.externalID)
	}
	if pushed.root != "root" {
		t.Fatalf("tree.Root = %q", pushed.root)
	}
}

func TestTalk_RefusesOnInvalidJSON(t *testing.T) {
	_, alice, _, aOut, _ := commPair(t)
	templates := repo.NewMemoryMobTemplateRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	tpl, err := templates.Create(context.Background(), creature.MobTemplate{
		ExternalID:   "test.broken",
		Core:         creature.Core{Name: "broken", HPMax: 1},
		DialogueJSON: []byte("{not json"),
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	inst := creature.NewInstanceFromTemplate(tpl, 1, 0)
	inst.Core.Name = "broken"
	if _, err := mobs.Create(context.Background(), inst); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	talk := NewTalk(mobs, templates, func(_ *telnet.Session, _, _ string, _ *dialogue.Tree) error {
		t.Fatal("pushDialogue should not be called")
		return nil
	})
	runCmd(t, talk, alice, "broken")
	if !strings.Contains(aOut.String(), "incoherent") {
		t.Fatalf("expected incoherent refusal, got %q", aOut.String())
	}
}

func TestTalk_DefangsCfmtInjectionViaTarget(t *testing.T) {
	// Player input flows into the refusal line via fmt.Sprintf inside
	// a {{...}}::yellow wrapper. defangCfmt must neutralise `{{` so a
	// crafted target can't open a styled span that consumes the
	// rest of the rendered prompt.
	_, alice, _, aOut, _ := commPair(t)
	mobs := repo.NewMemoryMobInstanceRepo()
	templates := repo.NewMemoryMobTemplateRepo()
	talk := NewTalk(mobs, templates, nil)
	runCmd(t, talk, alice, "{{evil}}::red")
	out := aOut.String()
	if strings.Contains(out, "{{evil}}::red") {
		t.Fatalf("raw cfmt tags reached output: %q", out)
	}
	if !strings.Contains(out, "isn't here") {
		t.Fatalf("expected refusal text, got %q", out)
	}
}

func TestTalk_NoArgsRejected(t *testing.T) {
	// MinArgs=1, so the dispatcher would emit Long. Run directly with
	// empty args to confirm the inner guard still catches it.
	_, alice, _, aOut, _ := commPair(t)
	mobs := repo.NewMemoryMobInstanceRepo()
	templates := repo.NewMemoryMobTemplateRepo()
	talk := NewTalk(mobs, templates, nil)
	ctx := &telnet.Context{
		Ctx:     context.Background(),
		Session: alice,
		Name:    "talk",
		Args:    nil,
		Raw:     "",
	}
	if err := talk.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(aOut.String(), "Talk to whom") {
		t.Fatalf("expected usage hint, got %q", aOut.String())
	}
}
