package telnet

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// AuthLevel marks the privilege a command requires. Registry.Dispatch
// rejects a command whose Auth exceeds Session.AuthLevel with the same
// "Unknown command" response as a missing verb, so the prompt cannot be
// used to enumerate privileged commands. Sessions start at AuthGuest;
// login mode is what bumps the level.
type AuthLevel uint8

const (
	AuthGuest AuthLevel = iota
	AuthPlayer
	AuthAdmin
)

// Command is a single dispatchable verb.
type Command struct {
	Name    string // canonical, lowercase, unique within a registry
	Aliases []string
	MinArgs int
	Help    string // one-line summary
	Long    string // optional multi-line help body
	Auth    AuthLevel
	Run     func(*Context) error
	// Completer, if non-nil, supplies argument-side tab completion. args
	// is the full argument line as typed (everything after the verb,
	// leading whitespace already stripped). Implementations should match
	// against the trailing partial — see CompletionPartial — and return
	// candidates whose Text is the full replacement token. A nil return
	// (or no Completer) bells.
	Completer func(s *Session, args string) []Candidate
}

// Context is passed to Command.Run. It is created fresh per dispatch.
type Context struct {
	Ctx     context.Context // canceled when the session ends
	Session *Session
	Name    string   // canonical name of the command that matched
	Args    []string // tokenized arguments
	Raw     string   // input after the verb, with leading/trailing whitespace trimmed
}

// Registry holds the commands available to a session.
type Registry struct {
	mu      sync.RWMutex
	sorted  []*Command          // sorted by Name; supports binary-search prefix scans
	aliases map[string]*Command // alias -> command (explicit only)
}

var (
	ErrUnknownCommand   = errors.New("telnet: unknown command")
	ErrAmbiguousPrefix  = errors.New("telnet: ambiguous command prefix")
	ErrDuplicateCommand = errors.New("telnet: duplicate command name or alias")
	ErrInvalidCommand   = errors.New("telnet: invalid command definition")
)

func NewRegistry() *Registry {
	return &Registry{
		aliases: make(map[string]*Command),
	}
}

// Register adds a command. Names and aliases must be unique across the
// registry and must be lowercase, non-empty, and contain no whitespace.
func (r *Registry) Register(cmds ...*Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range cmds {
		if err := r.registerLocked(c); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) registerLocked(c *Command) error {
	if c == nil || c.Run == nil {
		return fmt.Errorf("%w: nil command or Run", ErrInvalidCommand)
	}
	if !validVerb(c.Name) {
		return fmt.Errorf("%w: bad name %q", ErrInvalidCommand, c.Name)
	}
	for _, a := range c.Aliases {
		if !validVerb(a) {
			return fmt.Errorf("%w: bad alias %q", ErrInvalidCommand, a)
		}
	}
	if _, exists := r.lookupExactLocked(c.Name); exists {
		return fmt.Errorf("%w: %q", ErrDuplicateCommand, c.Name)
	}
	for _, a := range c.Aliases {
		if _, exists := r.lookupExactLocked(a); exists {
			return fmt.Errorf("%w: alias %q", ErrDuplicateCommand, a)
		}
	}

	idx := sort.Search(len(r.sorted), func(i int) bool { return r.sorted[i].Name >= c.Name })
	r.sorted = append(r.sorted, nil)
	copy(r.sorted[idx+1:], r.sorted[idx:])
	r.sorted[idx] = c

	for _, a := range c.Aliases {
		r.aliases[a] = c
	}
	return nil
}

func (r *Registry) lookupExactLocked(verb string) (*Command, bool) {
	if c, ok := r.aliases[verb]; ok {
		return c, true
	}
	idx := sort.Search(len(r.sorted), func(i int) bool { return r.sorted[i].Name >= verb })
	if idx < len(r.sorted) && r.sorted[idx].Name == verb {
		return r.sorted[idx], true
	}
	return nil, false
}

// Lookup resolves verb to a command. Resolution order: alias, exact name,
// unique prefix. Returns ErrUnknownCommand or ErrAmbiguousPrefix when
// resolution fails.
func (r *Registry) Lookup(verb string) (*Command, error) {
	verb = strings.ToLower(verb)
	if verb == "" {
		return nil, ErrUnknownCommand
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.lookupExactLocked(verb); ok {
		return c, nil
	}
	matches := r.prefixLocked(verb)
	switch len(matches) {
	case 0:
		return nil, ErrUnknownCommand
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrAmbiguousPrefix, joinNames(matches))
	}
}

// Prefix returns every command whose name starts with p, sorted by name.
// Aliases are not included. Useful for help listings and future autocomplete.
func (r *Registry) Prefix(p string) []*Command {
	p = strings.ToLower(p)
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.prefixLocked(p)
}

func (r *Registry) prefixLocked(p string) []*Command {
	if p == "" {
		out := make([]*Command, len(r.sorted))
		copy(out, r.sorted)
		return out
	}
	start := sort.Search(len(r.sorted), func(i int) bool { return r.sorted[i].Name >= p })
	var out []*Command
	for i := start; i < len(r.sorted); i++ {
		if !strings.HasPrefix(r.sorted[i].Name, p) {
			break
		}
		out = append(out, r.sorted[i])
	}
	return out
}

// All returns every registered command, sorted by name. Aliases excluded.
func (r *Registry) All() []*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Command, len(r.sorted))
	copy(out, r.sorted)
	return out
}

