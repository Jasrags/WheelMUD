package cmd

// feat is the spend verb for pending_feats (Phase E #25). Players type
// `feat` to see the menu of feats they can choose from, and
// `feat <id>` to lock one in. The pool was deposited by the
// level-up commit (Phase E #23 slice 4) at every level divisible by 3.
// The verb is named `feat` (not `pick feat`) to avoid colliding with
// the existing lockpicking verb in door.go.
//
// V1 scope:
//   - Allowed list = catalog feats that are not background-restricted,
//     plus background feats whose Backgrounds list includes the
//     character's background. Mirrors the chargen #15 menu logic.
//   - 1 pending feat per pick.
//   - No prereq enforcement (BAB / ability mins / prior feats).
//   - Refuses on duplicate (already in Character.Feats), unknown id,
//     and empty pool.
//
// Refusals do NOT mutate or audit. Successful spends write one
// admin_audit row.

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewFeat builds the `feat` verb.
func NewFeat(characters repo.CharacterRepo, cat *chargen.Catalog,
	audits repo.AdminAuditRepo,
) *telnet.Command {
	return &telnet.Command{
		Name: "feat",
		Help: "Feat — spend pending feat slots on a feat",
		Long: "Usage: feat                show menu\n" +
			"       feat <id>           spend a feat slot on <id>\n" +
			"       feat info <id>      show description for <id>",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			if cat == nil {
				return s.WriteString("{{Feat catalog unavailable.}}::red\r\n")
			}
			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("feat: char lookup", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't find your records.}}::red\r\n")
			}
			allowed := allowedFeatsFor(char, cat)
			args := c.Args
			if len(args) == 0 {
				return writeFeatMenu(s, char, allowed)
			}
			if strings.EqualFold(args[0], "info") || strings.EqualFold(args[0], "i") {
				if len(args) < 2 {
					return s.WriteString("{{Type 'feat info <id>' for a feat description.}}::yellow\r\n")
				}
				return writeFeatInfo(s, args[1], allowed, cat)
			}
			id, ok := matchFeatToken(args[0], allowed, cat)
			if !ok {
				return s.WriteString("{{No such feat.}}::yellow\r\n")
			}
			return commitFeat(c, characters, audits, char, cat, id)
		},
	}
}

// writeFeatMenu renders the pickable-feat list. Mirrors the `learn`
// menu look (SectionHeader / FieldRow / numbered rows).
func writeFeatMenu(s *telnet.Session, char repo.Character, allowed []*chargen.Feat) error {
	if err := display.SectionHeader(s, "Feat Selection"); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Feat slots",
		strconv.Itoa(int(char.PendingFeats)), 14); err != nil {
		return err
	}
	if len(allowed) == 0 {
		return s.WriteString("\r\n  {{(no feats available)}}::gray\r\n")
	}
	if err := display.Subsection(s, "Available feats"); err != nil {
		return err
	}
	known := featKnownSet(char)
	for i, f := range allowed {
		tag := ""
		if f.Background {
			tag = "[bg]"
		}
		marker := "  "
		if _, ok := known[chargen.HashID(f.ID)]; ok {
			marker = "✓ "
		}
		if err := s.WriteString(fmt.Sprintf(
			"  {{%2d)}}::gray %s{{%-26s}}::yellow|bold %s\r\n",
			i+1, marker, display.Defang(f.Name, ""), tag,
		)); err != nil {
			return err
		}
	}
	return s.WriteString(
		"\r\n  Usage: {{feat <id>}}::yellow|bold  ·  " +
			"{{feat info <id>}}::yellow\r\n",
	)
}

