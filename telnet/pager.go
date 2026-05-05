package telnet

import (
	"context"
	"strings"
	"sync"
)

// pagerMode chunks a long output buffer to Session.Height-1 lines per
// page, prompting `--More--` between pages and accepting a tiny
// vocabulary (space/enter to advance, q to quit). Pushed onto the
// session mode stack by Session.WritePaged when the body would
// overflow the visible window; popped automatically when the
// remainder is drained, or eagerly on `q`.
//
// Concurrency: a pagerMode instance is owned by exactly one session
// for its lifetime. The mu guards remaining against the Mode methods
// being called by the dispatcher (Handle, Prompt) on a different
// goroutine than OnEnter (which runs synchronously inside PushMode).
// In practice that's overkill — PushMode finishes OnEnter before any
// Handle can fire — but the lock is cheap and keeps invariants
// obvious.
type pagerMode struct {
	mu        sync.Mutex
	remaining []string // each element already ends in "\r\n"
	pageSize  int      // lines per page: max(Height-1, 1)
}

// pagerMoreLine is what Prompt() returns between pages. Kept short so
// it fits even on a 40-column terminal. Trailing space lets the
// terminal cursor sit one column past the dashes, which is the
// convention every terminal pager I know follows.
const pagerMoreLine = "-- More -- (space=next, q=quit) "

func newPagerMode(remaining []string, height int) *pagerMode {
	page := height - 1
	if page < 1 {
		page = 1
	}
	return &pagerMode{remaining: remaining, pageSize: page}
}

// OnEnter writes the first page. The dispatcher will paint our
// Prompt() right after, so the player sees [page] + `--More--`.
// We ignore the done bool: even on a body that fits in exactly
// pageSize lines, OnEnter cannot pop itself (PushMode is mid-flight
// and would unwind incorrectly). WritePaged guarantees we only
// reach OnEnter when len(lines) >= Height, so the first page
// cannot drain the remainder anyway.
func (p *pagerMode) OnEnter(s *Session) error {
	_, err := p.writeNextPage(s)
	return err
}

func (p *pagerMode) OnExit(s *Session) error { return nil }

// Prompt returns the `--More--` line while content remains. Returns
// "" when drained, but in practice Handle pops the mode the moment
// the last page is written — so a "" return here is a defensive
// fallback only.
func (p *pagerMode) Prompt(ctx context.Context, s *Session) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.remaining) == 0 {
		return ""
	}
	return pagerMoreLine
}

// Handle interprets one input line. Empty / whitespace / anything we
// don't recognize advances by one page. `q` (case-insensitive) quits
// immediately, discarding the unwritten remainder. When the page just
// written drains the remainder, the mode pops itself.
func (p *pagerMode) Handle(ctx context.Context, s *Session, line string) error {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "q", "quit":
		return s.PopMode()
	}
	done, err := p.writeNextPage(s)
	if err != nil {
		return err
	}
	if done {
		return s.PopMode()
	}
	return nil
}

// writeNextPage drains up to pageSize lines from remaining and
// writes them with WriteRaw. Returns done=true when the remainder is
// empty after this write. Caller must not hold mu.
func (p *pagerMode) writeNextPage(s *Session) (done bool, err error) {
	p.mu.Lock()
	n := p.pageSize
	if n > len(p.remaining) {
		n = len(p.remaining)
	}
	page := p.remaining[:n]
	p.remaining = p.remaining[n:]
	done = len(p.remaining) == 0
	p.mu.Unlock()
	if len(page) == 0 {
		return done, nil
	}
	if err := s.WriteRaw([]byte(strings.Join(page, ""))); err != nil {
		return done, err
	}
	return done, nil
}

// splitCRLFLines splits body on "\r\n" boundaries while preserving
// the trailing "\r\n" on each returned element. A body that does not
// end in "\r\n" gets its final partial line returned as-is. Bodies
// containing bare "\n" line breaks (uncommon — the codebase
// normalizes to CRLF before reaching the pager) are left intact;
// they'll just appear as one logical line, which the pager's
// height-based chunking handles correctly even if visually cramped.
func splitCRLFLines(body []byte) []string {
	if len(body) == 0 {
		return nil
	}
	s := string(body)
	var out []string
	for {
		i := strings.Index(s, "\r\n")
		if i < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:i+2])
		s = s[i+2:]
		if s == "" {
			return out
		}
	}
}
