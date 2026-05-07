package combat

import "testing"

// TestExpandTallyByGroup_SoloCharacter verifies the no-op path: a
// resolver that returns just the dealer's own id (the slice-1 default
// for ungrouped characters) leaves the tally alone.
func TestExpandTallyByGroup_SoloCharacter(t *testing.T) {
	dealer := ActorRef{Kind: ActorKindCharacter, ID: 1}
	in := map[ActorRef]int32{dealer: 100}
	resolver := func(id, _ int64) []int64 { return []int64{id} }

	out := expandTallyByGroup(in, 99, resolver)
	if got := out[dealer]; got != 100 {
		t.Fatalf("dealer share = %d, want 100", got)
	}
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
}

// TestExpandTallyByGroup_TwoMemberSplit checks the canonical case:
// a single dealer (alice) belongs to a 2-member party. Their 100
// damage should re-credit 50 to each member after expansion.
func TestExpandTallyByGroup_TwoMemberSplit(t *testing.T) {
	alice := ActorRef{Kind: ActorKindCharacter, ID: 1}
	bob := ActorRef{Kind: ActorKindCharacter, ID: 2}
	in := map[ActorRef]int32{alice: 100}
	resolver := func(id, _ int64) []int64 { return []int64{1, 2} }

	out := expandTallyByGroup(in, 99, resolver)
	if got := out[alice]; got != 50 {
		t.Fatalf("alice share = %d, want 50", got)
	}
	if got := out[bob]; got != 50 {
		t.Fatalf("bob share = %d, want 50", got)
	}
}

// TestExpandTallyByGroup_RemainderToDealer — an odd damage total
// (3 dmg / 2 members) splits 1+1 with the remainder credited to the
// dealer so totals don't drift.
func TestExpandTallyByGroup_RemainderToDealer(t *testing.T) {
	alice := ActorRef{Kind: ActorKindCharacter, ID: 1}
	bob := ActorRef{Kind: ActorKindCharacter, ID: 2}
	in := map[ActorRef]int32{alice: 3}
	resolver := func(id, _ int64) []int64 { return []int64{1, 2} }

	out := expandTallyByGroup(in, 99, resolver)
	if got := out[alice]; got != 2 { // 1 + 1 remainder
		t.Fatalf("alice share = %d, want 2", got)
	}
	if got := out[bob]; got != 1 {
		t.Fatalf("bob share = %d, want 1", got)
	}
}

// TestExpandTallyByGroup_PartialRoomMembership only credits group
// members the resolver lists for that room — an AFK teammate
// elsewhere doesn't pull XP from a far-away kill.
func TestExpandTallyByGroup_PartialRoomMembership(t *testing.T) {
	alice := ActorRef{Kind: ActorKindCharacter, ID: 1}
	bob := ActorRef{Kind: ActorKindCharacter, ID: 2}
	carol := ActorRef{Kind: ActorKindCharacter, ID: 3}
	in := map[ActorRef]int32{alice: 100}
	// Carol is in the party but NOT in this room — resolver returns
	// alice + bob only.
	resolver := func(id, _ int64) []int64 { return []int64{1, 2} }

	out := expandTallyByGroup(in, 99, resolver)
	if got := out[alice]; got != 50 {
		t.Fatalf("alice share = %d, want 50", got)
	}
	if got := out[bob]; got != 50 {
		t.Fatalf("bob share = %d, want 50", got)
	}
	if got := out[carol]; got != 0 {
		t.Fatalf("absent carol share = %d, want 0", got)
	}
}

// TestExpandTallyByGroup_NonCharacterPassthrough — mob / unknown
// contributors aren't expanded.
func TestExpandTallyByGroup_NonCharacterPassthrough(t *testing.T) {
	mob := ActorRef{Kind: ActorKindMob, ID: 7}
	in := map[ActorRef]int32{mob: 50}
	resolver := func(id, _ int64) []int64 { return []int64{id, 999} }

	out := expandTallyByGroup(in, 99, resolver)
	if got := out[mob]; got != 50 {
		t.Fatalf("mob share = %d, want 50 (no expansion)", got)
	}
	if len(out) != 1 {
		t.Fatalf("non-character should not have spawned new entries: %v", out)
	}
}

// TestExpandTallyByGroup_NilResolverNoOp — backwards-compat: nil
// resolver returns the input verbatim.
func TestExpandTallyByGroup_NilResolverNoOp(t *testing.T) {
	alice := ActorRef{Kind: ActorKindCharacter, ID: 1}
	in := map[ActorRef]int32{alice: 100}
	out := expandTallyByGroup(in, 99, nil)
	if got := out[alice]; got != 100 {
		t.Fatalf("nil resolver should leave tally alone; got %d", got)
	}
}

// TestExpandTallyByGroup_TwoDealersBothGrouped — both dealers belong
// to the same 2-person party; their tallies expand independently and
// accumulate at the shared teammate. Verifies the additive map write.
func TestExpandTallyByGroup_TwoDealersBothGrouped(t *testing.T) {
	alice := ActorRef{Kind: ActorKindCharacter, ID: 1}
	bob := ActorRef{Kind: ActorKindCharacter, ID: 2}
	in := map[ActorRef]int32{alice: 40, bob: 60}
	resolver := func(id, _ int64) []int64 { return []int64{1, 2} }

	out := expandTallyByGroup(in, 99, resolver)
	// Alice: 20 (own half) + 30 (Bob's half) = 50
	// Bob:   20 (Alice's half) + 30 (own half) = 50
	if got := out[alice]; got != 50 {
		t.Fatalf("alice combined share = %d, want 50", got)
	}
	if got := out[bob]; got != 50 {
		t.Fatalf("bob combined share = %d, want 50", got)
	}
}
