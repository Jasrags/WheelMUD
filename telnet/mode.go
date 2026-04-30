package telnet

import (
	"context"
	"errors"
)

// Mode is a per-session input handler. Sessions hold a stack of modes;
// dispatch and prompting always use the top mode. Login screens, character
// creation, in-game play, and editor sub-modes each implement Mode.
type Mode interface {
	// Handle processes a fully-buffered line of input. ctx is canceled when
	// the session ends, so handlers doing blocking I/O (DB lookup, network)
	// must respect it.
	Handle(ctx context.Context, s *Session, line string) error
	// Prompt returns the bytes to write after a line is handled. Empty
	// string means "no prompt this time" — useful for modes that print
	// their own prompt inside Handle.
	Prompt(s *Session) string
	// OnEnter runs when this mode is pushed onto the stack.
	OnEnter(s *Session) error
	// OnExit runs when this mode is popped off the stack.
	OnExit(s *Session) error
}

var ErrNoMode = errors.New("telnet: session has no mode")
