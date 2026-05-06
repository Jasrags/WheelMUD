package mode

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// AccountMenu is the post-authentication hub. Login / Create push it
// after firing the once-per-login MOTD/news block, so character
// management lives behind a stable set of verbs rather than the legacy
// auto-skip-to-select shortcut. Slice 1 verbs:
//
//	list           — re-display the character roster
//	play [name]    — promote into game; no-arg auto-picks when there's
//	                 exactly one character (preserves the "one keystroke
//	                 into the world" feel for single-character accounts)
//	new            — ReplaceMode into CharacterCreate
//	news           — replay the unseen-news block on demand without
//	                 advancing last_news_seen
//	help           — list verbs
//	quit           — close the connection
//
// Subsequent slices add password change (1b), account settings,
// security, email/recovery, and character delete (which needs cascade
// plumbing not in this slice).
type AccountMenu struct {
	chars   []repo.Character
	repo    repo.CharacterRepo
	game    telnet.Mode
	motd    MOTDFunc
	catalog *chargen.Catalog
	// lastSeen feeds the `news` replay verb so re-rendering shows the
	// same unseen entries the post-login block did. Sourced from the
	// most-recently-played character (chars[0] under ListByAccount's
	// ordering), or the zero value when the account has no characters.
	lastSeen  time.Time
	listShown bool
}

// NewAccountMenu returns an AccountMenu over the given character list.
// chars must be the account's roster as returned by
// CharacterRepo.ListByAccount (ordered by last_played_at desc); chars[0]
// is the auto-pick target and the news-replay watermark source.
func NewAccountMenu(chars []repo.Character, characters repo.CharacterRepo, game telnet.Mode) *AccountMenu {
	m := &AccountMenu{chars: chars, repo: characters, game: game}
	if len(chars) > 0 {
		m.lastSeen = chars[0].LastNewsSeen
	}
	return m
}

// SetMOTD wires the MOTD/news hook used by the `news` replay verb.
func (m *AccountMenu) SetMOTD(f MOTDFunc) { m.motd = f }

// SetCatalog forwards the chargen content catalog to a CharacterCreate
// spawned by the `new` verb.
func (m *AccountMenu) SetCatalog(c *chargen.Catalog) { m.catalog = c }

func (m *AccountMenu) Prompt(_ context.Context, _ *telnet.Session) string {
	return "[account] "
}

func (m *AccountMenu) OnEnter(s *telnet.Session) error {
	if m.listShown {
		return nil
	}
	m.listShown = true
	return m.writeList(s)
}

func (m *AccountMenu) OnExit(_ *telnet.Session) error { return nil }

func (m *AccountMenu) Handle(ctx context.Context, s *telnet.Session, line string) error {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return nil
	}
	verb := strings.ToLower(fields[0])
	args := fields[1:]
	switch verb {
	case "list", "ls", "characters":
		return m.writeList(s)
	case "play", "select":
		return m.handlePlay(ctx, s, args)
	case "new", "create":
		return m.handleNew(s)
	case "news", "motd":
		return m.handleNews(s)
	case "help", "?":
		return m.writeHelp(s)
	case "quit", "exit":
		_ = s.WriteRaw([]byte("Goodbye.\r\n"))
		_ = s.Conn.Close()
		return telnet.ErrSessionEnded
	}
	return writeError(s, "Unknown command. Type 'help' for the menu.")
}

func (m *AccountMenu) writeList(s *telnet.Session) error {
	var b strings.Builder
	b.WriteString("\r\nYour characters:\r\n")
	if len(m.chars) == 0 {
		b.WriteString("  (none — type 'new' to create one)\r\n")
		return s.WriteRaw([]byte(b.String()))
	}
	for i, c := range m.chars {
		last := "never"
		if c.LastPlayedAt != nil && !c.LastPlayedAt.IsZero() {
			last = c.LastPlayedAt.Format("2006-01-02")
		}
		fmt.Fprintf(&b, "  %d) %-20s  lvl %-2d  last %s\r\n",
			i+1, c.Name, totalClassLevels(c.ClassLevels), last)
	}
	return s.WriteRaw([]byte(b.String()))
}

// handlePlay resolves the target character and promotes the session
// into game mode. With no arg, single-character accounts auto-pick;
// multi-character accounts are required to name the character. Lookup
// is constrained to the menu's pre-loaded roster so a player can't
// promote into a character they don't own — the same ownership gate
// CharacterSelect uses.
func (m *AccountMenu) handlePlay(ctx context.Context, s *telnet.Session, args []string) error {
	if len(m.chars) == 0 {
		return writeError(s, "No characters on this account. Type 'new' to create one.")
	}
	var target *repo.Character
	switch len(args) {
	case 0:
		if len(m.chars) > 1 {
			return writeError(s, "Specify a character: play <name>.")
		}
		target = &m.chars[0]
	default:
		name := args[0]
		// Numeric pick (matches the list ordering shown by `list`).
		if i, err := parsePositiveIndex(name, len(m.chars)); err == nil {
			target = &m.chars[i]
			break
		}
		for i := range m.chars {
			if strings.EqualFold(m.chars[i].Name, name) {
				target = &m.chars[i]
				break
			}
		}
		if target == nil {
			// Re-resolve via repo to distinguish "no such character
			// anywhere" from "not yours" without leaking either bit.
			if _, err := m.repo.FindByName(ctx, name); err != nil && !errors.Is(err, repo.ErrCharacterNotFound) {
				return writeError(s, "Could not load that character. Try again.")
			}
			return writeError(s, "No such character on this account.")
		}
	}
	return promoteToGame(ctx, s, *target, m.repo, m.game)
}

func (m *AccountMenu) handleNew(s *telnet.Session) error {
	create := NewCharacterCreate(m.repo, m.game)
	create.SetCatalog(m.catalog)
	// MOTD intentionally not threaded into CharacterCreate: the news
	// block fires once per login in postAuth, before the menu lands.
	// promoteToGame downstream is now MOTD-free for the same reason.
	return s.ReplaceMode(create)
}

func (m *AccountMenu) handleNews(s *telnet.Session) error {
	if m.motd == nil {
		return writeError(s, "News is not configured on this server.")
	}
	return m.motd(s, m.lastSeen)
}

func (m *AccountMenu) writeHelp(s *telnet.Session) error {
	help := "" +
		"\r\nAccount menu:\r\n" +
		"  list              show your characters\r\n" +
		"  play [name|#]     enter the world (no-arg picks the only character)\r\n" +
		"  new               create a new character\r\n" +
		"  news              re-display the unread-news block\r\n" +
		"  help              this list\r\n" +
		"  quit              disconnect\r\n"
	return s.WriteRaw([]byte(help))
}

// parsePositiveIndex interprets `s` as a 1-based list index in
// [1, n] and returns the 0-based slot. Anything non-numeric or out of
// range returns an error so the caller can fall back to a name lookup.
func parsePositiveIndex(s string, n int) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 || v > n {
		return 0, errors.New("out of range")
	}
	return v - 1, nil
}

// totalClassLevels sums the multi-class level map. Returns 1 when the
// map is empty so a freshly-created character without populated
// ClassLevels still shows lvl 1 in the listing.
func totalClassLevels(cl map[creature.Class]int8) int {
	if len(cl) == 0 {
		return 1
	}
	total := 0
	for _, lv := range cl {
		total += int(lv)
	}
	if total < 1 {
		return 1
	}
	return total
}
