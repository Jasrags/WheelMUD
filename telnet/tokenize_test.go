package telnet

import (
	"errors"
	"reflect"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace only", "   \t  ", nil},
		{"single", "hello", []string{"hello"}},
		{"two", "hello world", []string{"hello", "world"}},
		{"runs of whitespace", "a   b\t\tc", []string{"a", "b", "c"}},
		{"double quoted", `say "hello world"`, []string{"say", "hello world"}},
		{"single quoted", `say 'hello "world"'`, []string{"say", `hello "world"`}},
		{"escaped quote in double", `say "she said \"hi\""`, []string{"say", `she said "hi"`}},
		{"escape sequences in double", `x "a\nb\tc"`, []string{"x", "a\nb\tc"}},
		{"backslash outside quotes", `a\ b c`, []string{"a b", "c"}},
		{"empty quoted string", `say ""`, []string{"say", ""}},
		{"adjacent quoted segments", `"foo""bar"`, []string{"foobar"}},
		{"single quotes preserve backslash", `'a\nb'`, []string{`a\nb`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Tokenize(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Tokenize(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestTokenizeUnbalanced(t *testing.T) {
	cases := []string{
		`say "hello`,
		`say 'hello`,
		`say "a \"`,
	}
	for _, c := range cases {
		_, err := Tokenize(c)
		if !errors.Is(err, ErrUnbalancedQuote) {
			t.Errorf("Tokenize(%q) err = %v, want ErrUnbalancedQuote", c, err)
		}
	}
}

func TestSplitOnSemicolon(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "look", []string{"look"}},
		{"three", "look; n; say hi", []string{"look", "n", "say hi"}},
		{"runs of semicolons", ";;look;;n;;", []string{"look", "n"}},
		{"leading and trailing", "  ; look ; n ;  ", []string{"look", "n"}},
		{"semicolon in double quotes", `say "hello; world"`, []string{`say "hello; world"`}},
		{"semicolon in single quotes", `say 'a;b;c'`, []string{`say 'a;b;c'`}},
		{"escaped quote in double", `say "she said \"hi; bye\""`, []string{`say "she said \"hi; bye\""`}},
		{"escaped semicolon outside quotes", `say a\;b`, []string{`say a\;b`}},
		{"mixed quoted and unquoted", `a "x;y" ; b`, []string{`a "x;y"`, "b"}},
		{"all whitespace segments", " ; ; ; ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SplitOnSemicolon(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitOnSemicolon(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitOnSemicolonUnbalanced(t *testing.T) {
	cases := []string{
		`say "hello; world`,
		`say 'a;b`,
	}
	for _, c := range cases {
		_, err := SplitOnSemicolon(c)
		if !errors.Is(err, ErrUnbalancedQuote) {
			t.Errorf("SplitOnSemicolon(%q) err = %v, want ErrUnbalancedQuote", c, err)
		}
	}
}

func TestCompletionPartialQuoteAware(t *testing.T) {
	tests := []struct {
		in       string
		partial  string
		inQuote  bool
	}{
		{"", "", false},
		{"loo", "loo", false},
		{"look ", "", false},
		{"look nor", "nor", false},
		{"say \"hel", "\"hel", true},
		{"say \"hello world", "\"hello world", true},
		{"say \"hello\" wor", "wor", false},
		{"a b\\ c", "b\\ c", false},
	}
	for _, tt := range tests {
		got, q := CompletionPartial(tt.in)
		if got != tt.partial || q != tt.inQuote {
			t.Errorf("CompletionPartial(%q) = (%q,%v), want (%q,%v)",
				tt.in, got, q, tt.partial, tt.inQuote)
		}
	}
}
