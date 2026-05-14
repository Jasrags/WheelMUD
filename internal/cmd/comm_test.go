package cmd

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// commPair builds two sessions bound to the same registry and
// returns them along with their captured-output bufConns. Each
// session goes through the dispatcher so AuthLevel + LastInputAt
// stamping match the production path.
func commPair(t *testing.T) (sessions *session.Registry, alice, bob *telnet.Session, aOut, bOut *bufConn) {
	t.Helper()
	sessions = session.NewRegistry()

	a, aConn := bufSession(t)
	a.AccountID = 100
	a.AuthLevel = telnet.AuthPlayer
	a.CharacterID = 1
	a.CharacterName = "Alice"
	a.CurrentRoomID = 1
	sessions.Bind(a.AccountID, a)

	b, bConn := bufSession(t)
	b.AccountID = 200
	b.AuthLevel = telnet.AuthPlayer
	b.CharacterID = 2
	b.CharacterName = "Bob"
	b.CurrentRoomID = 1
	sessions.Bind(b.AccountID, b)

	return sessions, a, b, aConn, bConn
}

// runCmd mirrors telnet.Registry.Dispatch from the perspective of
// individual command tests. Convention: callers pass `raw` as the
// post-verb argument string only (NOT the full line with verb), so
// `runCmd(t, NewGoto(...), s, "Player")` is equivalent to a real
// session typing `goto Player`. Args is tokenized from raw; Context.Raw
// matches what the production dispatcher sets — the post-verb
// remainder. A leading verb in `raw` would be tokenized as the first
// arg, which is almost never what tests want.
func runCmd(t *testing.T, c *telnet.Command, s *telnet.Session, raw string) {
	t.Helper()
	raw = strings.TrimSpace(raw)
	args, err := telnet.Tokenize(raw)
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	ctx := &telnet.Context{
		Ctx:     context.Background(),
		Session: s,
		Name:    c.Name,
		Args:    args,
		Raw:     raw,
	}
	if err := c.Run(ctx); err != nil {
		t.Fatalf("%s.Run: %v", c.Name, err)
	}
}

