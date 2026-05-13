package mode

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// REdit is the in-game room editor mode (Phase G #34). Pushed on top
// of the game mode by the `redit` verb. The mode buffers edits in a
// draft copy of the room; nothing reaches the SQLite world tables
// until the operator types `done`. `cancel` pops without committing.
//
// Permission is gated at PUSH time by cmd.CanEditZone — the mode
// itself trusts that the verb wouldn't have pushed it otherwise. A
// later revoke during an editing session is not enforced inside the
// mode; the operator's `done` succeeds (and is audited) regardless.
// This matches the "permissions are checked at the entry point"
// convention used by `goto` / `summon` / `spawn`.
//
// Subcommands (V1):
//
//	show               render the current draft
//	name <new>         set the room name (one line)
//	short <new>        set the short description (one line)
//	desc <new>         set the long description (one line; multi-line
//	                   deferred to a later slice — for now a single
//	                   line replaces the full body)
//	flag <name> [on|off]  toggle a flag, or set it explicitly
//	sector <name>      set the sector (validated against Sector enum)
//	light <n>          set LightLevel (0 = pitch black, 100 = full)
//	done               commit the draft (UPDATE rooms) + pop the mode
//	cancel             pop without committing (any unsaved edits lost)
//	help               print the subcommand list
//
// Exit-editing (`exit <dir> ...`) and ExtraDescs editing (`extra
// <keyword> ...`) are deferred to a follow-up slice — both need their
// own write surfaces on ExitRepo and a different multi-line buffer.
type REdit struct {
	rooms  repo.RoomRepo
	audits repo.AdminAuditRepo

	original repo.Room // snapshot on entry; used to compute the audit diff
	draft    repo.Room // buffered edits; written on `done`
	dirty    bool      // any subcommand that mutated the draft sets this
}

// NewREdit constructs a redit mode bound to the supplied room ID. The
// caller (the `redit` verb) is responsible for the permission check —
// REdit does not consult builder_zones itself.
func NewREdit(rooms repo.RoomRepo, audits repo.AdminAuditRepo, room repo.Room) *REdit {
	return &REdit{
		rooms:    rooms,
		audits:   audits,
		original: room,
		draft:    room,
	}
}

// OnEnter prints the entry banner + initial show so the operator has
// the room state in front of them immediately.
func (m *REdit) OnEnter(s *telnet.Session) error {
	if err := s.WriteString(fmt.Sprintf(
		"{{Editing room %s (#%d). Type 'help' for commands, 'done' to commit, 'cancel' to abandon.}}::cyan|bold\r\n",
		defangCfmt(m.draft.ExternalID), m.draft.ID,
	)); err != nil {
		return err
	}
	return m.writeShow(s)
}

// OnExit is a no-op — both `done` and `cancel` already wrote their
// closing message before popping.
func (m *REdit) OnExit(_ *telnet.Session) error { return nil }

// Prompt overrides the game prompt so the operator knows they're in
// editor mode. Dirty state surfaces as a `*` marker.
func (m *REdit) Prompt(_ context.Context, _ *telnet.Session) string {
	marker := ""
	if m.dirty {
		marker = "*"
	}
	return fmt.Sprintf("[redit %s%s]> ", m.draft.ExternalID, marker)
}

// Handle dispatches a single subcommand line. Empty input re-renders
// the prompt without action.
func (m *REdit) Handle(ctx context.Context, s *telnet.Session, line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	verb, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimSpace(rest)

	switch strings.ToLower(verb) {
	case "help", "?":
		return m.writeHelp(s)
	case "show", "look", "l":
		return m.writeShow(s)
	case "name":
		return m.setName(s, rest)
	case "short":
		return m.setShort(s, rest)
	case "desc", "description":
		return m.setDesc(s, rest)
	case "flag":
		return m.setFlag(s, rest)
	case "sector":
		return m.setSector(s, rest)
	case "light":
		return m.setLight(s, rest)
	case "done", "commit", "save":
		return m.commit(ctx, s)
	case "cancel", "abort", "quit":
		return m.cancel(s)
	default:
		return s.WriteString("{{Unknown editor command:}}::red " +
			defangCfmt(verb) + ". Type 'help' for the list.\r\n")
	}
}

func (m *REdit) writeHelp(s *telnet.Session) error {
	const body = "Editor commands:\r\n" +
		"  {{show}}::yellow                   render the current draft\r\n" +
		"  {{name}}::yellow <new>             set the room name\r\n" +
		"  {{short}}::yellow <new>            set the short description\r\n" +
		"  {{desc}}::yellow <new>             set the long description (one line)\r\n" +
		"  {{flag}}::yellow <name> [on|off]   toggle / set a flag\r\n" +
		"  {{sector}}::yellow <name>          set the sector\r\n" +
		"  {{light}}::yellow <0-100>          set the light level\r\n" +
		"  {{done}}::yellow                   commit changes and exit\r\n" +
		"  {{cancel}}::yellow                 abandon changes and exit\r\n" +
		"\r\nFlags: indoors, nopvp, noteleport, dark, silent, peaceful, nomap\r\n" +
		"Sectors: city, forest, field, hills, mountain, desert, water,\r\n" +
		"         underwater, air, underground, blight, waste, stedding, swamp\r\n"
	return s.WriteString(body)
}