// Dispatch parses line, resolves the verb, and runs the matching command.
// Errors from lookup are translated into user-facing messages and are not
// returned. Errors from Command.Run are returned to the caller. ctx is
// surfaced on the per-dispatch Context so commands can observe session
// cancellation while doing blocking work.
func (r *Registry) Dispatch(ctx context.Context, s *Session, line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	// User-level aliases are resolved once before lookup so a chained
	// alias-of-alias can't recurse. expandAlias is a no-op when no
	// matching alias exists or when the session has no table.
	line = expandAlias(s.Aliases, line)
	verb, rest := splitVerb(line)
	cmd, err := r.Lookup(verb)
	if err != nil {
		return writeLookupError(s, err)
	}
	if s.AuthLevel < cmd.Auth {
		// Don't disclose that the verb exists — render the same response
		// as ErrUnknownCommand so the prompt can't be used to enumerate
		// privileged commands.
		return s.WriteRaw([]byte("Unknown command\r\n"))
	}
	args, err := Tokenize(rest)
	if err != nil {
		if errors.Is(err, ErrUnbalancedQuote) {
			return s.WriteRaw([]byte("Unbalanced quote\r\n"))
		}
		return s.WriteRaw([]byte("Command error\r\n"))
	}
	if len(args) < cmd.MinArgs {
		usage := cmd.Help
		if usage == "" {
			usage = cmd.Name
		}
		return s.WriteRaw([]byte("Usage: " + usage + "\r\n"))
	}
	cctx := &Context{
		Ctx:     ctx,
		Session: s,
		Name:    cmd.Name,
		Args:    args,
		Raw:     rest,
	}
	return cmd.Run(cctx)
}

func writeLookupError(s *Session, err error) error {
	switch {
	case errors.Is(err, ErrUnknownCommand):
		return s.WriteRaw([]byte("Unknown command\r\n"))
	case errors.Is(err, ErrAmbiguousPrefix):
		return s.WriteRaw([]byte(err.Error() + "\r\n"))
	default:
		return s.WriteRaw([]byte("Command error\r\n"))
	}
}

// SplitVerb returns the leading word of line and the remainder with any
// leading/trailing whitespace trimmed. Exposed so completion plumbing
// elsewhere can split a buffer the same way Dispatch does.
func SplitVerb(line string) (verb, rest string) { return splitVerb(line) }

func splitVerb(line string) (verb, rest string) {
	idx := strings.IndexAny(line, " \t")
	if idx < 0 {
		return line, ""
	}
	return line[:idx], strings.TrimSpace(line[idx+1:])
}

// validVerb enforces ASCII-only, lowercase, no-whitespace command names and
// aliases. The Tab handler and column layout currently assume one display
// cell per byte; loosening this requires changing extendBuffer and
// writeColumns to do width-aware accounting (they already use rune counts,
// but combining marks / wide CJK would still misalign).
func validVerb(v string) bool {
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c >= 0x80 {
			return false // non-ASCII
		}
		if c >= 'A' && c <= 'Z' {
			return false // uppercase
		}
		switch c {
		case ' ', '\t', '\r', '\n', 0:
			return false
		}
		// Reject other control bytes; printable ASCII only.
		if c < 0x20 || c == 0x7F {
			return false
		}
	}
	return true
}

func joinNames(cmds []*Command) string {
	names := make([]string, len(cmds))
	for i, c := range cmds {
		names[i] = c.Name
	}
	return strings.Join(names, ", ")
}
