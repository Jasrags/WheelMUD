package mode

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/dialogue"
	"github.com/Jasrags/WheelMUD/telnet"
)

func sampleTree() *dialogue.Tree {
	return &dialogue.Tree{
		Root: "greet",
		Nodes: map[dialogue.NodeID]dialogue.Node{
			"greet": {
				ID:     "greet",
				Prompt: "Greetings, traveler.",
				Responses: []dialogue.Response{
					{Match: []string{"hello", "hi"}, Reply: "Well met.", Next: "menu"},
					{Match: []string{"bye"}, Reply: "Travel safely.", Effects: []dialogue.Effect{{Kind: dialogue.EffectEnd}}},
				},
			},
			"menu": {
				ID:     "menu",
				Prompt: "What brings you here?",
				Responses: []dialogue.Response{
					{Match: []string{"quest"}, Reply: "Then take this charge.",
						Effects: []dialogue.Effect{{Kind: dialogue.EffectSetFlag, Args: map[string]string{"name": "started"}}},
						Next:    "menu"},
					{Match: []string{"reward"}, Reply: "Your service is noted.",
						Show: dialogue.Show{RequireFlag: "started"}, Next: "greet"},
					{Match: []string{"farewell"}, Reply: "Until next time.", Next: ""},
				},
			},
		},
	}
}

// dialogueFixture wraps a Dialogue mode pushed onto a paired session
// with a draining peer so tests can assert on rendered output.
type dialogueFixture struct {
	t        *testing.T
	session  *telnet.Session
	mode     *Dialogue
	captured *safeBuf
	pushArgs *struct {
		mode string
		args map[string]string
	}
}

func pushDialogue(t *testing.T) *dialogueFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	s := telnet.NewSession(server)
	pushed := &struct {
		mode string
		args map[string]string
	}{}
	hook := func(_ context.Context, _ *telnet.Session, name string, args map[string]string) error {
		pushed.mode = name
		pushed.args = args
		// Simulate handing off to a sibling mode by replacing our own —
		// but for tests we keep the dialogue mode on the stack so we
		// can observe the post-effect state. The real cmd-layer hook
		// would PushMode/ReplaceMode here.
		return nil
	}
	m, err := NewDialogue("the elder", "tr.elder", sampleTree(), DialogueHooks{PushMode: hook})
	if err != nil {
		t.Fatalf("NewDialogue: %v", err)
	}
	if err := s.PushMode(m); err != nil {
		t.Fatalf("PushMode: %v", err)
	}
	return &dialogueFixture{t: t, session: s, mode: m, captured: captured, pushArgs: pushed}
}

func (f *dialogueFixture) feed(line string) {
	f.t.Helper()
	if err := f.session.CurrentMode().Handle(context.Background(), f.session, line); err != nil {
		f.t.Fatalf("Handle(%q): %v", line, err)
	}
	// Match charCreateFixture.feed: WriteRaw fires before drainPeer's
	// goroutine schedules; let it copy out before the assertion runs.
	time.Sleep(20 * time.Millisecond)
}

func TestDialogue_NewDialogue_RejectsInvalidTree(t *testing.T) {
	bad := &dialogue.Tree{Root: "missing", Nodes: map[dialogue.NodeID]dialogue.Node{
		"present": {ID: "present", Prompt: "x"},
	}}
	if _, err := NewDialogue("bob", "bob.id", bad, DialogueHooks{}); err == nil {
		t.Fatal("expected validation error for dangling root")
	}
}

func TestDialogue_PromptRendersNumberedChoices(t *testing.T) {
	f := pushDialogue(t)
	out := f.mode.Prompt(context.Background(), f.session)
	if !strings.Contains(out, "Greetings, traveler.") {
		t.Fatalf("prompt missing greeting: %q", out)
	}
	if !strings.Contains(out, "1) hello") || !strings.Contains(out, "2) bye") {
		t.Fatalf("prompt missing numbered choices: %q", out)
	}
}

