package main

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// msspWorldStats caches counts surfaced via MSSP. Captured once at boot
// from repo queries; the world is mostly static across a server run
// (admin `spawn` instantiates templates, doesn't add new ones), so a
// boot snapshot is accurate enough for crawler listings.
type msspWorldStats struct {
	areas   int
	rooms   int
	mobiles int
	objects int
}

// collectMSSPWorldStats snapshots world counts at boot. Errors degrade
// gracefully to zero — MSSP is best-effort metadata and a transient
// repo failure shouldn't tear down startup.
func collectMSSPWorldStats(ctx context.Context, zones repo.ZoneRepo, rooms repo.RoomRepo, mobTemplates repo.MobTemplateRepo, items repo.ItemRepo) msspWorldStats {
	var stats msspWorldStats
	if zList, err := zones.List(ctx); err == nil {
		stats.areas = len(zList)
	} else {
		slog.Warn("MSSP: zone count unavailable", "error", err)
	}
	if rList, err := rooms.ListAll(ctx); err == nil {
		stats.rooms = len(rList)
	} else {
		slog.Warn("MSSP: room count unavailable", "error", err)
	}
	if mList, err := mobTemplates.ListExternalIDs(ctx); err == nil {
		stats.mobiles = len(mList)
	} else {
		slog.Warn("MSSP: mob count unavailable", "error", err)
	}
	if iList, err := items.ListExternalIDs(ctx); err == nil {
		stats.objects = len(iList)
	} else {
		slog.Warn("MSSP: item count unavailable", "error", err)
	}
	return stats
}

// msspVars renders the MSSP variable block from live server state.
// Called on every inbound `DO MSSP` from a connected client; cheap
// enough that we don't bother caching (counts are pre-computed, session
// count is a map length, the rest is plain config/build strings).
func (srv *server) msspVars() []telnet.MSSPVar {
	uptime := strconv.FormatInt(srv.startedAt.Unix(), 10)
	players := strconv.Itoa(len(srv.sessions.Snapshot()))
	return []telnet.MSSPVar{
		// Required.
		{Name: "NAME", Value: "WheelMUD"},
		{Name: "PLAYERS", Value: players},
		{Name: "UPTIME", Value: uptime},

		// Generic.
		{Name: "CRAWL DELAY", Value: "-1"},
		{Name: "HOSTNAME", Value: srv.cfg.MSSP.Hostname},
		{Name: "LANGUAGE", Value: "English"},
		{Name: "LOCATION", Value: srv.cfg.MSSP.Location},
		{Name: "WEBSITE", Value: srv.cfg.MSSP.Website},
		{Name: "CONTACT", Value: srv.cfg.MSSP.Contact},

		// Categorisation.
		{Name: "FAMILY", Value: "Custom"},
		{Name: "GENRE", Value: "Fantasy"},
		{Name: "GAMEPLAY", Value: "Roleplaying"},
		{Name: "STATUS", Value: srv.cfg.MSSP.Status},
		{Name: "CODEBASE", Value: "WheelMUD"},
		{Name: "VERSION", Value: versionOr(buildVersion, "dev")},

		// World stats (boot snapshot).
		{Name: "AREAS", Value: strconv.Itoa(srv.worldStats.areas)},
		{Name: "ROOMS", Value: strconv.Itoa(srv.worldStats.rooms)},
		{Name: "MOBILES", Value: strconv.Itoa(srv.worldStats.mobiles)},
		{Name: "OBJECTS", Value: strconv.Itoa(srv.worldStats.objects)},
		// No help catalog repo today; advertise 0 with the option to
		// raise once the help system formalizes.
		{Name: "HELPFILES", Value: "0"},

		// Protocol capabilities. Flip these to "1" when each protocol
		// lands (ROADMAP §1).
		{Name: "ANSI", Value: "1"},
		{Name: "VT100", Value: "1"},
		{Name: "UTF-8", Value: "1"},
		{Name: "256 COLORS", Value: "1"},
		// Truecolor support exists in the renderer (cfmt +
		// ColorLevelTrueColor) but there's no server-side terminal
		// capability detection that confirms a given client supports
		// 24-bit SGR — Session.ColorLevel is set via the `colors`
		// verb or account settings, not negotiated. Advertise "0"
		// until TERM-driven detection lands; otherwise crawler
		// listings overstate the out-of-the-box experience.
		{Name: "TRUECOLOR", Value: "0"},
		{Name: "XTERM 256 COLORS", Value: "1"},
		{Name: "XTERM TRUE COLORS", Value: "0"},
		{Name: "MCCP", Value: "0"},
		{Name: "MCP", Value: "0"},
		{Name: "MSDP", Value: "0"},
		{Name: "MSP", Value: "0"},
		{Name: "GMCP", Value: "1"},
		{Name: "MXP", Value: "0"},
		{Name: "MNES", Value: "0"},
		{Name: "SSL", Value: "0"},
		{Name: "ZMP", Value: "0"},

		// Commercial / community flags — fixed at "0" for V1.
		{Name: "PAY TO PLAY", Value: "0"},
		{Name: "PAY FOR PERKS", Value: "0"},
		{Name: "HIRING BUILDERS", Value: "0"},
		{Name: "HIRING CODERS", Value: "0"},
	}
}
