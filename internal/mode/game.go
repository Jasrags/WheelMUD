package mode

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/i582/cfmt/cmd/cfmt"

	"github.com/Jasrags/WheelMUD/internal/prompt"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// promptFallback is the prompt used when rendering can't run (no
// session character, repo error, empty template). Matches the legacy
// hardcoded prompt so a render failure stays invisible to the player.
const promptFallback = "> "

// promptLookupTimeout caps the per-prompt repo read so a stalled DB
// can't wedge the dispatcher. Prompts run once per command and the
// reads are single-row primary-key lookups; a sub-second cap is
// generous.
const promptLookupTimeout = 250 * time.Millisecond

// Game is the in-world command-dispatch mode. It wraps a Registry and
// delegates Handle to Registry.Dispatch.
//
// Characters / Rooms back prompt rendering: every dispatch ends in
// Prompt(s), which fetches live HP / room name / coin off the repos
// to interpolate into Template. Both repos may be nil in tests that
// only exercise dispatch + tab completion.
type Game struct {
	Registry   *telnet.Registry
	Characters repo.CharacterRepo
	Rooms      repo.RoomRepo
	Template   string
}

// NewGame builds a Game with the given registry and prompt deps. An
// empty template falls back to the legacy "> " literal so callers
// that don't care about prompt rendering (most tests) can pass "".
func NewGame(r *telnet.Registry, chars repo.CharacterRepo, rooms repo.RoomRepo, template string) *Game {
	return &Game{
		Registry:   r,
		Characters: chars,
		Rooms:      rooms,
		Template:   template,
	}
}

func (g *Game) Handle(ctx context.Context, s *telnet.Session, line string) error {
	return g.Registry.Dispatch(ctx, s, line)
}

// Prompt renders Template against the session's live character /
// room state. Falls back to "> " (logged at warn) if anything goes
// wrong — the player should always get a usable prompt. parent is
// the dispatcher's per-session ctx; it is canceled when the read
// loop exits, so a stalled lookup against a torn-down session
// returns immediately instead of blocking on the timeout.
func (g *Game) Prompt(parent context.Context, s *telnet.Session) string {
	// Guard order matters: an empty template or missing CharacterRepo
	// short-circuits to the legacy "> " literal before we attempt any
	// repo work. A nil Characters therefore renders no placeholders at
	// all (including %g) — by design, since we have nowhere to read
	// HP / coin from. Tests that don't care about prompt rendering
	// pass nil here.
	if g.Template == "" || g.Characters == nil || s.CharacterName == "" {
		return promptFallback
	}
	ctx, cancel := context.WithTimeout(parent, promptLookupTimeout)
	defer cancel()

	c, err := g.Characters.FindByName(ctx, s.CharacterName)
	if err != nil {
		// A canceled / timed-out parent ctx is the expected signal on
		// session teardown — fall back silently rather than warn-spam
		// once per prompt cycle while the dispatcher drains.
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			slog.Warn("prompt: character lookup failed",
				"character", s.CharacterName, "error", err)
		}
		return promptFallback
	}

	tmpl := g.Template
	if c.PromptTemplate != "" {
		tmpl = c.PromptTemplate
	}

	v := prompt.Vars{
		HPCur: c.Core.HPCurrent,
		HPMax: c.Core.HPMax,
	}
	if prompt.NeedsGold(tmpl) {
		v.Gold = defangCfmt(c.Coin.Short())
	}
	if prompt.NeedsRoom(tmpl) && g.Rooms != nil && s.CurrentRoomID != 0 {
		r, err := g.Rooms.FindByID(ctx, s.CurrentRoomID)
		switch {
		case err == nil:
			v.RoomName = defangCfmt(r.Name)
		case errors.Is(err, repo.ErrRoomNotFound):
			// silent — empty %r is benign
		default:
			slog.Warn("prompt: room lookup failed",
				"room", s.CurrentRoomID, "error", err)
		}
	}
	return cfmt.Sprint(prompt.Render(tmpl, v))
}

// cfmtDefanger neutralizes cfmt's injection vectors inside interpolated
// values. We narrow on the actual unsafe sequences:
//
//   - `{{`     — opens a styled span; break it so a room name can't
//                start one that the user's template never closes.
//   - `}}::`   — closes a styled span and consumes a style selector;
//                this is the only sequence that can recolor the rest
//                of the user's prompt. Standalone `}}` or `::` are
//                harmless on their own and stay verbatim so legitimate
//                text like "Lv:: 5" or "}}note" survives.
//
// Player-authored templates (`prompt set`) are rendered verbatim —
// color is the feature.
var cfmtDefanger = strings.NewReplacer("{{", "{ {", "}}::", "} }::")

func defangCfmt(s string) string { return cfmtDefanger.Replace(s) }

func (g *Game) OnEnter(_ *telnet.Session) error { return nil }
func (g *Game) OnExit(_ *telnet.Session) error  { return nil }

// Complete satisfies telnet.Completer. With no whitespace in the buffer
// it does verb completion via Registry.Prefix; once whitespace appears
// it splits off the verb, looks up the command, and delegates to the
// command's Completer (if any). Privilege-gated commands fall through
// to nil so a tab can't be used to enumerate them — the same anti-
// enumeration policy Registry.Dispatch enforces.
func (g *Game) Complete(s *telnet.Session, buffer string) []telnet.Candidate {
	if !strings.ContainsAny(buffer, " \t") {
		cmds := g.Registry.Prefix(buffer)
		if len(cmds) == 0 {
			return nil
		}
		out := make([]telnet.Candidate, 0, len(cmds))
		for _, c := range cmds {
			if s.AuthLevel < c.Auth {
				continue
			}
			out = append(out, telnet.Candidate{Text: c.Name, Help: c.Help})
		}
		return out
	}
	verb, rest := telnet.SplitVerb(buffer)
	cmd, err := g.Registry.Lookup(verb)
	if err != nil || cmd.Completer == nil || s.AuthLevel < cmd.Auth {
		return nil
	}
	return cmd.Completer(s, rest)
}
