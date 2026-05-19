package cmd

import (
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/telnet"
)

func TestWizdemo_DispatchesToPushFlow(t *testing.T) {
	var calledFlow string
	push := func(_ *telnet.Session, flowID string) error {
		calledFlow = flowID
		return nil
	}
	cmd := NewWizdemo(push)
	if cmd.Auth != telnet.AuthPlayer {
		t.Fatalf("wizdemo auth = %v, want AuthPlayer", cmd.Auth)
	}
	_, alice, _, _, _ := commPair(t)
	runCmd(t, cmd, alice, "")
	if calledFlow != "wizdemo" {
		t.Fatalf("push received flowID=%q, want wizdemo", calledFlow)
	}
}

func TestFlowVerb_NoArgsLists(t *testing.T) {
	push := func(*telnet.Session, string) error { return nil }
	list := func() []string { return []string{"alpha", "wizdemo"} }
	cmd := NewFlowVerb(push, list)
	if cmd.Auth != telnet.AuthAdmin {
		t.Fatalf("flow verb auth = %v, want AuthAdmin", cmd.Auth)
	}
	_, alice, _, aOut, _ := commPair(t)
	runCmd(t, cmd, alice, "")
	got := aOut.String()
	if !strings.Contains(got, "alpha") || !strings.Contains(got, "wizdemo") {
		t.Fatalf("flow list missing entries: %q", got)
	}
}

func TestFlowVerb_NoArgsEmptyCatalog(t *testing.T) {
	push := func(*telnet.Session, string) error { return nil }
	list := func() []string { return nil }
	cmd := NewFlowVerb(push, list)
	_, alice, _, aOut, _ := commPair(t)
	runCmd(t, cmd, alice, "")
	if !strings.Contains(aOut.String(), "No flows loaded") {
		t.Fatalf("empty-catalog message missing: %q", aOut.String())
	}
}

func TestFlowVerb_ArgLaunches(t *testing.T) {
	var called string
	push := func(_ *telnet.Session, flowID string) error {
		called = flowID
		return nil
	}
	list := func() []string { return []string{"wizdemo"} }
	cmd := NewFlowVerb(push, list)
	_, alice, _, _, _ := commPair(t)
	runCmd(t, cmd, alice, "wizdemo")
	if called != "wizdemo" {
		t.Fatalf("push called with %q, want wizdemo", called)
	}
}

func TestFlowVerb_CompleterFiltersByPrefix(t *testing.T) {
	push := func(*telnet.Session, string) error { return nil }
	list := func() []string { return []string{"alpha", "beta", "wizdemo"} }
	cmd := NewFlowVerb(push, list)
	_, alice, _, _, _ := commPair(t)
	candidates := cmd.Completer(alice, "w")
	if len(candidates) != 1 || candidates[0].Text != "wizdemo" {
		t.Fatalf("completer for 'w' = %+v, want [wizdemo]", candidates)
	}
}
