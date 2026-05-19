package flow

import "strings"

// Renderer is the engine's only output path. The Runner writes
// prompts and validation-error notices through this interface; the
// engine itself stays unaware of whether the consumer is a
// *telnet.Session, a test buffer, or a future GMCP frame emitter.
//
// Adapter rule: the production adapter that wraps a *telnet.Session
// lives in `internal/mode/flow.go` (O.1) or `cmd/server/`. This
// package MUST NOT import `telnet` — keep the engine transport-
// agnostic so future renderers (web, GMCP) are additive.
type Renderer interface {
	// Write emits a single message. Implementations decide
	// whether to append CRLF, defang cfmt, etc. The Runner emits
	// already-cfmt-formatted strings; the renderer is responsible
	// for any session-specific framing.
	Write(s string) error
}

// BufferRenderer collects writes into an in-memory buffer. Test-only
// — production callers should never use this. Safe for sequential
// access only; mirrors strings.Builder semantics.
type BufferRenderer struct {
	buf strings.Builder
}

func (r *BufferRenderer) Write(s string) error {
	r.buf.WriteString(s)
	return nil
}

// String returns the accumulated output. Lets tests assert against
// the engine's output without re-implementing the renderer.
func (r *BufferRenderer) String() string {
	return r.buf.String()
}

// Reset clears the buffer. Useful between multi-step assertions in
// a single test.
func (r *BufferRenderer) Reset() {
	r.buf.Reset()
}
