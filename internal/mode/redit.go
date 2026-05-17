package mode

import (
	"context"
	"errors"
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
// Subcommands:
//
//	show                         render the current draft
//	name <new>                   set the room name (one line)
//	short <new>                  set the short description (one line)
//	desc <new>                   set the long description (one line)
//	desc                         enter multi-line buffer mode for the
//	                             long description; finish with `.` on
//	                             its own line, or `@abort` to discard
//	flag <name> [on|off]         toggle / set a flag
//	sector <name>                set the sector (validated)
//	light <n>                    set LightLevel (0 = pitch black,
//	                             100 = full)
//	extra                        list extra-description keywords
//	extra <kw>                   show one extra description
//	extra <kw> <text>            set / replace (one line)
//	extra <kw> .                 multi-line set (terminated by `.`)
//	extra <kw> delete            remove
//	exit <dir>                   show an exit's authoring fields
//	exit <dir> desc <text>       set the exit description
//	exit <dir> key <id|none>     set / clear the key external id
//	exit <dir> difficulty <n>    set lock difficulty (0-100)
//	exit <dir> flag <f> [on|off] toggle pickable / hidden / nopass
//	exit <dir> to <room_ext>     change destination room
//	done                         commit the draft + pop the mode
//	cancel                       pop without committing
//	help                         print the subcommand list
//
// Exit edits write through ExitRepo.Update immediately on each
// subcommand (not buffered into the draft) because the room draft
// model only carries Room fields; per-subcommand exit writes also
// match how runtime door verbs already mutate exits. Each successful
// exit edit emits its own admin_audit row (verb `redit_exit`).
type REdit struct {
	rooms     repo.RoomRepo
	exits     repo.ExitRepo
	audits    repo.AdminAuditRepo
	lookupExt func(ctx context.Context, externalID string) (repo.Room, error)

	original repo.Room // snapshot on entry; used to compute the audit diff
	draft    repo.Room // buffered edits; written on `done`
	dirty    bool      // any subcommand that mutated the draft sets this

	// Multi-line buffering state. When bufActive is true, Handle
	// short-circuits the verb switch and appends each input line to
	// bufLines until a "." line flushes or "@abort" discards.
	// bufKind is either "desc" (target Room.LongDesc) or
	// "extra:<keyword>" (target ExtraDescs[keyword]).
	bufActive bool
	bufKind   string
	bufLines  []string
}

// RoomLookupFn resolves a room by ExternalID for the `exit <dir> to
// <room_ext>` retarget subcommand. Returns repo.ErrRoomNotFound when
// no row matches.
type RoomLookupFn func(ctx context.Context, externalID string) (repo.Room, error)

// NewREdit constructs a redit mode bound to the supplied room. The
// caller (the `redit` verb) is responsible for the permission check —
// REdit does not consult builder_zones itself.
func NewREdit(
	rooms repo.RoomRepo,
	exits repo.ExitRepo,
	audits repo.AdminAuditRepo,
	lookup RoomLookupFn,
	room repo.Room,
) *REdit {
	return &REdit{
		rooms:     rooms,
		exits:     exits,
		audits:    audits,
		lookupExt: lookup,
		original:  room,
		draft:     room,
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
// the prompt without action. When a multi-line buffer is active,
// input bypasses the verb switch and feeds the buffer instead.
func (m *REdit) Handle(ctx context.Context, s *telnet.Session, line string) error {
	if m.bufActive {
		// Buffering mode: preserve leading/trailing whitespace on
		// content lines so prose indentation round-trips, but trim
		// the terminator/abort sentinels so a trailing space doesn't
		// trap the operator. The raw line is used verbatim for
		// content; only the trimmed form drives the sentinel check.
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case ".":
			return m.flushBuffer(s)
		case "@abort":
			return m.abortBuffer(s)
		}
		m.bufLines = append(m.bufLines, line)
		return nil
	}
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
	case "extra":
		return m.setExtra(s, rest)
	case "exit":
		return m.handleExit(ctx, s, rest)
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
		"  {{show}}::yellow                       render the current draft\r\n" +
		"  {{name}}::yellow <new>                 set the room name\r\n" +
		"  {{short}}::yellow <new>                set the short description\r\n" +
		"  {{desc}}::yellow <new>                 set long description (one line)\r\n" +
		"  {{desc}}::yellow                       multi-line; end with '.' or '@abort'\r\n" +
		"  {{flag}}::yellow <name> [on|off]       toggle / set a room flag\r\n" +
		"  {{sector}}::yellow <name>              set the sector\r\n" +
		"  {{light}}::yellow <0-100>              set the light level\r\n" +
		"  {{extra}}::yellow                      list extra-desc keywords\r\n" +
		"  {{extra}}::yellow <kw>                 show one extra description\r\n" +
		"  {{extra}}::yellow <kw> <text>          set / replace (one line)\r\n" +
		"  {{extra}}::yellow <kw> .               multi-line set (end with '.')\r\n" +
		"  {{extra}}::yellow <kw> delete          remove\r\n" +
		"  {{exit}}::yellow <dir>                 show an exit's authoring fields\r\n" +
		"  {{exit}}::yellow <dir> desc <text>     set exit description\r\n" +
		"  {{exit}}::yellow <dir> key <id|none>   set / clear key external id\r\n" +
		"  {{exit}}::yellow <dir> difficulty <n>  set lock difficulty (0-100)\r\n" +
		"  {{exit}}::yellow <dir> flag <f> [on|off]  toggle pickable/hidden/nopass\r\n" +
		"  {{exit}}::yellow <dir> to <room_ext>   change destination room\r\n" +
		"  {{done}}::yellow                       commit changes and exit\r\n" +
		"  {{cancel}}::yellow                     abandon changes and exit\r\n" +
		"\r\nRoom flags: indoors, nopvp, noteleport, dark, silent, peaceful, nomap, bindable\r\n" +
		"Exit flags: pickable, hidden, nopass\r\n" +
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
		m.bufActive = true
		m.bufKind = "desc"
		m.bufLines = m.bufLines[:0]
		return s.WriteString(
			"{{Enter long description. Finish with a single '.' on its own line. " +
				"Type '@abort' on its own line to cancel.}}::yellow\r\n",
		)
	}
	m.draft.LongDesc = v
	m.dirty = true
	return s.WriteString("{{Long description set.}}::green\r\n")
}

// flushBuffer commits the multi-line accumulator into whichever field
// bufKind targeted, clears buffering state, and confirms to the
// operator. Empty buffers still flush — a builder who types `desc`
// then `.` is explicitly clearing the field.
func (m *REdit) flushBuffer(s *telnet.Session) error {
	body := strings.Join(m.bufLines, "\n")
	kind := m.bufKind
	m.bufActive = false
	m.bufKind = ""
	m.bufLines = m.bufLines[:0]
	switch {
	case kind == "desc":
		m.draft.LongDesc = body
		m.dirty = true
		return s.WriteString(fmt.Sprintf(
			"{{Long description set (%d line%s).}}::green\r\n",
			lineCount(body), plural(lineCount(body)),
		))
	case strings.HasPrefix(kind, "extra:"):
		key := strings.TrimPrefix(kind, "extra:")
		if m.draft.ExtraDescs == nil {
			m.draft.ExtraDescs = map[string]string{}
		}
		m.draft.ExtraDescs[key] = body
		m.dirty = true
		return s.WriteString(fmt.Sprintf(
			"{{Extra %q set (%d line%s).}}::green\r\n",
			defangCfmt(key), lineCount(body), plural(lineCount(body)),
		))
	default:
		// Unknown kind shouldn't happen — log defensively and reset.
		slog.Warn("redit: flushBuffer with unknown kind", "kind", kind)
		return s.WriteString("{{Buffer discarded (internal state error).}}::red\r\n")
	}
}

// abortBuffer drops the accumulator without touching the draft.
func (m *REdit) abortBuffer(s *telnet.Session) error {
	m.bufActive = false
	m.bufKind = ""
	m.bufLines = m.bufLines[:0]
	return s.WriteString("{{Edit aborted. Draft unchanged.}}::yellow\r\n")
}

// lineCount returns 0 for empty bodies and the count of LF-separated
// lines otherwise. Used only for the operator-facing confirmation.
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
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
		"bindable":   &m.draft.Flags.Bindable,
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

// setExtra dispatches the `extra ...` family. Keywords are
// case-insensitive on input; ExtraDescs keys are normalized to
// lowercase to match the convention documented at
// repo/room.go (ExtraDescs comment).
func (m *REdit) setExtra(s *telnet.Session, rest string) error {
	if rest == "" {
		return m.listExtras(s)
	}
	kw, tail, _ := strings.Cut(rest, " ")
	kw = strings.ToLower(strings.TrimSpace(kw))
	tail = strings.TrimSpace(tail)
	if kw == "" {
		return s.WriteString("{{Usage: extra <keyword> [text|delete|.]}}::yellow\r\n")
	}
	switch {
	case tail == "":
		return m.showExtra(s, kw)
	case strings.EqualFold(tail, "delete") || strings.EqualFold(tail, "remove"):
		return m.deleteExtra(s, kw)
	case tail == ".":
		m.bufActive = true
		m.bufKind = "extra:" + kw
		m.bufLines = m.bufLines[:0]
		return s.WriteString(fmt.Sprintf(
			"{{Enter extra description for %q. Finish with '.' on its own line; '@abort' to cancel.}}::yellow\r\n",
			defangCfmt(kw),
		))
	default:
		if m.draft.ExtraDescs == nil {
			m.draft.ExtraDescs = map[string]string{}
		}
		m.draft.ExtraDescs[kw] = tail
		m.dirty = true
		return s.WriteString(fmt.Sprintf(
			"{{Extra %q set.}}::green\r\n", defangCfmt(kw),
		))
	}
}

func (m *REdit) listExtras(s *telnet.Session) error {
	if len(m.draft.ExtraDescs) == 0 {
		return s.WriteString("{{No extra descriptions on this room.}}::yellow\r\n")
	}
	keys := make([]string, 0, len(m.draft.ExtraDescs))
	for k := range m.draft.ExtraDescs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("{{Extra descriptions:}}::yellow\r\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "  %s\r\n", defangCfmt(k))
	}
	return s.WriteString(b.String())
}

func (m *REdit) showExtra(s *telnet.Session, kw string) error {
	body, ok := m.draft.ExtraDescs[kw]
	if !ok {
		return s.WriteString(fmt.Sprintf(
			"{{No extra %q on this room.}}::yellow\r\n", defangCfmt(kw),
		))
	}
	// Body stays undefanged — extras are authored content, so builders
	// can use cfmt markup in the prose intentionally (same policy as
	// LongDesc rendering in writeShow). The keyword is operator input
	// and stays defanged.
	return s.WriteString(fmt.Sprintf("{{Extra %q:}}::yellow\r\n%s\r\n",
		defangCfmt(kw), body))
}

func (m *REdit) deleteExtra(s *telnet.Session, kw string) error {
	if _, ok := m.draft.ExtraDescs[kw]; !ok {
		return s.WriteString(fmt.Sprintf(
			"{{No extra %q on this room.}}::yellow\r\n", defangCfmt(kw),
		))
	}
	delete(m.draft.ExtraDescs, kw)
	m.dirty = true
	return s.WriteString(fmt.Sprintf("{{Extra %q deleted.}}::green\r\n", defangCfmt(kw)))
}

// reditDirectionAliases maps every accepted spelling to a canonical
// short code. Kept local to the mode package — `internal/cmd/door.go`
// carries its own copy for the runtime door verbs; the duplication is
// preferable to a third package taking on the dependency.
var reditDirectionAliases = map[string]string{
	"n": repo.DirNorth, "north": repo.DirNorth,
	"s": repo.DirSouth, "south": repo.DirSouth,
	"e": repo.DirEast, "east": repo.DirEast,
	"w": repo.DirWest, "west": repo.DirWest,
	"u": repo.DirUp, "up": repo.DirUp,
	"d": repo.DirDown, "down": repo.DirDown,
	"ne": repo.DirNortheast, "northeast": repo.DirNortheast,
	"nw": repo.DirNorthwest, "northwest": repo.DirNorthwest,
	"se": repo.DirSoutheast, "southeast": repo.DirSoutheast,
	"sw": repo.DirSouthwest, "southwest": repo.DirSouthwest,
}

func resolveReditDir(s string) (string, bool) {
	d, ok := reditDirectionAliases[strings.ToLower(strings.TrimSpace(s))]
	return d, ok
}

// handleExit dispatches the `exit <dir> [<subverb> [args]]` family.
// Exit edits write through ExitRepo.Update immediately (no draft
// buffer); each successful edit emits its own audit row.
func (m *REdit) handleExit(ctx context.Context, s *telnet.Session, rest string) error {
	if rest == "" {
		return s.WriteString("{{Usage: exit <dir> [show|desc|key|difficulty|flag|to] ...}}::yellow\r\n")
	}
	dirTok, tail, _ := strings.Cut(rest, " ")
	tail = strings.TrimSpace(tail)
	dir, ok := resolveReditDir(dirTok)
	if !ok {
		return s.WriteString("{{Unknown direction:}}::red " + defangCfmt(dirTok) + "\r\n")
	}
	ex, err := m.exits.FindByDirection(ctx, m.draft.ID, dir)
	if err != nil {
		if errors.Is(err, repo.ErrExitNotFound) {
			return s.WriteString(fmt.Sprintf(
				"{{No exit %s from this room.}}::yellow\r\n", repo.DirLong(dir),
			))
		}
		slog.Warn("redit: exit lookup failed", "room", m.draft.ID, "dir", dir, "error", err)
		return s.WriteString("{{Exit lookup failed.}}::red\r\n")
	}
	if tail == "" {
		return m.showExit(s, ex)
	}
	sub, args, _ := strings.Cut(tail, " ")
	args = strings.TrimSpace(args)
	switch strings.ToLower(sub) {
	case "show":
		return m.showExit(s, ex)
	case "desc", "description":
		return m.exitSetDesc(ctx, s, ex, args)
	case "key":
		return m.exitSetKey(ctx, s, ex, args)
	case "difficulty", "lockdiff", "diff":
		return m.exitSetDifficulty(ctx, s, ex, args)
	case "flag":
		return m.exitSetFlag(ctx, s, ex, args)
	case "to":
		return m.exitSetTo(ctx, s, ex, args)
	default:
		return s.WriteString("{{Unknown exit subverb:}}::red " + defangCfmt(sub) +
			"\r\n  Try: show, desc, key, difficulty, flag, to\r\n")
	}
}

func (m *REdit) showExit(s *telnet.Session, ex repo.Exit) error {
	var b strings.Builder
	fmt.Fprintf(&b, "{{Exit %s (#%d):}}::cyan|bold\r\n",
		repo.DirLong(ex.Direction), ex.ID)
	fmt.Fprintf(&b, "  {{to:}}::yellow         room #%d\r\n", ex.ToRoomID)
	fmt.Fprintf(&b, "  {{desc:}}::yellow       %s\r\n", emptyOr(defangCfmt(ex.Description)))
	fmt.Fprintf(&b, "  {{key:}}::yellow        %s\r\n", emptyOr(defangCfmt(ex.KeyExternalID)))
	fmt.Fprintf(&b, "  {{difficulty:}}::yellow %d\r\n", ex.LockDifficulty)
	fmt.Fprintf(&b, "  {{flags:}}::yellow      %s\r\n", exitFlagSummary(ex.Flags))
	fmt.Fprintf(&b, "  {{runtime:}}::gray      closed=%s locked=%s\r\n",
		boolLabel(ex.Flags.Closed), boolLabel(ex.Flags.Locked))
	return s.WriteString(b.String())
}

func (m *REdit) exitSetDesc(ctx context.Context, s *telnet.Session, ex repo.Exit, v string) error {
	if v == "" {
		return s.WriteString("{{Usage: exit <dir> desc <text>}}::yellow\r\n")
	}
	ex.Description = v
	return m.persistExit(ctx, s, ex, "desc")
}

func (m *REdit) exitSetKey(ctx context.Context, s *telnet.Session, ex repo.Exit, v string) error {
	if v == "" {
		return s.WriteString("{{Usage: exit <dir> key <item_external_id|none>}}::yellow\r\n")
	}
	if strings.EqualFold(v, "none") || strings.EqualFold(v, "clear") {
		ex.KeyExternalID = ""
	} else {
		ex.KeyExternalID = v
	}
	return m.persistExit(ctx, s, ex, "key")
}

func (m *REdit) exitSetDifficulty(ctx context.Context, s *telnet.Session, ex repo.Exit, v string) error {
	if v == "" {
		return s.WriteString("{{Usage: exit <dir> difficulty <0-100>}}::yellow\r\n")
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 100 {
		return s.WriteString("{{Expected an integer in [0, 100].}}::yellow\r\n")
	}
	ex.LockDifficulty = n
	return m.persistExit(ctx, s, ex, "difficulty")
}

func (m *REdit) exitSetFlag(ctx context.Context, s *telnet.Session, ex repo.Exit, v string) error {
	if v == "" {
		return s.WriteString("{{Usage: exit <dir> flag <pickable|hidden|nopass> [on|off]}}::yellow\r\n")
	}
	name, valStr, _ := strings.Cut(v, " ")
	name = strings.ToLower(strings.TrimSpace(name))
	valStr = strings.TrimSpace(valStr)
	var ptr *bool
	switch name {
	case "pickable":
		ptr = &ex.Flags.Pickable
	case "hidden":
		ptr = &ex.Flags.Hidden
	case "nopass":
		ptr = &ex.Flags.NoPass
	default:
		return s.WriteString("{{Unknown exit flag:}}::red " + defangCfmt(name) +
			"\r\n  Editable: pickable, hidden, nopass\r\n")
	}
	var target bool
	switch strings.ToLower(valStr) {
	case "":
		target = !*ptr
	case "on", "true", "1", "yes":
		target = true
	case "off", "false", "0", "no":
		target = false
	default:
		return s.WriteString("{{Expected on/off or no value to toggle.}}::yellow\r\n")
	}
	if *ptr == target {
		return s.WriteString(fmt.Sprintf("{{Flag %s already %s.}}::yellow\r\n",
			name, boolLabel(target)))
	}
	*ptr = target
	return m.persistExit(ctx, s, ex, "flag="+name)
}

func (m *REdit) exitSetTo(ctx context.Context, s *telnet.Session, ex repo.Exit, v string) error {
	if v == "" {
		return s.WriteString("{{Usage: exit <dir> to <room_external_id>}}::yellow\r\n")
	}
	if m.lookupExt == nil {
		return s.WriteString("{{Room lookup unavailable.}}::red\r\n")
	}
	target, err := m.lookupExt(ctx, v)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return s.WriteString("{{No room with external id:}}::red " +
				defangCfmt(v) + "\r\n")
		}
		slog.Warn("redit: room lookup failed", "external", v, "error", err)
		return s.WriteString("{{Room lookup failed.}}::red\r\n")
	}
	ex.ToRoomID = target.ID
	return m.persistExit(ctx, s, ex, "to="+target.ExternalID)
}