func TestSay_BroadcastsToSameRoom(t *testing.T) {
	sessions, alice, _, aOut, bOut := commPair(t)
	say := NewSay(sessions, repo.NewMemoryRoomRepo(), nil)

	runCmd(t, say, alice, "hello there")

	if !strings.Contains(aOut.String(), "You say,") {
		t.Fatalf("alice: missing self echo; got %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "Alice says,") {
		t.Fatalf("bob: missing speaker line; got %q", bOut.String())
	}
	if !strings.Contains(bOut.String(), "hello there") {
		t.Fatalf("bob: missing payload; got %q", bOut.String())
	}
}

func TestSay_SilentRoomBlocks(t *testing.T) {
	sessions, alice, _, aOut, bOut := commPair(t)
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Hush Chapel", Flags: repo.RoomFlags{Silent: true}})
	say := NewSay(sessions, rooms, nil)

	runCmd(t, say, alice, "anyone here?")

	if !strings.Contains(aOut.String(), "smothers your words") {
		t.Fatalf("alice: missing silent message; got %q", aOut.String())
	}
	if strings.Contains(bOut.String(), "anyone here") {
		t.Fatalf("bob heard speech in silent room: %q", bOut.String())
	}
}

func TestSay_BroadcastsWhenRoomMissing(t *testing.T) {
	// A FindByID miss (e.g. soft-deleted room) is intentionally
	// allowed to broadcast — speech still reaches peers in the same
	// logical CurrentRoomID. Pin the behavior so a future refactor
	// can't silently regress it to "smother".
	sessions, alice, _, _, bOut := commPair(t)
	emptyRooms := repo.NewMemoryRoomRepo() // no row for room 1
	say := NewSay(sessions, emptyRooms, nil)

	runCmd(t, say, alice, "still here?")

	if !strings.Contains(bOut.String(), "still here?") {
		t.Fatalf("bob did not hear speech under ErrRoomNotFound: %q", bOut.String())
	}
}

func TestSay_PublishesPlayerSaid(t *testing.T) {
	sessions, alice, _, _, _ := commPair(t)
	bus := eventbus.New()
	var got atomic.Int32
	var captured world.PlayerSaid
	eventbus.Subscribe[world.PlayerSaid](bus, func(_ context.Context, ev world.PlayerSaid) {
		captured = ev
		got.Add(1)
	})
	say := NewSay(sessions, repo.NewMemoryRoomRepo(), bus)

	runCmd(t, say, alice, "rumor of trolloks")

	if got.Load() != 1 {
		t.Fatalf("expected 1 PlayerSaid event, got %d", got.Load())
	}
	if captured.SpeakerCharacterID != alice.CharacterID || captured.RoomID != alice.CurrentRoomID {
		t.Fatalf("event: %+v", captured)
	}
	if !strings.Contains(captured.Text, "rumor") {
		t.Fatalf("event text: %q", captured.Text)
	}
}

func TestSay_SilentRoomDoesNotPublish(t *testing.T) {
	sessions, alice, _, _, _ := commPair(t)
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Hush", Flags: repo.RoomFlags{Silent: true}})
	bus := eventbus.New()
	var got atomic.Int32
	eventbus.Subscribe[world.PlayerSaid](bus, func(_ context.Context, _ world.PlayerSaid) {
		got.Add(1)
	})
	say := NewSay(sessions, rooms, bus)

	runCmd(t, say, alice, "anyone?")

	if got.Load() != 0 {
		t.Fatalf("silent room published PlayerSaid (%d)", got.Load())
	}
}

func TestSay_DoesNotReachOtherRooms(t *testing.T) {
	sessions, alice, bob, _, bOut := commPair(t)
	bob.CurrentRoomID = 99 // different room
	say := NewSay(sessions, repo.NewMemoryRoomRepo(), nil)

	runCmd(t, say, alice, "anyone there?")

	if strings.Contains(bOut.String(), "anyone there") {
		t.Fatalf("bob got cross-room say: %q", bOut.String())
	}
}

func TestTell_DeliversAndSetsLastTellFrom(t *testing.T) {
	sessions, alice, bob, aOut, bOut := commPair(t)
	tell := NewTell(sessions, nil)

	runCmd(t, tell, alice, "Bob this is private")

	if !strings.Contains(aOut.String(), "You tell Bob") {
		t.Fatalf("alice: missing self echo; got %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "Alice tells you") {
		t.Fatalf("bob: missing recipient line; got %q", bOut.String())
	}
	if !strings.Contains(bOut.String(), "this is private") {
		t.Fatalf("bob: missing payload; got %q", bOut.String())
	}
	if got := bob.LastTellFrom(); got != "Alice" {
		t.Fatalf("LastTellFrom = %q, want Alice", got)
	}
}

func TestTell_UnknownTargetFriendlyError(t *testing.T) {
	sessions, alice, _, aOut, _ := commPair(t)
	tell := NewTell(sessions, nil)

	runCmd(t, tell, alice, "Ghost are you there?")

	if !strings.Contains(aOut.String(), "no one by that name") {
		t.Fatalf("missing friendly error; got %q", aOut.String())
	}
}

func TestReply_RoutesToLastTellFrom(t *testing.T) {
	sessions, alice, bob, aOut, bOut := commPair(t)
	tell := NewTell(sessions, nil)
	reply := NewReply(sessions, nil)

	runCmd(t, tell, alice, "Bob hi")
	bOut.Reset()
	aOut.Reset()
	runCmd(t, reply, bob, "right back at you")

	if !strings.Contains(aOut.String(), "Bob tells you") {
		t.Fatalf("alice: missing reply receipt; got %q", aOut.String())
	}
	if !strings.Contains(aOut.String(), "right back at you") {
		t.Fatalf("alice: missing reply payload; got %q", aOut.String())
	}
	if got := alice.LastTellFrom(); got != "Bob" {
		t.Fatalf("alice.LastTellFrom = %q, want Bob", got)
	}
}

func TestReply_NoLastTellFromIsFriendly(t *testing.T) {
	sessions, _, bob, _, bOut := commPair(t)
	reply := NewReply(sessions, nil)

	runCmd(t, reply, bob, "anyone?")

	if !strings.Contains(bOut.String(), "no one to reply to") {
		t.Fatalf("missing friendly error; got %q", bOut.String())
	}
}

func TestSanitizeChat(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"", "", false},
		{"   \t\r ", "", false},
		{"hi", "hi", true},
		// control bytes stripped
		{"hi\x01\x07there", "hithere", true},
		// cfmt syntax defanged
		// `{{` → `{ {`, then `}}` → `} }`, then `::` → `: :`. The
		// second replacement runs over the output of the first, so
		// `}}::` collapses to `} }: :` (no space between `}` and `:`).
		{"{{red}}::red boom", "{ {red} }: :red boom", true},
		// trim surrounding whitespace
		{"   spaced out   ", "spaced out", true},
	}
	for _, tc := range cases {
		got, ok := sanitizeChat(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("sanitizeChat(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
