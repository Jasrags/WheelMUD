package trigger

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/testhelper"
)

func TestSayAction_BroadcastsToRoom(t *testing.T) {
	registry := session.NewRegistry()
	alice, aOut := testhelper.BufSession(t)
	alice.SetInWorld(1, "Alice", 100)
	registry.Bind(1, alice)

	bob, bOut := testhelper.BufSession(t)
	bob.SetInWorld(2, "Bob", 100)
	registry.Bind(2, bob)

	// A peer in another room must NOT receive the broadcast.
	carol, cOut := testhelper.BufSession(t)
	carol.SetInWorld(3, "Carol", 200)
	registry.Bind(3, carol)

	mobs := repo.NewMemoryMobInstanceRepo()
	mobInst, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: 7,
		Core:       creature.Core{Name: "the innkeeper", CurrentRoomID: 100},
	})
	if err != nil {
		t.Fatalf("mob create: %v", err)
	}

	deps := ActionDeps{Sessions: registry, Mobs: mobs}
	owner := OwnerRef{Kind: OwnerMobTemplate, ID: 7, InstanceID: mobInst.ID, RoomID: 100}
	if err := SayAction(context.Background(), deps, owner,
		EventCtx{Event: EventOnEnter, RoomID: 100},
		[]byte(`{"text":"Welcome, traveler."}`)); err != nil {
		t.Fatalf("SayAction: %v", err)
	}

	if !strings.Contains(aOut.String(), "the innkeeper says,") {
		t.Fatalf("alice missing speaker line: %q", aOut.String())
	}
	if !strings.Contains(aOut.String(), "Welcome, traveler.") {
		t.Fatalf("alice missing payload: %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "the innkeeper says,") {
		t.Fatalf("bob missing speaker line: %q", bOut.String())
	}
	if cOut.String() != "" {
		t.Fatalf("carol (other room) got: %q", cOut.String())
	}
}

func TestEmoteAction_NoQuotes(t *testing.T) {
	registry := session.NewRegistry()
	alice, aOut := testhelper.BufSession(t)
	alice.SetInWorld(1, "Alice", 100)
	registry.Bind(1, alice)

	mobs := repo.NewMemoryMobInstanceRepo()
	mobInst, _ := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: 7,
		Core:       creature.Core{Name: "the innkeeper", CurrentRoomID: 100},
	})

	deps := ActionDeps{Sessions: registry, Mobs: mobs}
	owner := OwnerRef{Kind: OwnerMobTemplate, ID: 7, InstanceID: mobInst.ID, RoomID: 100}
	if err := EmoteAction(context.Background(), deps, owner,
		EventCtx{Event: EventOnSay, RoomID: 100},
		[]byte(`{"text":"leans in conspiratorially."}`)); err != nil {
		t.Fatalf("EmoteAction: %v", err)
	}

	out := aOut.String()
	if !strings.Contains(out, "the innkeeper leans in conspiratorially.") {
		t.Fatalf("emote missing: %q", out)
	}
	if strings.Contains(out, `"`) {
		t.Fatalf("emote should not wrap in quotes: %q", out)
	}
}

func TestSayAction_EmptyTextNoOp(t *testing.T) {
	registry := session.NewRegistry()
	alice, aOut := testhelper.BufSession(t)
	alice.SetInWorld(1, "Alice", 100)
	registry.Bind(1, alice)

	deps := ActionDeps{Sessions: registry}
	owner := OwnerRef{Kind: OwnerRoom, ID: 100, RoomID: 100}
	for _, payload := range [][]byte{nil, []byte(``), []byte(`{}`), []byte(`{"text":"  "}`), []byte(`{"text":""}`)} {
		if err := SayAction(context.Background(), deps, owner, EventCtx{Event: EventOnEnter, RoomID: 100}, payload); err != nil {
			t.Fatalf("payload %q: %v", payload, err)
		}
	}
	if aOut.String() != "" {
		t.Fatalf("expected silent no-op; got %q", aOut.String())
	}
}

func TestSayAction_RoomOwnerSpeaksAsVoice(t *testing.T) {
	registry := session.NewRegistry()
	alice, aOut := testhelper.BufSession(t)
	alice.SetInWorld(1, "Alice", 100)
	registry.Bind(1, alice)

	deps := ActionDeps{Sessions: registry}
	owner := OwnerRef{Kind: OwnerRoom, ID: 100, RoomID: 100}
	if err := SayAction(context.Background(), deps, owner,
		EventCtx{Event: EventOnEnter, RoomID: 100},
		[]byte(`{"text":"Beware."}`)); err != nil {
		t.Fatalf("SayAction: %v", err)
	}
	if !strings.Contains(aOut.String(), "A voice says,") {
		t.Fatalf("expected room-owner voice prefix: %q", aOut.String())
	}
}

func TestSayAction_StripsControlBytesAndCfmt(t *testing.T) {
	registry := session.NewRegistry()
	alice, aOut := testhelper.BufSession(t)
	alice.SetInWorld(1, "Alice", 100)
	registry.Bind(1, alice)

	deps := ActionDeps{Sessions: registry}
	owner := OwnerRef{Kind: OwnerRoom, ID: 100, RoomID: 100}
	// Payload includes ESC, bare CR, NUL, plus a cfmt close sequence.
	payload := []byte("{\"text\":\"hi\\u001b[31mthere\\r}}::red\"}")
	if err := SayAction(context.Background(), deps, owner,
		EventCtx{Event: EventOnEnter, RoomID: 100}, payload); err != nil {
		t.Fatalf("SayAction: %v", err)
	}
	out := aOut.String()
	// The payload's ESC byte (0x1b) and bare CR (0x0d) must be gone.
	// cfmt itself emits SGR escapes for color and WriteAsync prepends
	// an EL-erase prefix, so we can't assert "no ESC anywhere"; we
	// instead confirm the SGR sequence the payload tried to inject
	// is absent. Same for the bare CR — payload "there\r" should not
	// produce "there\r" in the output (\r between rune positions).
	if strings.Contains(out, "\x1b[31m") {
		t.Fatalf("payload SGR escape leaked: %q", out)
	}
	if strings.Contains(out, "there\r") && !strings.HasSuffix(out, "there\r\n") {
		t.Fatalf("bare CR survived defang: %q", out)
	}
	// cfmt close sequence inside the payload must be defanged. The
	// renderer's own SGR resets are fine; we're checking the literal
	// `}}::red` substring (which the payload tried to inject) is gone.
	if strings.Contains(out, "}}::red") {
		t.Fatalf("cfmt close sequence not defanged: %q", out)
	}
}

func TestActionRegistry_DefaultsHaveBuiltins(t *testing.T) {
	r := DefaultActions()
	for _, k := range []string{"noop", "say", "emote"} {
		if r.Lookup(k) == nil {
			t.Errorf("DefaultActions missing %q", k)
		}
	}
	if r.Lookup("missing") != nil {
		t.Error("Lookup of unknown returned non-nil")
	}
}