func (m *REdit) writeShow(s *telnet.Session) error {
	var b strings.Builder
	dirtyTag := ""
	if m.dirty {
		dirtyTag = " {{(uncommitted)}}::yellow"
	}
	fmt.Fprintf(&b, "{{Room:}}::cyan|bold     %s {{(#%d)}}::gray%s\r\n",
		defangCfmt(m.draft.ExternalID), m.draft.ID, dirtyTag)
	// Name + Short are operator-supplied and rendered inside cfmt
	// markup, so defang to prevent a hostile/typo'd `{{...}}::style`
	// run from injecting styling on the editor's own terminal. LongDesc
	// stays undefanged: builders intentionally author cfmt prose for
	// the room description, matching look's treatment.
	fmt.Fprintf(&b, "  {{Name:}}::yellow     %s\r\n", defangCfmt(m.draft.Name))
	if m.draft.ShortDesc != "" {
		fmt.Fprintf(&b, "  {{Short:}}::yellow    %s\r\n", defangCfmt(m.draft.ShortDesc))
	}
	if m.draft.LongDesc != "" {
		fmt.Fprintf(&b, "  {{Desc:}}::yellow     %s\r\n", m.draft.LongDesc)
	} else {
		fmt.Fprintf(&b, "  {{Desc:}}::yellow     (empty)\r\n")
	}
	fmt.Fprintf(&b, "  {{Sector:}}::yellow   %s\r\n", m.draft.Sector)
	fmt.Fprintf(&b, "  {{Light:}}::yellow    %d\r\n", m.draft.LightLevel)
	fmt.Fprintf(&b, "  {{Flags:}}::yellow    %s\r\n", flagSummary(m.draft.Flags))
	return s.WriteString(b.String())
}

func (m *REdit) setName(s *telnet.Session, v string) error {
	if v == "" {
		return s.WriteString("{{Usage: name <new room name>}}::yellow\r\n")
	}
	m.draft.Name = v
	m.dirty = true
	return s.WriteString("{{Name set.}}::green\r\n")
}

func (m *REdit) setShort(s *telnet.Session, v string) error {
	if v == "" {
		return s.WriteString("{{Usage: short <new short description>}}::yellow\r\n")
	}
	m.draft.ShortDesc = v
	m.dirty = true
	return s.WriteString("{{Short description set.}}::green\r\n")
}

func (m *REdit) setDesc(s *telnet.Session, v string) error {
	if v == "" {
		return s.WriteString("{{Usage: desc <new long description>}}::yellow\r\n")
	}
	m.draft.LongDesc = v
	m.dirty = true
	return s.WriteString("{{Long description set.}}::green\r\n")
}

// flagPointers maps the user-facing flag name to a pointer into the
// draft's RoomFlags so set/toggle can mutate it generically. Kept in
// one place so the help text, the validator, and the mutator stay in
// lock-step.
func (m *REdit) flagPointers() map[string]*bool {
	return map[string]*bool{
		"indoors":    &m.draft.Flags.Indoors,
		"nopvp":      &m.draft.Flags.NoPVP,
		"noteleport": &m.draft.Flags.NoTeleport,
		"dark":       &m.draft.Flags.Dark,
		"silent":     &m.draft.Flags.Silent,
		"peaceful":   &m.draft.Flags.Peaceful,
		"nomap":      &m.draft.Flags.NoMap,
	}
}

func (m *REdit) setFlag(s *telnet.Session, rest string) error {
	if rest == "" {
		return s.WriteString("{{Usage: flag <name> [on|off]}}::yellow\r\n")
	}
	name, valStr, _ := strings.Cut(rest, " ")
	name = strings.ToLower(strings.TrimSpace(name))
	valStr = strings.TrimSpace(valStr)
	flags := m.flagPointers()
	ptr, ok := flags[name]
	if !ok {
		return s.WriteString("{{Unknown flag:}}::red " + defangCfmt(name) + "\r\n")
	}
	var target bool
	switch strings.ToLower(valStr) {
	case "":
		target = !*ptr // toggle
	case "on", "true", "1", "yes":
		target = true
	case "off", "false", "0", "no":
		target = false
	default:
		return s.WriteString("{{Expected on/off or no value to toggle.}}::yellow\r\n")
	}
	if *ptr == target {
		return s.WriteString(fmt.Sprintf("{{Flag %s already %s.}}::yellow\r\n", name, boolLabel(target)))
	}
	*ptr = target
	m.dirty = true
	return s.WriteString(fmt.Sprintf("{{Flag %s = %s.}}::green\r\n", name, boolLabel(target)))
}

func (m *REdit) setSector(s *telnet.Session, v string) error {
	if v == "" {
		return s.WriteString("{{Usage: sector <name>}}::yellow\r\n")
	}
	v = strings.ToLower(strings.TrimSpace(v))
	if !isValidSector(repo.Sector(v)) {
		return s.WriteString("{{Unknown sector:}}::red " + defangCfmt(v) + "\r\n")
	}
	m.draft.Sector = repo.Sector(v)
	m.dirty = true
	return s.WriteString(fmt.Sprintf("{{Sector = %s.}}::green\r\n", v))
}