// persistExit writes the staged exit through ExitRepo.Update and
// emits a single audit row for the field touched. Audit target
// encodes "<room_ext>:<dir>:<field>" so a grep across audit rows is
// useful.
func (m *REdit) persistExit(ctx context.Context, s *telnet.Session, ex repo.Exit, field string) error {
	if err := m.exits.Update(ctx, ex); err != nil {
		slog.Warn("redit: exit update failed",
			"room", m.draft.ID, "exit", ex.ID, "error", err)
		return s.WriteString("{{Failed to save exit.}}::red\r\n")
	}
	if m.audits != nil {
		target := fmt.Sprintf("%s:%s:%s", m.draft.ExternalID, ex.Direction, field)
		if err := m.audits.Record(ctx, repo.AdminAuditEntry{
			ActorCharacterID: s.CharacterID,
			ActorName:        s.CharacterName,
			ActorType:        repo.ActorTypeCharacter,
			Verb:             "redit_exit",
			Target:           target,
		}); err != nil {
			slog.Warn("redit: exit audit write failed",
				"exit", ex.ID, "field", field, "error", err)
		}
	}
	// `field` for `to=<room_external_id>` carries a builder-authored
	// string that we don't want closing the surrounding `{{..}}::green`
	// context if it ever contained `}}::red ...`. Defang at the render
	// site rather than at each call site so future fields are safe by
	// default.
	return s.WriteString(fmt.Sprintf("{{Exit %s.%s saved.}}::green\r\n",
		repo.DirLong(ex.Direction), defangCfmt(field)))
}

func exitFlagSummary(f repo.ExitFlags) string {
	var on []string
	if f.Pickable {
		on = append(on, "pickable")
	}
	if f.Hidden {
		on = append(on, "hidden")
	}
	if f.NoPass {
		on = append(on, "nopass")
	}
	if len(on) == 0 {
		return "(none)"
	}
	return strings.Join(on, ", ")
}

func emptyOr(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
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
	if f.Bindable {
		on = append(on, "bindable")
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