func TestDialogue_NumberedChoiceAdvances(t *testing.T) {
	f := pushDialogue(t)
	// Force visible-list computation.
	_ = f.mode.Prompt(context.Background(), f.session)
	f.feed("1")
	if f.mode.currentID != "menu" {
		t.Fatalf("currentID = %q, want menu", f.mode.currentID)
	}
	if !strings.Contains(f.captured.String(), "Well met.") {
		t.Fatalf("expected reply in output: %q", f.captured.String())
	}
}

func TestDialogue_KeywordChoiceMatches(t *testing.T) {
	f := pushDialogue(t)
	_ = f.mode.Prompt(context.Background(), f.session)
	f.feed("hello")
	if f.mode.currentID != "menu" {
		t.Fatalf("currentID = %q, want menu", f.mode.currentID)
	}
}

func TestDialogue_FreeTextSubstringMatches(t *testing.T) {
	f := pushDialogue(t)
	_ = f.mode.Prompt(context.Background(), f.session)
	f.feed("tell me about your quest please")
	if f.mode.currentID != "greet" {
		// "quest" only matches at the menu node — first we have to advance.
	}
	// Direct path: at greet, "hello" is the keyword. Switch to menu first.
	f.feed("hello")
	_ = f.mode.Prompt(context.Background(), f.session)
	f.feed("can I take a quest?")
	if !f.mode.flags["started"] {
		t.Fatalf("set_flag did not fire; flags=%v", f.mode.flags)
	}
}

func TestDialogue_ShortInputDoesNotShadowKeywords(t *testing.T) {
	// Regression: an earlier version matched bidirectionally, so a
	// player typing "i" matched any keyword containing "i" (hi, hello,
	// quit, etc.). Confirm the unidirectional match: input must be
	// long enough to contain the keyword, not the other way around.
	f := pushDialogue(t)
	_ = f.mode.Prompt(context.Background(), f.session)
	f.feed("i")
	if f.mode.currentID != "greet" {
		t.Fatalf("single-char input shadowed a keyword: currentID = %q", f.mode.currentID)
	}
	if !strings.Contains(f.captured.String(), "isn't one of your replies") {
		t.Fatalf("expected re-prompt hint for unmatched short input: %q", f.captured.String())
	}
}

func TestDialogue_HiddenResponseGatedByFlag(t *testing.T) {
	f := pushDialogue(t)
	_ = f.mode.Prompt(context.Background(), f.session) // greet
	f.feed("hello")                                    // → menu
	out := f.mode.Prompt(context.Background(), f.session)
	if strings.Contains(out, "reward") {
		t.Fatalf("reward should be hidden before set_flag: %q", out)
	}
	f.feed("quest") // sets started=true, stays at menu
	out = f.mode.Prompt(context.Background(), f.session)
	if !strings.Contains(out, "reward") {
		t.Fatalf("reward should appear after set_flag: %q", out)
	}
}

func TestDialogue_BareEnterAndByeBothPop(t *testing.T) {
	cases := []string{"", "bye", "BYE", "quit", "leave"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			f := pushDialogue(t)
			f.feed(in)
			if _, ok := f.session.CurrentMode().(*Dialogue); ok {
				t.Fatalf("input %q should have popped Dialogue mode", in)
			}
		})
	}
}

func TestDialogue_EmptyNextEndsConversation(t *testing.T) {
	f := pushDialogue(t)
	_ = f.mode.Prompt(context.Background(), f.session)
	f.feed("hello") // greet → menu
	_ = f.mode.Prompt(context.Background(), f.session)
	f.feed("farewell") // empty Next pops mode
	if _, ok := f.session.CurrentMode().(*Dialogue); ok {
		t.Fatalf("empty Next should pop mode")
	}
}

func TestDialogue_UnmatchedInputReprompts(t *testing.T) {
	f := pushDialogue(t)
	_ = f.mode.Prompt(context.Background(), f.session)
	f.feed("nonsense words")
	if f.mode.currentID != "greet" {
		t.Fatalf("currentID changed on unmatched input: %q", f.mode.currentID)
	}
	if !strings.Contains(f.captured.String(), "isn't one of your replies") {
		t.Fatalf("expected re-prompt hint: %q", f.captured.String())
	}
}

