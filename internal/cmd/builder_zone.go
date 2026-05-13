package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// CanEditZone reports whether the session may use OLC verbs against
// zoneID. AuthAdmin bypasses the per-zone grant table entirely; lower
// tiers must hold a builder_zones row (cached on the session at login
// by promoteToGame and refreshed by grant / revoke when the target is
// online). Exported as the single permission gate for #34's redit /
// oedit / medit / zedit so the policy lives in one place.
func CanEditZone(s *telnet.Session, zoneID int64) bool {
	if s == nil {
		return false
	}
	if s.AuthLevel >= telnet.AuthAdmin {
		return true
	}
	return s.IsBuilderFor(zoneID)
}

// refreshBuilderZonesForOnline rebuilds the live cache on target's
// session if they're online. Called from runGrant / runRevoke after
// the persistent write lands. Failure logs at debug — the persistent
// grant still took effect, the cache is just briefly stale until next
// login.
func refreshBuilderZonesForOnline(ctx context.Context, sessions *session.Registry, builders repo.BuilderZoneRepo, characterID int64) {
	if sessions == nil {
		return
	}
	target := LookupByCharacterID(sessions, characterID)
	if target == nil {
		return
	}
	rows, err := builders.ListForCharacter(ctx, characterID)
	if err != nil {
		slog.Debug("grant refresh: list failed",
			"char", characterID, "error", err)
		return
	}
	var grants map[int64]struct{}
	if len(rows) > 0 {
		grants = make(map[int64]struct{}, len(rows))
		for _, r := range rows {
			grants[r.ZoneID] = struct{}{}
		}
	}
	target.SetBuilderZones(grants)
	if err := target.WriteAsync("{{Your builder grants have been updated.}}::magenta"); err != nil {
		slog.Debug("grant refresh: notify failed",
			"char", characterID, "error", err)
	}
}

// NewGrant builds the `grant <player> <zone>` admin verb. Issues a
// per-zone builder grant to a character. The target player need not
// be online — grants are persistent and consulted at promoteToGame.
// When the target IS online, the session's cached grant set is
// refreshed in-place so the change takes effect without a re-login.
// AuthAdmin only; audited per Phase A 5.
func NewGrant(builders repo.BuilderZoneRepo, characters repo.CharacterRepo, zones repo.ZoneRepo, sessions *session.Registry, audits repo.AdminAuditRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "grant",
		Help:    "grant <player> <zone> — authorise a player to edit a zone (admin)",
		Long:    "Usage: grant <player> <zone>\n\n<player> is a character name (case-insensitive; need not be online).\n<zone>   is a numeric zone id or its external_id (e.g. emonds_field).",
		Auth:    telnet.AuthAdmin,
		MinArgs: 2,
		Run: func(c *telnet.Context) error {
			return runGrant(c, c.Args[0], c.Args[1], builders, characters, zones, sessions, audits)
		},
	}
}

// NewRevoke builds the `revoke <player> <zone>` admin verb. Inverse
// of `grant`. AuthAdmin only; audited.
func NewRevoke(builders repo.BuilderZoneRepo, characters repo.CharacterRepo, zones repo.ZoneRepo, sessions *session.Registry, audits repo.AdminAuditRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "revoke",
		Help:    "revoke <player> <zone> — remove a per-zone builder grant (admin)",
		Long:    "Usage: revoke <player> <zone>\n\nSee 'grant' for the argument shape.",
		Auth:    telnet.AuthAdmin,
		MinArgs: 2,
		Run: func(c *telnet.Context) error {
			return runRevoke(c, c.Args[0], c.Args[1], builders, characters, zones, sessions, audits)
		},
	}
}

// NewGrants builds the `grants [<player>]` admin viewer.
//
//	grants            — list every grant in the world (zone, player)
//	grants <player>   — list one player's zones
//
// Read-only, AuthAdmin only. Not audited (read-side).
func NewGrants(builders repo.BuilderZoneRepo, characters repo.CharacterRepo, zones repo.ZoneRepo) *telnet.Command {
	return &telnet.Command{
		Name: "grants",
		Help: "grants [<player>] — list builder-zone grants (admin)",
		Long: "Usage: grants            - list every grant in the world\n" +
			"       grants <player>   - list zones granted to <player>\n",
		Auth: telnet.AuthAdmin,
		Run: func(c *telnet.Context) error {
			if len(c.Args) == 0 {
				return runGrantsAll(c, builders, characters, zones)
			}
			return runGrantsForCharacter(c, c.Args[0], builders, characters, zones)
		},
	}
}

func runGrant(c *telnet.Context, playerArg, zoneArg string, builders repo.BuilderZoneRepo, characters repo.CharacterRepo, zones repo.ZoneRepo, sessions *session.Registry, audits repo.AdminAuditRepo) error {
	ch, err := characters.FindByName(c.Ctx, playerArg)
	if errors.Is(err, repo.ErrCharacterNotFound) {
		return c.Session.WriteString("{{No such character:}}::red " + defangCfmt(playerArg) + "\r\n")
	}
	if err != nil {
		return c.Session.WriteString("{{Could not look up that character right now.}}::red\r\n")
	}
	z, ok, err := resolveZone(c, zones, zoneArg)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := builders.Grant(c.Ctx, ch.ID, z.ID, c.Session.CharacterID, time.Now().UTC()); err != nil {
		return c.Session.WriteString("{{Could not record the grant.}}::red\r\n")
	}
	audit.Record(c.Ctx, audits, c.Session, "grant", ch.Name, fmt.Sprintf("zone=%s", z.ExternalID))
	refreshBuilderZonesForOnline(c.Ctx, sessions, builders, ch.ID)
	return c.Session.WriteString("{{Granted}}::green " + defangCfmt(ch.Name) + " {{builder rights on}}::green " + defangCfmt(z.ExternalID) + ".\r\n")
}

