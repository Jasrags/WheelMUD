package auth

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	// Drop bcrypt cost so the test suite stays fast. Production callers
	// hit DefaultCost via init().
	prev := SetCost(bcrypt.MinCost)
	defer SetCost(prev)
	m.Run()
}

func TestHash_RoundTrip(t *testing.T) {
	pw := "correct-horse"
	h, err := Hash(pw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if h == pw {
		t.Fatal("hash equals plaintext")
	}
	if !Verify(h, pw) {
		t.Fatal("Verify rejected the matching password")
	}
}

func TestHash_DifferentSaltsProduceDifferentHashes(t *testing.T) {
	pw := "correct-horse"
	a, _ := Hash(pw)
	b, _ := Hash(pw)
	if a == b {
		t.Fatal("two hashes of the same password collided (bcrypt should salt)")
	}
}

func TestHash_TooShort(t *testing.T) {
	_, err := Hash("short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("err = %v, want ErrPasswordTooShort", err)
	}
}

func TestHash_TooLong(t *testing.T) {
	pw := strings.Repeat("a", MaxPasswordLen+1)
	_, err := Hash(pw)
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("err = %v, want ErrPasswordTooLong", err)
	}
}

func TestHash_BoundaryAccepted(t *testing.T) {
	// Exactly MaxPasswordLen bytes is valid (bcrypt's hard ceiling).
	pw := strings.Repeat("a", MaxPasswordLen)
	if _, err := Hash(pw); err != nil {
		t.Fatalf("MaxPasswordLen rejected: %v", err)
	}
	// Exactly MinPasswordLen runes is valid.
	pw = strings.Repeat("a", MinPasswordLen)
	if _, err := Hash(pw); err != nil {
		t.Fatalf("MinPasswordLen rejected: %v", err)
	}
}

func TestVerify_RejectsMismatch(t *testing.T) {
	h, _ := Hash("correct-horse")
	if Verify(h, "battery-staple") {
		t.Fatal("Verify accepted wrong password")
	}
}

func TestVerify_RejectsMalformedHash(t *testing.T) {
	if Verify("not-a-bcrypt-hash", "correct-horse") {
		t.Fatal("Verify accepted bogus hash")
	}
}

func TestVerify_RejectsEmptyPassword(t *testing.T) {
	h, _ := Hash("correct-horse")
	if Verify(h, "") {
		t.Fatal("Verify accepted empty password")
	}
}

func TestVerify_RejectsOversizedPassword(t *testing.T) {
	h, _ := Hash("correct-horse")
	pw := strings.Repeat("a", MaxPasswordLen+10)
	if Verify(h, pw) {
		t.Fatal("Verify accepted oversized password")
	}
}

func TestHash_UTF8RuneCountForMin(t *testing.T) {
	// 8 multi-byte runes — under the byte threshold but at the rune
	// threshold. Should be accepted.
	pw := strings.Repeat("é", MinPasswordLen) // 16 bytes / 8 runes
	if _, err := Hash(pw); err != nil {
		t.Fatalf("UTF-8 password rejected: %v", err)
	}
}
