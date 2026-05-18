package cmd

import (
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/emote"
	"github.com/Jasrags/WheelMUD/internal/help"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// ReloadDeps bundles everything the §M.6 `reload` verb needs. Kept
// as a named struct rather than positional args because the verb
// already has 6+ collaborators and the call site in registry.go is
// easier to read with field names.
//
// emoteCatalog: in-place mutated via Catalog.Replace so that
// NewSocialsList (which holds the *Catalog pointer) sees new
// state without a pointer swap.
//
// helpCatalog: in-place reloaded via Catalog.ReloadFS, then
// re-MergeGenerated against the current registry.
//
// registry: the live telnet.Registry. `reload socials` calls
// Unregister/Register on this to swap per-social verbs.
//
// sessions, mobs: passed straight through to buildSocialCommand
// so re-registered socials retain their target-resolution wiring.
//
// audits: admin_audit sink; nil-safe in audit.Record.
type ReloadDeps struct {
	EmoteCatalog *emote.Catalog
	HelpCatalog  *help.Catalog
	Registry     *telnet.Registry
	Sessions     *session.Registry
	Mobs         repo.MobInstanceRepo
	Audits       repo.AdminAuditRepo
}

// NewReload builds the §M.6 admin verb. AuthAdmin; subcommands
// `socials` and `help`. No-arg form lists supported subsystems.
func NewReload(deps ReloadDeps) *telnet.Command {
	return &telnet.Command{
		Name:    "reload",
		Help:    "reload <subsystem> — hot-reload socials or help",
		Long:    "reload socials — re-read EMOTE_DIR catalog, re-register social verbs\nreload help    — re-read HELP_DIR topics, re-merge command help\nreload         — list subsystems",
		Auth:    telnet.AuthAdmin,
		MinArgs: 0,
		Run: func(c *telnet.Context) error {
			if len(c.Args) == 0 {
				return c.Session.WriteString("{{Subsystems: socials, help}}::yellow\r\n")
			}
			switch strings.ToLower(c.Args[0]) {
			case "socials":
				return runReloadSocials(deps, c)
			case "help":
				return runReloadHelp(deps, c)
			default:
				return c.Session.WriteString("{{Unknown subsystem.  Try: socials | help}}::yellow\r\n")
			}
		},
		Completer: func(_ *telnet.Session, args string) []telnet.Candidate {
			slot, partial := completerSlot(args)
			if slot != 0 {
				return nil
			}
			out := []telnet.Candidate{}
			for _, name := range []string{"socials", "help"} {
				if strings.HasPrefix(name, strings.ToLower(partial)) {
					out = append(out, telnet.Candidate{Text: name, Help: "subsystem"})
				}
			}
			return out
		},
	}
}

func runReloadSocials(deps ReloadDeps, c *telnet.Context) error {
	if deps.EmoteCatalog == nil || deps.Registry == nil {
		return c.Session.WriteString("{{reload: socials wiring missing.}}::red\r\n")
	}
	fsys, err := emote.SourceFS()
	if err != nil {
		return c.Session.WriteString(fmt.Sprintf("{{reload socials: %s}}::red\r\n", err))
	}
	newCat, err := emote.Load(fsys)
	if err != nil {
		return c.Session.WriteString(fmt.Sprintf("{{reload socials: parse failed: %s}}::red\r\n", err))
	}

	oldSocials := deps.EmoteCatalog.All()
	newSocials := newCat.All()

	// Build the diff: id sets, kept/changed detection.
	oldByID := map[string]emote.Social{}
	for _, s := range oldSocials {
		oldByID[s.ID] = s
	}
	newByID := map[string]emote.Social{}
	for _, s := range newSocials {
		newByID[s.ID] = s
	}

	var added, removed, kept []string
	for id := range newByID {
		if _, ok := oldByID[id]; ok {
			kept = append(kept, id)
		} else {
			added = append(added, id)
		}
	}
	for id := range oldByID {
		if _, ok := newByID[id]; !ok {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(kept)

	// Pre-validate: any new id or alias must either be currently owned
	// by an old social (about to be Unregistered) or be unknown to the
	// registry. Otherwise we'd collide with a non-social verb mid-loop
	// and leave the registry partially mutated.
	oldOwned := map[string]struct{}{}
	for _, s := range oldSocials {
		oldOwned[s.ID] = struct{}{}
		for _, a := range s.Aliases {
			oldOwned[a] = struct{}{}
		}
	}
	for _, s := range newSocials {
		for _, name := range append([]string{s.ID}, s.Aliases...) {
			if _, owned := oldOwned[name]; owned {
				continue
			}
			if _, lookupErr := deps.Registry.Lookup(name); lookupErr == nil {
				return c.Session.WriteString(fmt.Sprintf(
					"{{reload socials: %s collides with existing non-social verb; aborted.}}::red\r\n",
					name))
			}
		}
	}

	// Mutate-in-place so NewSocialsList (which holds the *Catalog
	// pointer) sees new state. Per-social commands still need the
	// Unregister/Register pass below because §M.3 closures capture
	// Social by value.
	deps.EmoteCatalog.Replace(newCat)

	// Apply registry changes. Anything that fails here is "shouldn't
	// happen" thanks to pre-validation; log loudly but keep going.
	var changed []string
	for _, id := range removed {
		if err := deps.Registry.Unregister(id); err != nil {
			slog.Error("reload socials: Unregister removed", "id", id, "err", err)
		}
	}
	for _, id := range kept {
		if err := deps.Registry.Unregister(id); err != nil {
			slog.Error("reload socials: Unregister kept", "id", id, "err", err)
			continue
		}
		s := newByID[id]
		if err := deps.Registry.Register(buildSocialCommand(s, deps.Sessions, deps.Mobs)); err != nil {
			slog.Error("reload socials: Register kept", "id", id, "err", err)
			continue
		}
		if socialChanged(oldByID[id], s) {
			changed = append(changed, id)
		}
	}
	for _, id := range added {
		s := newByID[id]
		if err := deps.Registry.Register(buildSocialCommand(s, deps.Sessions, deps.Mobs)); err != nil {
			slog.Error("reload socials: Register added", "id", id, "err", err)
		}
	}

	// Re-attach the §M.1 auto-help layer so new socials get help
	// topics. We don't surface help counts to the operator — the
	// admin asked about socials.
	if deps.HelpCatalog != nil {
		if a, _ := deps.HelpCatalog.MergeGenerated(GenerateCommandTopics(deps.Registry)); a > 0 {
			slog.Debug("reload socials: help topics merged", "added", a)
		}
	}

	msg := formatReloadCounts("reload socials", len(newSocials), added, removed, changed)
	audit.Record(c.Ctx, deps.Audits, c.Session, "reload", "socials",
		fmt.Sprintf("added=%d removed=%d changed=%d", len(added), len(removed), len(changed)))
	return c.Session.WriteString(msg + "\r\n")
}

func runReloadHelp(deps ReloadDeps, c *telnet.Context) error {
	if deps.HelpCatalog == nil {
		return c.Session.WriteString("{{reload: help wiring missing.}}::red\r\n")
	}
	fsys, err := help.SourceFS()
	if err != nil {
		return c.Session.WriteString(fmt.Sprintf("{{reload help: %s}}::red\r\n", err))
	}
	added, removed, changed, err := deps.HelpCatalog.ReloadFS(fsys)
	if err != nil {
		return c.Session.WriteString(fmt.Sprintf("{{reload help: parse failed: %s}}::red\r\n", err))
	}
	// Re-attach the auto-generated command topics. The Reload above
	// blew away both authored and generated; MergeGenerated brings
	// the generated set back. Help-side `added` counts the actual
	// new authored topics — the merged-generated count is purely
	// informational.
	genAdded := 0
	if deps.Registry != nil {
		genAdded, _ = deps.HelpCatalog.MergeGenerated(GenerateCommandTopics(deps.Registry))
	}
	msg := fmt.Sprintf(
		"{{reload help: }}::green{{authored added=%d removed=%d changed=%d; }}::cyan{{generated topics=%d.}}::gray",
		added, removed, changed, genAdded)

	// Subtract the generated topics that were "added" so the audit
	// row reflects only authored deltas (the operator can't author
	// generated topics).
	if removed >= genAdded {
		removed -= genAdded
	} else {
		removed = 0
	}
	audit.Record(c.Ctx, deps.Audits, c.Session, "reload", "help",
		fmt.Sprintf("added=%d removed=%d changed=%d", added, removed, changed))
	return c.Session.WriteString(msg + "\r\n")
}

// socialChanged returns true when two same-id socials differ in any
// rendered field. Used to flag the operator-visible `changed` count.
func socialChanged(a, b emote.Social) bool {
	if a.Help != b.Help ||
		a.Self != b.Self ||
		a.Other != b.Other ||
		a.TargetSelf != b.TargetSelf ||
		a.TargetView != b.TargetView ||
		a.TargetOther != b.TargetOther {
		return true
	}
	if !reflect.DeepEqual(a.Aliases, b.Aliases) {
		return true
	}
	return false
}

// formatReloadCounts renders the per-item summary line shown to the
// operator. ids in each group are space-joined and capped at a
// reasonable preview length so a 100-item reload doesn't wrap into
// the next screen.
func formatReloadCounts(prefix string, total int, added, removed, changed []string) string {
	parts := []string{}
	if len(added) > 0 {
		parts = append(parts, fmt.Sprintf("added %d: %s", len(added), previewIDs(added)))
	}
	if len(removed) > 0 {
		parts = append(parts, fmt.Sprintf("removed %d: %s", len(removed), previewIDs(removed)))
	}
	if len(changed) > 0 {
		parts = append(parts, fmt.Sprintf("changed %d: %s", len(changed), previewIDs(changed)))
	}
	body := "no changes"
	if len(parts) > 0 {
		body = strings.Join(parts, "; ")
	}
	return fmt.Sprintf("{{%s: }}::green{{%d loaded (%s)}}::cyan", prefix, total, body)
}

func previewIDs(ids []string) string {
	const maxShow = 8
	if len(ids) <= maxShow {
		return strings.Join(ids, " ")
	}
	return strings.Join(ids[:maxShow], " ") + " …"
}

