package cmd

// learn_weave is the spend verb for pending_weaves (Phase E #25). The
// `learn weave` subcommand of the `learn` verb drains the channeler's
// per-level weave pool against the chargen weave catalog.
//
// V1 scope:
//   - Channeler-only: refused unless Character.Channeling is non-nil.
//   - Affinity-gated: weave's Power must be in Channeling.Affinities.
//     Mirrors the chargen #15 channeler-branch starting-weaves filter.
//   - 1 pending weave per pick. Anywhere, anytime.
//   - Chargen catalog is currently level-0-only; a future #12 weave
//     table will widen the menu to higher levels.
//
// Refusals (non-channeler, miss-affinity, duplicate, unknown, empty
// pool) do NOT mutate or audit. Successful spends write one
// admin_audit row.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// learnWeavePowerNames mirrors the mode-package private table — the
// same canonical capitalization used in chargen channeler menus.
var learnWeavePowerNames = [...]string{"Air", "Earth", "Fire", "Water", "Spirit"}

// resolvedWeaveTeacher carries the teacher-NPC + its repo row through
// the verb's branches. nil means no teacher in the room (chargen-pool
// path).
type resolvedWeaveTeacher struct {
	teacher   repo.WeaveTeacher
	keeper    creature.MobInstance
	keeperTpl creature.MobTemplate
}

func (r *resolvedWeaveTeacher) keeperName() string {
	if r.keeperTpl.Core.Name != "" {
		return r.keeperTpl.Core.Name
	}
	return "a weave teacher"
}

var errNoWeaveTeacherHere = errors.New("no weave teacher in this room")

// findWeaveTeacher walks the mobs in roomID and returns the first one
// whose template has a matching weave_teachers row. Mirrors
// findTrainer in train.go.
func findWeaveTeacher(ctx context.Context, roomID int64,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	teachers repo.WeaveTeacherRepo,
) (resolvedWeaveTeacher, error) {
	if roomID == 0 || teachers == nil {
		return resolvedWeaveTeacher{}, errNoWeaveTeacherHere
	}
	occupants, err := mobs.ListInRoom(ctx, roomID)
	if err != nil {
		return resolvedWeaveTeacher{}, fmt.Errorf("list room mobs: %w", err)
	}
	for _, m := range occupants {
		t, err := teachers.GetByMobTemplateID(ctx, m.TemplateID)
		if errors.Is(err, repo.ErrWeaveTeacherNotFound) {
			continue
		}
		if err != nil {
			return resolvedWeaveTeacher{}, fmt.Errorf("get weave teacher: %w", err)
		}
		tpl, err := templates.GetByID(ctx, m.TemplateID)
		if err != nil {
			return resolvedWeaveTeacher{}, fmt.Errorf("get template: %w", err)
		}
		return resolvedWeaveTeacher{teacher: t, keeper: m, keeperTpl: tpl}, nil
	}
	return resolvedWeaveTeacher{}, errNoWeaveTeacherHere
}

// runLearnWeave is the dispatch point invoked by NewLearn when the
// player typed `learn weave …`. With a weave-teacher in the room it
// runs the Phase E #28 mid-game path (drains practice_points,
// applies the teacher's level/power filter); otherwise it runs the
// Phase E #25 chargen-pool path (drains pending_weaves).
func runLearnWeave(c *telnet.Context, characters repo.CharacterRepo,
	cat *chargen.Catalog, audits repo.AdminAuditRepo,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	weaveTeachers repo.WeaveTeacherRepo,
) error {
	s := c.Session
	if cat == nil {
		return s.WriteString("{{Weave catalog unavailable.}}::red\r\n")
	}
	char, err := characters.FindByName(c.Ctx, s.CharacterName)
	if err != nil {
		slog.Error("learn weave: char lookup", "char", s.CharacterID, "error", err)
		return s.WriteString("{{You can't find your records.}}::red\r\n")
	}
	if char.Channeling == nil {
		return s.WriteString("{{You cannot weave the One Power.}}::yellow\r\n")
	}

	// Teacher detection — nil result means "no teacher in this room",
	// which falls through to the chargen-pool path verbatim.
	var teacher *resolvedWeaveTeacher
	if mobs != nil && templates != nil && weaveTeachers != nil {
		res, err := findWeaveTeacher(c.Ctx, s.CurrentRoomID, mobs, templates, weaveTeachers)
		if err != nil && !errors.Is(err, errNoWeaveTeacherHere) {
			slog.Error("learn weave: teacher lookup", "error", err)
			return s.WriteString("{{The teacher is busy with something else.}}::red\r\n")
		}
		if err == nil {
			teacher = &res
		}
	}

	allowed := allowedWeavesFor(char, cat)
	if teacher != nil {
		allowed = filterByTeacher(allowed, teacher)
	}

	rest := c.Args[1:] // strip the leading "weave"
	if len(rest) == 0 {
		return writeLearnWeaveMenu(s, char, allowed, teacher)
	}
	if strings.EqualFold(rest[0], "info") || strings.EqualFold(rest[0], "i") {
		if len(rest) < 2 {
			return s.WriteString("{{Type 'learn weave info <id>' for a weave description.}}::yellow\r\n")
		}
		return writeLearnWeaveInfo(s, rest[1], allowed, cat)
	}
	id, ok := matchWeaveToken(rest[0], allowed, cat)
	if !ok {
		return s.WriteString("{{No such weave.}}::yellow\r\n")
	}
	if teacher != nil {
		return commitWeaveStudy(c, characters, audits, char, cat, id, teacher, allowed)
	}
	return commitLearnWeave(c, characters, audits, char, cat, id)
}

