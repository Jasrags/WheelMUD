package telnet

// historyCap bounds the per-session history ring. Beyond this lines roll
// off the oldest end. Sized for human typing patterns; not a tunable.
const historyCap = 100

// History is a fixed-cap ring of previously-entered lines plus an index
// cursor used by ↑/↓ navigation. The zero value is ready to use.
//
// Convention:
//   - idx == len(entries) means "off the bottom" — i.e., editing the
//     live line that hasn't been entered yet. draft holds whatever the
//     user had typed before they started walking back.
//   - idx in [0, len(entries)) points at a stored line.
//
// History is owned by the read goroutine inside RunSession and is not
// safe for concurrent use, matching Session.InputBuffer's rules.
type History struct {
	entries []string
	idx     int
	draft   string
}

// Add records line as the newest entry, drops the oldest when full, and
// resets navigation so the next ↑ starts from the bottom. Empty lines
// and exact duplicates of the last entry are skipped — the standard
// shell behavior.
func (h *History) Add(line string) {
	if line == "" {
		h.Reset()
		return
	}
	if n := len(h.entries); n > 0 && h.entries[n-1] == line {
		h.Reset()
		return
	}
	if len(h.entries) == historyCap {
		copy(h.entries, h.entries[1:])
		h.entries = h.entries[:len(h.entries)-1]
	}
	h.entries = append(h.entries, line)
	h.Reset()
}

// Reset clears navigation state. Called automatically by Add and after
// a line is dispatched.
func (h *History) Reset() {
	h.idx = len(h.entries)
	h.draft = ""
}

// Prev moves one step toward the oldest entry. live is the buffer the
// user has currently composed; on the first Prev call it's snapshotted
// into draft so a subsequent Next at the bottom restores it. Returns
// the entry to display and ok=false when already at the oldest.
func (h *History) Prev(live string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.idx == len(h.entries) {
		h.draft = live
	}
	if h.idx == 0 {
		return h.entries[0], true
	}
	h.idx--
	return h.entries[h.idx], true
}

// Next moves one step toward the live line. At the bottom it returns
// the snapshotted draft and resets navigation. ok=false means there's
// nothing newer to show — the caller should bell.
func (h *History) Next() (string, bool) {
	if h.idx >= len(h.entries) {
		return "", false
	}
	h.idx++
	if h.idx == len(h.entries) {
		d := h.draft
		h.draft = ""
		return d, true
	}
	return h.entries[h.idx], true
}

// Len returns the number of stored entries. Test helper.
func (h *History) Len() int { return len(h.entries) }
