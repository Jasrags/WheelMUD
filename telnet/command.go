package telnet

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
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
	// Lag is the global cooldown stamped on Session.nextReady after a
	// successful Run. Zero = no lag (default). Verbs that represent
	// significant in-world actions (combat strikes, zone broadcasts)
	// opt in. Stamped on success only; a failing Run leaves the
	// session unlagged. Phase E #26 / ROADMAP §4.
	Lag time.Duration
	Run func(*Context) error
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
// resolution fails. Verb is lowercased internally; callers may pass any case.
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

// LookupExact resolves verb to a command via alias or exact name only.
// Unlike Lookup, it never falls through to unique-prefix matching —
// callers (notably the help command, which merges its prefix pass with
// the help-topic catalog) sometimes need to distinguish a true exact
// hit from a prefix coincidence so the merged resolution can pick the
// right side. Returns ErrUnknownCommand on miss. Verb is lowercased
// internally; callers may pass any case.
func (r *Registry) LookupExact(verb string) (*Command, error) {
	verb = strings.ToLower(verb)
	if verb == "" {
		return nil, ErrUnknownCommand
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if c, ok := r.lookupExactLocked(verb); ok {
		return c, nil
	}
	return nil, ErrUnknownCommand
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

// maxSegmentsPerLine caps the number of `;`-separated commands a single
// input line may produce. Beyond this, extra segments are dropped after
// a one-line truncation notice; prevents a runaway alias or pasted
// payload from consuming the dispatcher with one keystroke.
const maxSegmentsPerLine = 16

// maxAliasDepth bounds re-splitting after alias expansion. expandAlias
// itself is one-shot per call, so depth only grows when an alias's
// expansion contains `;` AND one of those segments matches another
// alias whose expansion also contains `;`. Three levels is plenty for
// nested macros without unbounded fan-out.
const maxAliasDepth = 3

// Dispatch parses line, resolves the verb, and runs the matching command.
// A line containing top-level `;` (outside quotes) is split into
// segments and each segment is dispatched independently in order.
// Errors from lookup are translated into user-facing messages and are
// not returned. Errors from Command.Run propagate; when chaining, the
// first non-nil Run error is returned but later segments still run.
// ctx is surfaced on the per-dispatch Context so commands can observe
// session cancellation while doing blocking work.
func (r *Registry) Dispatch(ctx context.Context, s *Session, line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	// Stamp activity once per input line, at entry, so `who` idle time
	// reflects the moment the player typed regardless of how long the
	// chain takes to run.
	s.StampInput(time.Now().UTC())

	segments, err := SplitOnSemicolon(line)
	if err != nil {
		if errors.Is(err, ErrUnbalancedQuote) {
			return s.WriteRaw([]byte("Unbalanced quote\r\n"))
		}
		return s.WriteRaw([]byte("Command error\r\n"))
	}
	truncated := false
	if len(segments) > maxSegmentsPerLine {
		segments = segments[:maxSegmentsPerLine]
		truncated = true
	}

	var firstErr error
	for _, seg := range segments {
		if e := r.dispatchOne(ctx, s, seg, 0); e != nil && firstErr == nil {
			firstErr = e
		}
	}
	if truncated {
		if e := s.WriteRaw([]byte("(too many commands; truncated)\r\n")); e != nil && firstErr == nil {
			firstErr = e
		}
	}
	return firstErr
}

// dispatchOne runs a single already-split segment. depth tracks alias
// expansions that themselves contained `;` so the splitter doesn't
// recurse without bound. expandAlias is one-shot per call, so the only
// recursion source is alias-expansion-introduces-`;`.
func (r *Registry) dispatchOne(ctx context.Context, s *Session, line string, depth int) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	// User-level aliases are resolved once per segment. expandAlias
	// itself is closed against alias-of-alias loops; we only re-split
	// when an alias expansion introduces a top-level `;`, and bound
	// that re-split with depth.
	expanded := expandAlias(s.Aliases, line)
	if expanded != line && depth < maxAliasDepth {
		segs, err := SplitOnSemicolon(expanded)
		if err != nil {
			if errors.Is(err, ErrUnbalancedQuote) {
				return s.WriteRaw([]byte("Unbalanced quote\r\n"))
			}
			return s.WriteRaw([]byte("Command error\r\n"))
		}
		if len(segs) > 1 {
			var firstErr error
			for _, seg := range segs {
				if e := r.dispatchOne(ctx, s, seg, depth+1); e != nil && firstErr == nil {
					firstErr = e
				}
			}
			return firstErr
		}
		// Single segment from expansion — fall through with the
		// expanded form so we don't re-expand and risk a loop.
		line = expanded
	} else {
		line = expanded
	}

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
		// Prefer Long when it's set — Help is a one-line summary
		// (often the description, not the syntax) and prefixing it
		// with "Usage: " produces a misleading line. Long is the
		// authoritative usage block for commands that bother to
		// write one. Fall back to Help, then to the verb name so
		// the response is never empty.
		if cmd.Long != "" {
			// Long is authored with bare "\n" newlines; telnet
			// requires CR+LF for proper line breaks. Normalize so
			// authors can write natural multi-line strings without
			// peppering them with \r.
			body := strings.ReplaceAll(cmd.Long, "\r\n", "\n")
			body = strings.ReplaceAll(body, "\n", "\r\n")
			if !strings.HasSuffix(body, "\r\n") {
				body += "\r\n"
			}
			return s.WriteRaw([]byte(body))
		}
		usage := cmd.Help
		if usage == "" {
			usage = cmd.Name
		}
		return s.WriteRaw([]byte("Usage: " + usage + "\r\n"))
	}
	// Phase E #26 / §4: refuse a lagged segment with a copy that
	// shows the remaining time. Per-segment so chained `;` inputs
	// like `look; attack bob` don't refuse the unlagged head.
	if locked, remaining := s.IsLagged(time.Now()); locked {
		secs := int64(remaining.Round(time.Second) / time.Second)
		if secs < 1 {
			secs = 1
		}
		return s.WriteString(fmt.Sprintf("{{You're too busy. (~%ds)}}::yellow\r\n", secs))
	}
	cctx := &Context{
		Ctx:     ctx,
		Session: s,
		Name:    cmd.Name,
		Args:    args,
		Raw:     rest,
	}
	if err := cmd.Run(cctx); err != nil {
		return err
	}
	// Stamp lag on success only — failing commands (bad target,
	// repo error, etc.) do not rate-limit the player.
	if cmd.Lag > 0 {
		s.StampLag(cmd.Lag)
	}
	return nil
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