// weaveInAllowed reports whether weaveID is present in the allowed
// slice. Defense-in-depth used by commitWeaveStudy so a caller that
// bypasses the menu still hits the teacher-filter gate.
func weaveInAllowed(weaveID string, allowed []*chargen.Weave) bool {
	for _, w := range allowed {
		if w.ID == weaveID {
			return true
		}
	}
	return false
}

// filterByTeacher narrows allowed by the teacher's level cap and
// affinity_filter bitmask. Affinity zero is the "any in-affinity"
// sentinel — pass-through, no Power restriction.
func filterByTeacher(in []*chargen.Weave, t *resolvedWeaveTeacher) []*chargen.Weave {
	out := make([]*chargen.Weave, 0, len(in))
	for _, w := range in {
		if int8(w.Level) > t.teacher.MaxLevelTaught {
			continue
		}
		if t.teacher.AffinityFilter != 0 {
			idx := powerIndex(w.Power)
			if idx < 0 {
				slog.Warn("learn weave: unknown power on weave",
					"weave", w.ID, "power", w.Power)
				continue
			}
			if t.teacher.AffinityFilter&(1<<uint(idx)) == 0 {
				continue
			}
		}
		out = append(out, w)
	}
	return out
}

// writeLearnWeaveMenu renders affinity-eligible weaves with an
// already-known marker. With a teacher present, the menu shows the
// teacher's byline + the per-weave practice-point cost; without a
// teacher, it shows the pending_weaves balance.
func writeLearnWeaveMenu(s *telnet.Session, char repo.Character,
	allowed []*chargen.Weave, teacher *resolvedWeaveTeacher,
) error {
	if err := display.SectionHeader(s, "Weave Training"); err != nil {
		return err
	}
	if teacher != nil {
		if err := display.FieldRow(s, "Teacher",
			display.Defang(teacher.keeperName(), ""), 18); err != nil {
			return err
		}
		if err := display.FieldRow(s, "Practice points",
			strconv.Itoa(int(char.PracticePoints)), 18); err != nil {
			return err
		}
	} else {
		if err := display.FieldRow(s, "Weaves available",
			strconv.Itoa(int(char.PendingWeaves)), 18); err != nil {
			return err
		}
	}
	if err := display.FieldRow(s, "Affinities",
		formatAffinities(char.Channeling.Affinities), 18); err != nil {
		return err
	}
	if len(allowed) == 0 {
		if teacher != nil {
			return s.WriteString("\r\n  {{(this teacher has nothing to teach you)}}::gray\r\n")
		}
		return s.WriteString("\r\n  {{(no in-affinity weaves available)}}::gray\r\n")
	}
	if err := display.Subsection(s, "Available weaves"); err != nil {
		return err
	}
	known := weaveKnownSet(char)
	for i, w := range allowed {
		marker := "  "
		if _, ok := known[w.ID]; ok {
			marker = "✓ "
		}
		costStr := ""
		if teacher != nil {
			costStr = fmt.Sprintf("  cost %d", w.PracticeCost)
		}
		if err := s.WriteString(fmt.Sprintf(
			"  {{%2d)}}::gray %s{{%-22s}}::yellow|bold {{%-6s}}::gray  L%d%s\r\n",
			i+1, marker,
			display.Defang(w.Name, ""),
			display.Defang(w.Power, ""),
			w.Level,
			costStr,
		)); err != nil {
			return err
		}
	}
	return s.WriteString(
		"\r\n  Usage: {{learn weave <id>}}::yellow|bold  ·  " +
			"{{learn weave info <id>}}::yellow\r\n",
	)
}

