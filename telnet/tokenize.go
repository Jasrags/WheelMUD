package telnet

import (
	"errors"
	"strings"
)

// ErrUnbalancedQuote is returned by Tokenize when a quoted token is not
// closed by the end of input. Dispatch surfaces this to the user.
var ErrUnbalancedQuote = errors.New("telnet: unbalanced quote")

// Tokenize splits s into shell-style argument tokens. It supports:
//   - whitespace as a token separator (any run of spaces/tabs)
//   - "double quotes" — preserves spaces; supports \" \\ \n \t \r escapes
//   - 'single quotes' — preserves spaces; no escape processing
//   - bare backslash before any byte outside quotes escapes that byte
//
// Empty input produces a nil slice.
func Tokenize(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	var (
		out    []string
		cur    strings.Builder
		inTok  bool
		quote  byte // 0, '"', or '\''
	)
	flush := func() {
		if inTok {
			out = append(out, cur.String())
			cur.Reset()
			inTok = false
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote == 0 {
			switch {
			case c == ' ' || c == '\t':
				flush()
				continue
			case c == '"' || c == '\'':
				quote = c
				inTok = true
				continue
			case c == '\\' && i+1 < len(s):
				cur.WriteByte(s[i+1])
				i++
				inTok = true
				continue
			}
			cur.WriteByte(c)
			inTok = true
			continue
		}

		if c == quote {
			quote = 0
			continue
		}
		if quote == '"' && c == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case 'n':
				cur.WriteByte('\n')
			case 't':
				cur.WriteByte('\t')
			case 'r':
				cur.WriteByte('\r')
			default:
				cur.WriteByte(next)
			}
			i++
			continue
		}
		cur.WriteByte(c)
	}

	if quote != 0 {
		return nil, ErrUnbalancedQuote
	}
	flush()
	return out, nil
}

// SplitOnSemicolon splits s on top-level `;` separators, respecting the
// same quote/escape rules as Tokenize: a `;` inside double or single
// quotes (or escaped with `\;` outside quotes) does not split. Each
// returned segment is trimmed of surrounding whitespace; empty segments
// are dropped (so `;;`, leading/trailing `;`, and runs of `;` are
// no-ops). Returns ErrUnbalancedQuote when a quoted region is not
// closed by end of input.
//
// Common case (no `;` at top level) returns a single-element slice
// containing s as-is.
func SplitOnSemicolon(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	var (
		out   []string
		cur   strings.Builder
		quote byte // 0, '"', or '\''
	)
	flush := func() {
		seg := strings.TrimSpace(cur.String())
		if seg != "" {
			out = append(out, seg)
		}
		cur.Reset()
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote == 0 {
			if c == ';' {
				flush()
				continue
			}
			if c == '"' || c == '\'' {
				quote = c
				cur.WriteByte(c)
				continue
			}
			if c == '\\' && i+1 < len(s) {
				cur.WriteByte(c)
				cur.WriteByte(s[i+1])
				i++
				continue
			}
			cur.WriteByte(c)
			continue
		}
		// Inside quotes: copy verbatim. Tokenize will re-parse later;
		// our job is just to find segment boundaries.
		cur.WriteByte(c)
		if c == quote {
			quote = 0
			continue
		}
		if quote == '"' && c == '\\' && i+1 < len(s) {
			cur.WriteByte(s[i+1])
			i++
		}
	}
	if quote != 0 {
		return nil, ErrUnbalancedQuote
	}
	flush()
	return out, nil
}

// CompletionPartial returns the trailing partial token at the end of buf
// for tab completion, along with whether the buffer currently sits inside
// an open quote. The partial includes the opening quote byte when
// inQuote is true (e.g., `say "hel` → partial=`"hel`); callers doing a
// prefix match against unquoted candidate text should strip the leading
// quote first. Whitespace at the very end yields an empty partial.
func CompletionPartial(buf string) (partial string, inQuote bool) {
	var quote byte
	tokenStart := 0
	hasToken := false
	for i := 0; i < len(buf); i++ {
		c := buf[i]
		if quote == 0 {
			switch c {
			case ' ', '\t':
				hasToken = false
				continue
			case '"', '\'':
				if !hasToken {
					tokenStart = i
					hasToken = true
				}
				quote = c
				continue
			case '\\':
				if !hasToken {
					tokenStart = i
					hasToken = true
				}
				if i+1 < len(buf) {
					i++
				}
				continue
			}
			if !hasToken {
				tokenStart = i
				hasToken = true
			}
			continue
		}
		if c == quote {
			quote = 0
			continue
		}
		if quote == '"' && c == '\\' && i+1 < len(buf) {
			i++
			continue
		}
	}
	if !hasToken {
		return "", quote != 0
	}
	return buf[tokenStart:], quote != 0
}
