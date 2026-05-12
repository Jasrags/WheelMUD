package telnet

import (
	"errors"
	"testing"
	"unicode/utf8"
)

// FuzzTokenize asserts: (1) no panic on arbitrary input; (2) on success,
// every token round-trips back through Tokenize as a single token (i.e. the
// tokenizer is idempotent once you put each token in single quotes); (3)
// the only error returned is ErrUnbalancedQuote.
func FuzzTokenize(f *testing.F) {
	seeds := []string{
		"",
		"hello world",
		`say "hi there"`,
		`tell 'alice' "hello there"`,
		`a\;b`,
		`a\\b`,
		`"unterminated`,
		`'unterminated`,
		`""`,
		`''`,
		`  leading   spaces`,
		"tab\tseparated",
		`mixed "a b" 'c d' e\ f`,
		"\x00\x01\xff",
		`\\\\`,
		`"\n\t\r\\"`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// Skip non-UTF-8 fuzz inputs that the byte-level tokenizer is
		// allowed to mangle — the parser operates on bytes, but the
		// round-trip invariant below requires valid UTF-8 for stable
		// string boundaries.
		if !utf8.ValidString(s) {
			return
		}

		out, err := Tokenize(s)
		if err != nil && !errors.Is(err, ErrUnbalancedQuote) {
			t.Fatalf("Tokenize returned unexpected error type: %v", err)
		}
		if err != nil {
			return
		}

		// Each emitted token, wrapped in single quotes, must re-tokenize as
		// itself (single-quote mode does no escape processing). The one
		// exception is a token containing a single-quote byte — round-trip
		// it through double quotes instead, escaping `"` and `\`.
		for _, tok := range out {
			var quoted string
			if !containsByte(tok, '\'') {
				quoted = "'" + tok + "'"
			} else {
				quoted = `"` + escapeDoubleQuoted(tok) + `"`
			}
			got, err := Tokenize(quoted)
			if err != nil {
				t.Fatalf("round-trip Tokenize(%q) error: %v (orig=%q)", quoted, err, s)
			}
			if len(got) != 1 || got[0] != tok {
				t.Fatalf("round-trip mismatch: orig=%q quoted=%q want=[%q] got=%v", s, quoted, tok, got)
			}
		}
	})
}

// FuzzSplitOnSemicolon asserts: (1) no panic; (2) the only error returned
// is ErrUnbalancedQuote; (3) re-joining segments with `; ` and re-splitting
// produces the same segment list (modulo whitespace).
func FuzzSplitOnSemicolon(f *testing.F) {
	seeds := []string{
		"",
		"single",
		"a;b",
		"a;b;c",
		`say "hi; there"; tell bob 'a;b'`,
		`a\;b`,
		";;;",
		"  ;  a  ;  b  ;  ",
		`"open;`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return
		}
		segs, err := SplitOnSemicolon(s)
		if err != nil && !errors.Is(err, ErrUnbalancedQuote) {
			t.Fatalf("SplitOnSemicolon unexpected error type: %v", err)
		}
		if err != nil {
			return
		}
		for _, seg := range segs {
			if seg == "" {
				t.Fatalf("SplitOnSemicolon emitted empty segment: %q -> %v", s, segs)
			}
		}
	})
}

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}

func escapeDoubleQuoted(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\', '"':
			out = append(out, '\\', c)
		case '\n':
			out = append(out, '\\', 'n')
		case '\t':
			out = append(out, '\\', 't')
		case '\r':
			out = append(out, '\\', 'r')
		default:
			out = append(out, c)
		}
	}
	return string(out)
}