// writeLearnWeaveInfo prints the descriptor for one weave. Read-only.
func writeLearnWeaveInfo(s *telnet.Session, token string, allowed []*chargen.Weave,
	cat *chargen.Catalog,
) error {
	id, ok := matchWeaveToken(token, allowed, cat)
	if !ok {
		return s.WriteString("{{No such weave.}}::yellow\r\n")
	}
	w, _ := cat.Weave(id)
	if w == nil {
		return s.WriteString("{{No description on file.}}::yellow\r\n")
	}
	if err := s.WriteString(fmt.Sprintf(
		"{{%s (%s)}}::cyan|bold\r\n",
		display.Defang(w.Name, ""), display.Defang(w.ID, ""),
	)); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Power", w.Power, 14); err != nil {
		return err
	}
	if err := display.FieldRow(s, "Level", strconv.Itoa(w.Level), 14); err != nil {
		return err
	}
	if w.Description != "" {
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
		if err := s.WriteWrapped(strings.TrimRight(w.Description, "\n")); err != nil {
			return err
		}
		if err := s.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return display.Rule(s)
}

// weaveAlreadyKnown reports whether the character already has weaveID
// in WeavesKnownIDs. On hit, it emits the cfmt refusal and returns
// (true, writeErr) so the caller can `if known, err := ...; known {
// return err }`.
func weaveAlreadyKnown(s *telnet.Session, char repo.Character, weaveID string,
	cat *chargen.Catalog,
) (bool, error) {
	for _, existing := range char.Channeling.WeavesKnownIDs {
		if existing == weaveID {
			w, _ := cat.Weave(weaveID)
			name := weaveID
			if w != nil {
				name = w.Name
			}
			return true, s.WriteString(fmt.Sprintf(
				"{{You already know %s.}}::yellow\r\n",
				display.Defang(name, ""),
			))
		}
	}
	return false, nil
}

// commitLearnWeave enforces budget + duplicate + affinity gate for
// the chargen-pool path. The affinity gate is implicit because
// matchWeaveToken only resolves against allowedWeavesFor — but we
// re-check here as defense in depth in case a future caller bypasses
// the menu.
func commitLearnWeave(c *telnet.Context, characters repo.CharacterRepo,
	audits repo.AdminAuditRepo, char repo.Character, cat *chargen.Catalog,
	weaveID string,
) error {
	s := c.Session
	if char.PendingWeaves <= 0 {
		return s.WriteString("{{No weaves to learn.}}::yellow\r\n")
	}
	if known, err := weaveAlreadyKnown(s, char, weaveID, cat); known {
		return err
	}
	w, _ := cat.Weave(weaveID)
	if w == nil {
		return s.WriteString("{{No such weave.}}::yellow\r\n")
	}
	if !weaveInAffinity(w, char.Channeling.Affinities) {
		return s.WriteString(fmt.Sprintf(
			"{{%s is not in your affinity.}}::yellow\r\n",
			display.Defang(w.Power, ""),
		))
	}
	newPending := char.PendingWeaves - 1
	if err := characters.RecordWeavePick(c.Ctx, char.ID, weaveID, newPending); err != nil {
		slog.Error("learn weave: record weave pick",
			"char", char.ID, "weave", weaveID, "error", err)
		return s.WriteString("{{The weave unravels in your hands.}}::red\r\n")
	}
	audit.Record(c.Ctx, audits, s, "learn", weaveID,
		fmt.Sprintf("kind=weave power=%s", strings.ToLower(w.Power)))

	weaves := "weaves remain"
	if newPending == 1 {
		weaves = "weave remains"
	}
	return s.WriteString(fmt.Sprintf(
		"{{You weave %s.}}::green|bold  (%s)  %d %s.\r\n",
		display.Defang(w.Name, ""), display.Defang(w.Power, ""),
		newPending, weaves,
	))
}

