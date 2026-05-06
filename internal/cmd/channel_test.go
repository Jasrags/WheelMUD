package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

var oocChannel = repo.Channel{ID: 1, Name: "ooc", Color: "cyan"}

func TestChannel_BroadcastReachesOthers(t *testing.T) {
	sessions, alice, _, aOut, bOut := commPair(t)
	chars := repo.NewMemoryCharacterRepo()
	cmd := NewChannel(oocChannel, sessions, chars)

	runCmd(t, cmd, alice, "anyone awake")

	if !strings.Contains(aOut.String(), "[OOC] You:") {
		t.Fatalf("alice missing self echo; got %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "[OOC] Alice:") {
		t.Fatalf("bob missing peer line; got %q", bOut.String())
	}
	if !strings.Contains(bOut.String(), "anyone awake") {
		t.Fatalf("bob missing payload; got %q", bOut.String())
	}
}

func TestChannel_NoArgsTogglesAndPersists(t *testing.T) {
	ctx := context.Background()
	sessions, alice, _, aOut, _ := commPair(t)
	chars := repo.NewMemoryCharacterRepo()
	acc := int64(7)
	created, err := chars.Create(ctx, repo.Character{AccountID: acc, Name: "Alice"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	alice.CharacterID = created.ID
	cmd := NewChannel(oocChannel, sessions, chars)

	runCmd(t, cmd, alice, "")
	if !alice.IsChannelMuted("ooc") {
		t.Fatalf("first toggle should mute ooc")
	}
	if !strings.Contains(aOut.String(), "ooc is now off") {
		t.Fatalf("missing toggle echo; got %q", aOut.String())
	}
	got, _ := chars.FindByName(ctx, "Alice")
	if !got.ChannelSettings["ooc"] {
		t.Fatalf("RecordChannelSettings did not persist mute: %+v", got.ChannelSettings)
	}

	aOut.Reset()
	runCmd(t, cmd, alice, "")
	if alice.IsChannelMuted("ooc") {
		t.Fatalf("second toggle should unmute ooc")
	}
	if !strings.Contains(aOut.String(), "ooc is now on") {
		t.Fatalf("missing toggle echo; got %q", aOut.String())
	}
	got, _ = chars.FindByName(ctx, "Alice")
	if got.ChannelSettings["ooc"] {
		t.Fatalf("RecordChannelSettings did not persist unmute: %+v", got.ChannelSettings)
	}
}

func TestChannel_MutedPeerSkipped(t *testing.T) {
	sessions, alice, bob, _, bOut := commPair(t)
	chars := repo.NewMemoryCharacterRepo()
	cmd := NewChannel(oocChannel, sessions, chars)

	bob.SetChannelMuted(map[string]bool{"ooc": true})

	runCmd(t, cmd, alice, "anyone there")
	if strings.Contains(bOut.String(), "anyone there") {
		t.Fatalf("muted peer received message: %q", bOut.String())
	}
}

func TestChannel_SpeakingWhileMutedRefuses(t *testing.T) {
	sessions, alice, _, aOut, _ := commPair(t)
	chars := repo.NewMemoryCharacterRepo()
	cmd := NewChannel(oocChannel, sessions, chars)

	alice.SetChannelMuted(map[string]bool{"ooc": true})
	runCmd(t, cmd, alice, "hi all")
	if !strings.Contains(aOut.String(), "channel is off") {
		t.Fatalf("expected refusal; got %q", aOut.String())
	}
}

func TestChannelsList_RendersStateForCaller(t *testing.T) {
	sessions, alice, _, aOut, _ := commPair(t)
	_ = sessions
	alice.SetChannelMuted(map[string]bool{"gossip": true})
	// Strip ANSI so test assertions don't fight the cfmt layer; the
	// per-segment styling splits "ooc — on" across SGR boundaries.
	alice.ColorLevel = telnet.ColorLevelNone
	cat := []repo.Channel{
		{Name: "ooc", Color: "cyan"},
		{Name: "gossip", Color: "magenta"},
		{Name: "newbie", Color: "green"},
	}
	cmd := NewChannelsList(cat)
	runCmd(t, cmd, alice, "")
	got := aOut.String()
	if !strings.Contains(got, "Channels") {
		t.Fatalf("missing section header; got %q", got)
	}
	if !strings.Contains(got, "ooc — on") {
		t.Fatalf("expected ooc on; got %q", got)
	}
	if !strings.Contains(got, "gossip — off") {
		t.Fatalf("expected gossip off; got %q", got)
	}
	if !strings.Contains(got, "newbie — on") {
		t.Fatalf("expected newbie on; got %q", got)
	}
}