func TestDialogue_PushModeInvokesHook(t *testing.T) {
	tree := &dialogue.Tree{
		Root: "root",
		Nodes: map[dialogue.NodeID]dialogue.Node{
			"root": {
				ID:     "root",
				Prompt: "Want to shop?",
				Responses: []dialogue.Response{
					{Match: []string{"shop"}, Effects: []dialogue.Effect{
						{Kind: dialogue.EffectPushMode, Args: map[string]string{"mode": "shop"}},
					}},
				},
			},
		},
	}
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })
	captured := &safeBuf{}
	drainPeer(t, client, captured)
	s := telnet.NewSession(server)

	pushed := struct {
		called bool
		name   string
	}{}
	hook := func(_ context.Context, _ *telnet.Session, name string, _ map[string]string) error {
		pushed.called = true
		pushed.name = name
		return nil
	}
	m, err := NewDialogue("merchant", "tr.merchant", tree, DialogueHooks{PushMode: hook})
	if err != nil {
		t.Fatalf("NewDialogue: %v", err)
	}
	if err := s.PushMode(m); err != nil {
		t.Fatalf("push: %v", err)
	}
	_ = m.Prompt(context.Background(), s)
	if err := m.Handle(context.Background(), s, "shop"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !pushed.called || pushed.name != "shop" {
		t.Fatalf("push hook = %+v", pushed)
	}
}

// Phase F #32 slice 2: dialogue `script` effect should dispatch
// through DialogueHooks.RunScript with the script name from
// effect.Args["script"]. The hook is non-fatal — a returned error
// logs but does not abort the response chain.
func TestDialogue_ScriptEffectInvokesHook(t *testing.T) {
	tree := &dialogue.Tree{
		Root: "root",
		Nodes: map[dialogue.NodeID]dialogue.Node{
			"root": {
				ID:     "root",
				Prompt: "Greetings.",
				Responses: []dialogue.Response{
					{Match: []string{"hint"}, Reply: "Listen well.", Effects: []dialogue.Effect{
						{Kind: dialogue.EffectScript, Args: map[string]string{"script": "warden_alert"}},
					}, Next: "root"},
				},
			},
		},
	}
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })
	captured := &safeBuf{}
	drainPeer(t, client, captured)
	s := telnet.NewSession(server)

	var seen string
	hook := func(_ context.Context, _ *telnet.Session, name string) error {
		seen = name
		return nil
	}
	m, err := NewDialogue("warden", "tr.warden", tree, DialogueHooks{RunScript: hook})
	if err != nil {
		t.Fatalf("NewDialogue: %v", err)
	}
	if err := s.PushMode(m); err != nil {
		t.Fatalf("push: %v", err)
	}
	_ = m.Prompt(context.Background(), s)
	if err := m.Handle(context.Background(), s, "hint"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if seen != "warden_alert" {
		t.Fatalf("hook captured %q, want warden_alert", seen)
	}
}

// Unbound RunScript hook is non-fatal: applyEffects logs a warning
// and continues, so the dialogue stays usable.
func TestDialogue_ScriptEffectUnboundHook_NoFault(t *testing.T) {
	tree := &dialogue.Tree{
		Root: "root",
		Nodes: map[dialogue.NodeID]dialogue.Node{
			"root": {
				ID:     "root",
				Prompt: "Greetings.",
				Responses: []dialogue.Response{
					{Match: []string{"hint"}, Reply: "Listen well.", Effects: []dialogue.Effect{
						{Kind: dialogue.EffectScript, Args: map[string]string{"script": "warden_alert"}},
					}, Next: "root"},
				},
			},
		},
	}
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })
	captured := &safeBuf{}
	drainPeer(t, client, captured)
	s := telnet.NewSession(server)

	m, err := NewDialogue("warden", "tr.warden", tree, DialogueHooks{})
	if err != nil {
		t.Fatalf("NewDialogue: %v", err)
	}
	if err := s.PushMode(m); err != nil {
		t.Fatalf("push: %v", err)
	}
	_ = m.Prompt(context.Background(), s)
	if err := m.Handle(context.Background(), s, "hint"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}