func (m *REdit) setLight(s *telnet.Session, v string) error {
	if v == "" {
		return s.WriteString("{{Usage: light <0-100>}}::yellow\r\n")
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 100 {
		return s.WriteString("{{Expected an integer in [0, 100].}}::yellow\r\n")
	}
	m.draft.LightLevel = n
	m.dirty = true
	return s.WriteString(fmt.Sprintf("{{Light = %d.}}::green\r\n", n))
}

func (m *REdit) commit(ctx context.Context, s *telnet.Session) error {
	if !m.dirty {
		_ = s.WriteString("{{No changes to commit.}}::yellow\r\n")
		return s.PopMode()
	}
	if err := m.rooms.Update(ctx, m.draft); err != nil {
		slog.Warn("redit: room update failed",
			"room", m.draft.ID, "external", m.draft.ExternalID, "error", err)
		return s.WriteString("{{Failed to save changes.}}::red\r\n")
	}
	changes := diffRoom(m.original, m.draft)
	if m.audits != nil {
		// Audit through the AdminAuditRepo directly (not via the
		// internal/audit helper) so this package doesn't have to import
		// internal/cmd's audit wrapper. Mirrors what audit.Record does
		// under the hood — actor snapshot + the change summary as args.
		// Audit-write failures log at warn so a corrupt audit table or
		// transient DB hiccup leaves a forensic trail; the commit
		// itself already landed and we don't roll it back on audit
		// failure (mirrors internal/audit.Record's policy).
		if err := m.audits.Record(ctx, repo.AdminAuditEntry{
			ActorCharacterID: s.CharacterID,
			ActorName:        s.CharacterName,
			ActorType:        repo.ActorTypeCharacter,
			Verb:             "redit",
			Target:           m.draft.ExternalID,
			Args:             strings.Join(changes, ","),
		}); err != nil {
			slog.Warn("redit: audit write failed",
				"room", m.draft.ExternalID, "error", err)
		}
	}
	if err := s.WriteString(fmt.Sprintf(
		"{{Saved %s (%d field%s changed).}}::green\r\n",
		defangCfmt(m.draft.ExternalID), len(changes), plural(len(changes)),
	)); err != nil {
		return err
	}
	return s.PopMode()
}

func (m *REdit) cancel(s *telnet.Session) error {
	msg := "{{Editor closed. No changes saved.}}::yellow\r\n"
	if !m.dirty {
		msg = "{{Editor closed.}}::yellow\r\n"
	}
	_ = s.WriteString(msg)
	return s.PopMode()
}

// flagSummary renders the flag set as a comma-separated list of the
// flags that are ON. Returns "(none)" when nothing is set so the show
// output never blanks out.
func flagSummary(f repo.RoomFlags) string {
	var on []string
	if f.Indoors {
		on = append(on, "indoors")
	}
	if f.NoPVP {
		on = append(on, "nopvp")
	}
	if f.NoTeleport {
		on = append(on, "noteleport")
	}
	if f.Dark {
		on = append(on, "dark")
	}
	if f.Silent {
		on = append(on, "silent")
	}
	if f.Peaceful {
		on = append(on, "peaceful")
	}
	if f.NoMap {
		on = append(on, "nomap")
	}
	if len(on) == 0 {
		return "(none)"
	}
	return strings.Join(on, ", ")
}

func boolLabel(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// isValidSector mirrors the SQLite CHECK constraint added by migrations
// 0012 / 0025. Kept here so the verb's refusal path doesn't have to
// import internal/world (which carries its own validSectors copy).
// Any change to the schema's allowed sector list must update this and
// internal/world/validate.go::validSectors in lock-step.
func isValidSector(s repo.Sector) bool {
	switch s {
	case repo.SectorCity, repo.SectorForest, repo.SectorField, repo.SectorHills,
		repo.SectorMountain, repo.SectorDesert, repo.SectorWater,
		repo.SectorUnderwater, repo.SectorAir, repo.SectorUnderground,
		repo.SectorBlight, repo.SectorWaste, repo.SectorStedding, repo.SectorSwamp:
		return true
	}
	return false
}

// diffRoom returns the list of changed field names between original and
// draft, sorted for deterministic audit args. Only inspects fields the
// editor can mutate.
func diffRoom(orig, draft repo.Room) []string {
	var changes []string
	if orig.Name != draft.Name {
		changes = append(changes, "name")
	}
	if orig.ShortDesc != draft.ShortDesc {
		changes = append(changes, "short")
	}
	if orig.LongDesc != draft.LongDesc {
		changes = append(changes, "desc")
	}
	if orig.Sector != draft.Sector {
		changes = append(changes, "sector")
	}
	if orig.LightLevel != draft.LightLevel {
		changes = append(changes, "light")
	}
	if orig.Flags != draft.Flags {
		changes = append(changes, "flags")
	}
	sort.Strings(changes)
	return changes
}