// writeFeatInfo prints the descriptor for one feat. Read-only.
func writeFeatInfo(s *telnet.Session, token string, allowed []*chargen.Feat,
	cat *chargen.Catalog,
) error {
	id, ok := matchFeatToken(token, allowed, cat)
	if !ok {
		return s.WriteString("{{No such feat.}}::yellow\r\n")
	}
	f, _ := cat.Feat(id)
	if f == nil {
		return s.WriteString("{{No description on file.}}::yellow\r\n")
	}
	if err := s.WriteString(fmt.Sprintf(
		"{{%s (%s)}}::cyan|bold\r\n",
		display.Defang(f.Name, ""), display.Defang(f.ID, ""),
	)); err != nil {
		return err
	}
	if f.Description != "" {
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
		if err := s.WriteWrapped(strings.TrimRight(f.Description, "\n")); err != nil {
			return err
		}
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return display.Rule(s)
}

// commitFeat enforces budget + duplicate then calls RecordFeatPick.
// On success it writes a confirmation line and audits.
func commitFeat(c *telnet.Context, characters repo.CharacterRepo,
	audits repo.AdminAuditRepo, char repo.Character, cat *chargen.Catalog,
	featID string,
) error {
	s := c.Session
	// Duplicate check runs before the pool check so a zero-pool
	// player who tries to pick a feat they already have gets the
	// informative "already know" message instead of a misleading
	// "no slots available".
	key := chargen.HashID(featID)
	for _, existing := range char.Feats {
		if existing == key {
			f, _ := cat.Feat(featID)
			name := featID
			if f != nil {
				name = f.Name
			}
			return s.WriteString(fmt.Sprintf(
				"{{You already know %s.}}::yellow\r\n",
				display.Defang(name, ""),
			))
		}
	}
	if char.PendingFeats <= 0 {
		return s.WriteString("{{No feat picks available.}}::yellow\r\n")
	}
	newPending := char.PendingFeats - 1
	if err := characters.RecordFeatPick(c.Ctx, char.ID, key, newPending); err != nil {
		slog.Error("feat: record feat pick",
			"char", char.ID, "feat", featID, "error", err)
		return s.WriteString("{{The lesson slips away as you reach for it.}}::red\r\n")
	}
	audit.Record(c.Ctx, audits, s, "feat", featID, "")

	f, _ := cat.Feat(featID)
	name := featID
	if f != nil {
		name = f.Name
	}
	slots := "feat slots remain"
	if newPending == 1 {
		slots = "feat slot remains"
	}
	return s.WriteString(fmt.Sprintf(
		"{{You learn %s.}}::green|bold  %d %s.\r\n",
		display.Defang(name, ""), newPending, slots,
	))
}

// allowedFeatsFor returns catalog feats the character can pick:
// general (non-background) feats + background feats matching the
// character's background. Sorted by Name for stable menu order.
func allowedFeatsFor(ch repo.Character, cat *chargen.Catalog) []*chargen.Feat {
	if cat == nil {
		return nil
	}
	bgID := backgroundIDFor(ch, cat)
	out := make([]*chargen.Feat, 0, 16)
	for _, f := range cat.Feats() {
		if !f.Background {
			out = append(out, f)
			continue
		}
		if bgID == "" {
			continue
		}
		for _, b := range f.Backgrounds {
			if b == bgID {
				out = append(out, f)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// backgroundIDFor maps the character's Background enum back to the
// catalog string id. Returns "" when the background isn't found.
func backgroundIDFor(ch repo.Character, cat *chargen.Catalog) string {
	for _, bg := range cat.Backgrounds() {
		if bg.Enum == ch.Background {
			return bg.ID
		}
	}
	return ""
}

// featKnownSet returns the character's already-picked feat hashes as
// a set for the menu's ✓ marker.
func featKnownSet(ch repo.Character) map[int32]struct{} {
	out := make(map[int32]struct{}, len(ch.Feats))
	for _, id := range ch.Feats {
		out[id] = struct{}{}
	}
	return out
}

// matchFeatToken resolves a player-typed token (numeric menu index or
// string id, case-insensitive) against the allowed list.
func matchFeatToken(token string, allowed []*chargen.Feat, cat *chargen.Catalog) (string, bool) {
	if token == "" {
		return "", false
	}
	if n, err := strconv.Atoi(token); err == nil {
		if n >= 1 && n <= len(allowed) {
			return allowed[n-1].ID, true
		}
		return "", false
	}
	t := strings.ToLower(strings.TrimSpace(token))
	for _, f := range allowed {
		if strings.EqualFold(f.ID, t) || strings.EqualFold(f.Name, t) {
			return f.ID, true
		}
	}
	return "", false
}