func runRevoke(c *telnet.Context, playerArg, zoneArg string, builders repo.BuilderZoneRepo, characters repo.CharacterRepo, zones repo.ZoneRepo, sessions *session.Registry, audits repo.AdminAuditRepo) error {
	ch, err := characters.FindByName(c.Ctx, playerArg)
	if errors.Is(err, repo.ErrCharacterNotFound) {
		return c.Session.WriteString("{{No such character:}}::red " + defangCfmt(playerArg) + "\r\n")
	}
	if err != nil {
		return c.Session.WriteString("{{Could not look up that character right now.}}::red\r\n")
	}
	z, ok, err := resolveZone(c, zones, zoneArg)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	err = builders.Revoke(c.Ctx, ch.ID, z.ID)
	if errors.Is(err, repo.ErrBuilderZoneNotFound) {
		return c.Session.WriteString("{{No such grant exists.}}::yellow\r\n")
	}
	if err != nil {
		return c.Session.WriteString("{{Could not remove the grant.}}::red\r\n")
	}
	audit.Record(c.Ctx, audits, c.Session, "revoke", ch.Name, fmt.Sprintf("zone=%s", z.ExternalID))
	refreshBuilderZonesForOnline(c.Ctx, sessions, builders, ch.ID)
	return c.Session.WriteString("{{Revoked}}::yellow " + defangCfmt(ch.Name) + " {{from}}::yellow " + defangCfmt(z.ExternalID) + ".\r\n")
}

func runGrantsAll(c *telnet.Context, builders repo.BuilderZoneRepo, characters repo.CharacterRepo, zones repo.ZoneRepo) error {
	all, err := zones.List(c.Ctx)
	if err != nil {
		return c.Session.WriteString("{{Could not enumerate zones right now.}}::red\r\n")
	}
	var b strings.Builder
	any := false
	for _, z := range all {
		rows, err := builders.ListForZone(c.Ctx, z.ID)
		if err != nil || len(rows) == 0 {
			continue
		}
		any = true
		fmt.Fprintf(&b, "{{%s}}::cyan|bold (#%d)\r\n", z.ExternalID, z.ID)
		for _, r := range rows {
			name := characterNameOrUnknown(c, characters, r.CharacterID)
			fmt.Fprintf(&b, "  %s\r\n", name)
		}
	}
	if !any {
		return c.Session.WriteString("{{No builder grants are currently in effect.}}::yellow\r\n")
	}
	return c.Session.WriteString(b.String())
}

func runGrantsForCharacter(c *telnet.Context, playerArg string, builders repo.BuilderZoneRepo, characters repo.CharacterRepo, zones repo.ZoneRepo) error {
	ch, err := characters.FindByName(c.Ctx, playerArg)
	if errors.Is(err, repo.ErrCharacterNotFound) {
		return c.Session.WriteString("{{No such character:}}::red " + defangCfmt(playerArg) + "\r\n")
	}
	if err != nil {
		return c.Session.WriteString("{{Could not look up that character right now.}}::red\r\n")
	}
	rows, err := builders.ListForCharacter(c.Ctx, ch.ID)
	if err != nil {
		return c.Session.WriteString("{{Could not list grants right now.}}::red\r\n")
	}
	if len(rows) == 0 {
		return c.Session.WriteString("{{" + defangCfmt(ch.Name) + " has no builder grants.}}::yellow\r\n")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "{{%s}}::cyan|bold may edit:\r\n", defangCfmt(ch.Name))
	for _, r := range rows {
		z, err := zones.GetByID(c.Ctx, r.ZoneID)
		label := fmt.Sprintf("#%d", r.ZoneID)
		if err == nil {
			label = z.ExternalID
		}
		fmt.Fprintf(&b, "  %s\r\n", label)
	}
	return c.Session.WriteString(b.String())
}

// resolveZone parses zoneArg as either a numeric ID or an external_id
// and returns the matching repo.Zone. The bool reports whether a zone
// was resolved; on a known miss (ErrZoneNotFound) it writes a refusal
// to the caller's session and returns (Zone{}, false, nil) so the
// caller can short-circuit without bubbling a user-facing error to
// the dispatcher. Only system-level errors (driver failures) return a
// non-nil err.
func resolveZone(c *telnet.Context, zones repo.ZoneRepo, zoneArg string) (repo.Zone, bool, error) {
	var (
		z   repo.Zone
		err error
	)
	if id, perr := strconv.ParseInt(zoneArg, 10, 64); perr == nil && id > 0 {
		z, err = zones.GetByID(c.Ctx, id)
	} else {
		z, err = zones.GetByExternalID(c.Ctx, zoneArg)
	}
	if errors.Is(err, repo.ErrZoneNotFound) {
		_ = c.Session.WriteString("{{No such zone:}}::red " + defangCfmt(zoneArg) + "\r\n")
		return repo.Zone{}, false, nil
	}
	if err != nil {
		_ = c.Session.WriteString("{{Could not look up that zone right now.}}::red\r\n")
		return repo.Zone{}, false, err
	}
	return z, true, nil
}

func characterNameOrUnknown(c *telnet.Context, characters repo.CharacterRepo, id int64) string {
	ch, err := characters.GetByID(c.Ctx, id)
	if err != nil {
		return fmt.Sprintf("(deleted character #%d)", id)
	}
	return ch.Name
}
