package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewChannel builds a single channel command (e.g. `ooc`, `gossip`,
// `newbie`). Verb name and color come from the channels-table row;
// behavior is uniform:
//
//   - no args: toggle the caller's mute bit, persist via
//     characters.RecordChannelSettings, echo the new state.
//   - args: sanitize, broadcast `[<NAME>] <speaker>: <text>` to every
//     other authenticated session whose mute bit is off.
//
// Persistence is write-through on toggle so the setting survives a
// crash even if the periodic save bucket hasn't fired. Failure to
// persist is logged but does not block the toggle (the in-memory map
// stays authoritative for this session).
func NewChannel(ch repo.Channel, sessions *session.Registry, characters repo.CharacterRepo) *telnet.Command {
	name := strings.ToLower(ch.Name)
	color := ch.Color
	if color == "" {
		color = "cyan"
	}
	display := strings.ToUpper(ch.Name)
	return &telnet.Command{
		Name:    name,
		Help:    "Speak on the " + ch.Name + " channel; no args toggles it on/off",
		MinArgs: 0,
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			raw := strings.TrimSpace(c.Raw)
			if raw == "" {
				return toggleChannel(c, name, characters)
			}
			text, ok := sanitizeChat(raw)
			if !ok {
				return c.Session.WriteString("{{Say what?}}::yellow\r\n")
			}
			if c.Session.IsChannelMuted(name) {
				return c.Session.WriteString("{{The " + ch.Name + " channel is off — turn it on first.}}::yellow\r\n")
			}
			speaker := c.Session.CharacterName
			if speaker == "" {
				speaker = "Someone"
			}
			selfMsg := "{{[" + display + "] You: }}::" + color + "{{" + text + "}}::white\r\n"
			otherMsg := "{{[" + display + "] " + speaker + ": }}::" + color + "{{" + text + "}}::white\r\n"
			for _, peer := range sessions.Snapshot() {
				if peer == c.Session {
					continue
				}
				if peer.AuthLevel < telnet.AuthPlayer {
					continue
				}
				if peer.IsChannelMuted(name) {
					continue
				}
				if err := peer.WriteAsync(otherMsg); err != nil {
					_, peerName, _ := peer.InWorld()
					slog.Debug("channel: peer write failed", "channel", name, "to", peerName, "error", err)
				}
			}
			return c.Session.WriteString(selfMsg)
		},
	}
}

// toggleChannel flips the mute bit and persists the new map.
func toggleChannel(c *telnet.Context, name string, characters repo.CharacterRepo) error {
	muted := c.Session.ToggleChannelMuted(name)
	if c.Session.CharacterID != 0 {
		// Best-effort write-through. ctx may carry a deadline from the
		// dispatcher's read loop; fall back to a fresh background ctx
		// only if it's already done so the toggle still gets stored.
		ctx := c.Ctx
		if ctx == nil || ctx.Err() != nil {
			ctx = context.Background()
		}
		if err := characters.RecordChannelSettings(ctx, c.Session.CharacterID, c.Session.ChannelMutedSnapshot()); err != nil {
			slog.Warn("channel: RecordChannelSettings failed",
				"channel", name, "char", c.Session.CharacterID, "error", err)
		}
	}
	state := "on"
	if muted {
		state = "off"
	}
	return c.Session.WriteString("{{Channel " + name + " is now " + state + ".}}::yellow\r\n")
}

// NewChannelsList builds the `channels` overview command: lists every
// known channel with the caller's current on/off state.
//
// Rendering goes through internal/display so the section header
// matches the chargen review / score sheet. Channel names render in
// the channel's own color (so the visual key stays consistent with
// broadcast lines), and on/off status renders green/gray.
func NewChannelsList(channels []repo.Channel) *telnet.Command {
	// Defensive: copy-and-sort so ordering is stable regardless of
	// what the catalog source did.
	sorted := append([]repo.Channel(nil), channels...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return &telnet.Command{
		Name: "channels",
		Help: "List chat channels and your current on/off state",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			if err := display.SectionHeader(c.Session, "Channels"); err != nil {
				return err
			}
			var b strings.Builder
			for _, ch := range sorted {
				on := !c.Session.IsChannelMuted(strings.ToLower(ch.Name))
				b.WriteString(formatChannelRow(ch, on))
			}
			return c.Session.WriteString(b.String())
		},
	}
}

// formatChannelRow emits one row of the channels overview:
//
//	  <name> — on    (channel-color name + green status)
//	  <name> — off   (gray name + gray status)
//
// Names are defanged so a hostile catalog entry can't recolour
// downstream rows. Color falls back to "cyan" if the channel row
// left it blank, mirroring NewChannel.
func formatChannelRow(ch repo.Channel, on bool) string {
	color := ch.Color
	if color == "" {
		color = "cyan"
	}
	name := display.Defang(ch.Name, "(unnamed)")
	if !on {
		// Muted channels render quietly so the visual scan jumps to
		// the active ones first.
		return fmt.Sprintf("  {{%s}}::gray {{—}}::gray {{off}}::gray\r\n", name)
	}
	return fmt.Sprintf("  {{%s}}::%s {{—}}::gray {{on}}::green\r\n", name, color)
}