// commitWeaveStudy is the Phase E #28 mid-game commit path. Drains
// practice_points instead of pending_weaves; refuses on insufficient
// PP, duplicate pick, miss-affinity, or weaves outside the teacher's
// offerings (defense in depth — runLearnWeave already filtered the
// allowed list before resolving the token, but a future caller that
// bypasses the menu must hit the gate here).
func commitWeaveStudy(c *telnet.Context, characters repo.CharacterRepo,
	audits repo.AdminAuditRepo, char repo.Character, cat *chargen.Catalog,
	weaveID string, teacher *resolvedWeaveTeacher, allowed []*chargen.Weave,
) error {
	s := c.Session
	if known, err := weaveAlreadyKnown(s, char, weaveID, cat); known {
		return err
	}
	if !weaveInAllowed(weaveID, allowed) {
		return s.WriteString("{{No such weave.}}::yellow\r\n")
	}
	w, _ := cat.Weave(weaveID)
	if w == nil {
		return s.WriteString("{{No such weave.}}::yellow\r\n")
	}
	if !weaveInAffinity(w, char.Channeling.Affinities) {
		return s.WriteString(fmt.Sprintf(
			"{{%s is not in your affinity.}}::yellow\r\n",
			display.Defang(w.Power, ""),
		))
	}
	if int(char.PracticePoints) < w.PracticeCost {
		return s.WriteString(fmt.Sprintf(
			"{{%s shakes their head. \"You need %d practice points to study %s.\"}}::yellow\r\n",
			display.Defang(teacher.keeperName(), ""),
			w.PracticeCost,
			display.Defang(w.Name, ""),
		))
	}
	newPP := char.PracticePoints - int32(w.PracticeCost)
	if err := characters.RecordWeaveStudy(c.Ctx, char.ID, weaveID, newPP); err != nil {
		slog.Error("learn weave: record weave study",
			"char", char.ID, "weave", weaveID, "error", err)
		return s.WriteString("{{The weave unravels in your hands.}}::red\r\n")
	}
	audit.Record(c.Ctx, audits, s, "learn", weaveID,
		fmt.Sprintf("kind=weave_study power=%s cost=%d",
			strings.ToLower(w.Power), w.PracticeCost))

	pluralPP := "points remain"
	if newPP == 1 {
		pluralPP = "point remains"
	}
	return s.WriteString(fmt.Sprintf(
		"{{%s teaches you %s.}}::green|bold  (%s)  %d practice %s.\r\n",
		display.Defang(teacher.keeperName(), ""),
		display.Defang(w.Name, ""), display.Defang(w.Power, ""),
		newPP, pluralPP,
	))
}

// allowedWeavesFor returns affinity-eligible chargen-catalog weaves
// at the catalog's currently-authored levels (V1 = level 0). Sorted
// by ID for stable menu order.
func allowedWeavesFor(ch repo.Character, cat *chargen.Catalog) []*chargen.Weave {
	if cat == nil || ch.Channeling == nil {
		return nil
	}
	out := make([]*chargen.Weave, 0, 8)
	for _, w := range cat.Weaves() {
		if !weaveInAffinity(w, ch.Channeling.Affinities) {
			continue
		}
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// weaveInAffinity returns true when the weave's Power is set in the
// character's Affinities bitmask. Unrecognized Power tokens are
// rejected (defensive against catalog typos).
func weaveInAffinity(w *chargen.Weave, aff creature.PowerSet) bool {
	if w == nil {
		return false
	}
	idx := powerIndex(w.Power)
	if idx < 0 {
		return false
	}
	return aff&(1<<uint(idx)) != 0
}

// powerIndex maps a YAML power token to the creature.Power enum value
// (0..4). Returns -1 on miss.
func powerIndex(token string) int {
	t := strings.ToLower(strings.TrimSpace(token))
	for i, name := range learnWeavePowerNames {
		if t == strings.ToLower(name) {
			return i
		}
	}
	return -1
}

// formatAffinities renders the PowerSet as a comma-joined list ("Fire,
// Spirit") for the menu header.
func formatAffinities(ps creature.PowerSet) string {
	parts := make([]string, 0, 5)
	for i, name := range learnWeavePowerNames {
		if ps&(1<<uint(i)) != 0 {
			parts = append(parts, name)
		}
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}

// weaveKnownSet collapses the character's WeavesKnownIDs into a set
// for the menu's ✓ marker.
func weaveKnownSet(ch repo.Character) map[string]struct{} {
	out := make(map[string]struct{}, len(ch.Channeling.WeavesKnownIDs))
	for _, id := range ch.Channeling.WeavesKnownIDs {
		out[id] = struct{}{}
	}
	return out
}

// matchWeaveToken resolves a player-typed token to a catalog weave id
// against the affinity-filtered allowed list.
func matchWeaveToken(token string, allowed []*chargen.Weave, cat *chargen.Catalog) (string, bool) {
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
	for _, w := range allowed {
		if strings.EqualFold(w.ID, t) || strings.EqualFold(w.Name, t) {
			return w.ID, true
		}
	}
	return "", false
}
