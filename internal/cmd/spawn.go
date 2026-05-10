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
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

const (
	spawnCountMax = 20
	spawnKindMob  = "mob"
	spawnKindItem = "item"
	spawnKindCoin = "coin"
)

// NewSpawn builds the admin `spawn` command. Three forms:
//
//	spawn mob <external_id> [count]
//	spawn item <external_id> [count]
//	spawn coin <amount> [count]
//
// Mobs route through MobTemplateRepo.GetByExternalID + a fresh
// MobInstance per copy. Items have no separate template repo today —
// the YAML-seeded item with that external_id is read as a template,
// its taxonomy copied into a new row with a unique runtime
// external_id (the column has a UNIQUE index, so duplicates would be
// rejected). Stats pointers are deep-copied so two spawned bags
// don't share a *ContainerStats shell.
//
// `spawn coin` mirrors the mob-death gold-drop path
// (combat.spawnCoinPile): one ItemTypeTradeGood "a small pile of
// coins" per count, Value=parsed amount, FlagTradeGood, dropped in
// the admin's current room.
//
// AuthAdmin gated. Count defaults to 1, capped at spawnCountMax to
// keep a fat-finger spawn from flooding the room.
func NewSpawn(items repo.ItemRepo, mobTemplates repo.MobTemplateRepo, mobs repo.MobInstanceRepo, characters repo.CharacterRepo, sessions *session.Registry, audits repo.AdminAuditRepo) *telnet.Command {
	return &telnet.Command{
		Name: "spawn",
		Help: "Spawn a mob, item, or coin pile from a template",
		Long: "Usage: spawn mob <external_id> [count]\n" +
			"       spawn item <external_id> [count]\n" +
			"       spawn coin <amount> [count]\n\n" +
			"Count defaults to 1 and is capped at " + strconv.Itoa(spawnCountMax) + ".\n" +
			"Coin amounts use the currency syntax. Single-denomination forms\n" +
			"like 1gc or 100cp pass bare; multi-denomination amounts must be\n" +
			"quoted, e.g. spawn coin \"1gc 5sp\" 2.",
		Auth:      telnet.AuthAdmin,
		MinArgs:   2,
		Completer: completeSpawn(items, mobTemplates),
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CurrentRoomID == 0 {
				return s.WriteString("{{You must be in a room to spawn things.}}::yellow\r\n")
			}
			kind := strings.ToLower(c.Args[0])
			count, msg, ok := parseSpawnCount(c.Args)
			if !ok {
				return s.WriteString(msg)
			}

			switch kind {
			case spawnKindMob:
				return spawnMobs(c, mobTemplates, mobs, sessions, c.Args[1], count, audits)
			case spawnKindItem:
				return spawnItems(c, items, sessions, c.Args[1], count, audits)
			case spawnKindCoin:
				return spawnCoins(c, items, sessions, c.Args[1], count, audits)
			default:
				return s.WriteString("{{First argument must be 'mob', 'item', or 'coin'.}}::yellow\r\n")
			}
		},
	}
}

// parseSpawnCount reads c.Args[2] (optional). Defaults to 1; caps at
// spawnCountMax with an explicit clamp message so the admin notices.
// Anything non-numeric or <= 0 is a hard refusal.
func parseSpawnCount(args []string) (int, string, bool) {
	if len(args) < 3 {
		return 1, "", true
	}
	n, err := strconv.Atoi(args[2])
	if err != nil || n < 1 {
		return 0, "{{Count must be a positive integer.}}::yellow\r\n", false
	}
	if n > spawnCountMax {
		n = spawnCountMax
	}
	return n, "", true
}

// spawnMobs creates `count` MobInstances from the named template in
// the admin's current room. Stops on the first per-iteration error
// and reports how many landed before the failure so the admin knows
// the partial state.
func spawnMobs(c *telnet.Context, mobTemplates repo.MobTemplateRepo, mobs repo.MobInstanceRepo, sessions *session.Registry, ext string, count int, audits repo.AdminAuditRepo) error {
	s := c.Session
	tpl, err := mobTemplates.GetByExternalID(c.Ctx, ext)
	if errors.Is(err, repo.ErrTemplateNotFound) {
		return s.WriteString("{{No mob template with that id.}}::yellow\r\n")
	}
	if err != nil {
		slog.Error("spawn: mob template lookup", "ext", ext, "error", err)
		return s.WriteString("{{Spawn failed.}}::red\r\n")
	}

	created := 0
	for i := 0; i < count; i++ {
		spawn := creature.NewInstanceFromTemplate(tpl, s.CurrentRoomID, 0)
		if _, err := mobs.Create(c.Ctx, spawn); err != nil {
			slog.Warn("spawn: mob_instance create", "ext", ext, "i", i, "error", err)
			return s.WriteString(fmt.Sprintf("{{Spawned %d of %d before error: %v}}::red\r\n", created, count, err))
		}
		created++
	}
	slog.Info("admin: spawn",
		"actor", s.CharacterID, "kind", "mob", "ext", ext,
		"count", created, "room", s.CurrentRoomID)
	if created > 0 {
		audit.Record(c.Ctx, audits, s, "spawn", ext,
			fmt.Sprintf("mob %s %d room=%d", ext, created, s.CurrentRoomID))
	}

	return announceSpawn(s, sessions, tpl.Core.Name, created)
}

// spawnItems uses the YAML-seeded item with the given external_id as
// a template: copy its typed fields, deep-copy Stats, and persist a
// fresh row with a unique external_id. The seed row stays put.
func spawnItems(c *telnet.Context, items repo.ItemRepo, sessions *session.Registry, ext string, count int, audits repo.AdminAuditRepo) error {
	s := c.Session
	template, err := items.FindByExternalID(c.Ctx, ext)
	if errors.Is(err, repo.ErrItemNotFound) {
		return s.WriteString("{{No item template with that id.}}::yellow\r\n")
	}
	if err != nil {
		slog.Error("spawn: item template lookup", "ext", ext, "error", err)
		return s.WriteString("{{Spawn failed.}}::red\r\n")
	}

	created := 0
	now := time.Now().UnixNano()
	for i := 0; i < count; i++ {
		copy := repo.Item{
			ExternalID: fmt.Sprintf("%s#sp-%d-%d", ext, now, i),
			Name:       template.Name,
			NameLower:  template.NameLower,
			ShortDesc:  template.ShortDesc,
			RoomID:     s.CurrentRoomID,
			Type:       template.Type,
			Weight:     template.Weight,
			Value:      template.Value,
			Quality:    template.Quality,
			Flags:      template.Flags,
			Stats:      repo.CloneItemStats(template.Stats),
		}
		if _, err := items.Create(c.Ctx, copy); err != nil {
			slog.Warn("spawn: item create", "ext", ext, "i", i, "error", err)
			return s.WriteString(fmt.Sprintf("{{Spawned %d of %d before error: %v}}::red\r\n", created, count, err))
		}
		created++
	}
	slog.Info("admin: spawn",
		"actor", s.CharacterID, "kind", "item", "ext", ext,
		"count", created, "room", s.CurrentRoomID)
	if created > 0 {
		audit.Record(c.Ctx, audits, s, "spawn", ext,
			fmt.Sprintf("item %s %d room=%d", ext, created, s.CurrentRoomID))
	}

	return announceSpawn(s, sessions, template.Name, created)
}

// spawnCoins drops `count` coin-pile TradeGood items in the admin's
// current room, each with Value parsed from the amount string. Mirrors
// combat.spawnCoinPile so the runtime shape matches what mob death
// already produces. Negative or zero amounts are refused (currency.Parse
// rejects negatives, and a zero-cp pile has no QA value).
func spawnCoins(c *telnet.Context, items repo.ItemRepo, sessions *session.Registry, amountArg string, count int, audits repo.AdminAuditRepo) error {
	s := c.Session
	amount, err := currency.Parse(amountArg)
	if err != nil {
		return s.WriteString(fmt.Sprintf("{{Bad coin amount %q: %v}}::yellow\r\n", amountArg, err))
	}
	if amount <= 0 {
		return s.WriteString("{{Coin amount must be positive.}}::yellow\r\n")
	}

	created := 0
	now := time.Now().UnixNano()
	for i := 0; i < count; i++ {
		pile := repo.Item{
			ExternalID: fmt.Sprintf("coin-pile-spawn-%d-%d", now, i),
			Name:       "a small pile of coins",
			ShortDesc:  "A small pile of coins lies here.",
			RoomID:     s.CurrentRoomID,
			Type:       repo.ItemTypeTradeGood,
			Value:      amount,
			Flags:      repo.FlagTradeGood,
		}
		if _, err := items.Create(c.Ctx, pile); err != nil {
			slog.Warn("spawn: coin pile create", "amount", amount, "i", i, "error", err)
			return s.WriteString(fmt.Sprintf("{{Spawned %d of %d before error: %v}}::red\r\n", created, count, err))
		}
		created++
	}
	slog.Info("admin: spawn",
		"actor", s.CharacterID, "kind", "coin", "amount", amount.Format(),
		"count", created, "room", s.CurrentRoomID)
	if created > 0 {
		audit.Record(c.Ctx, audits, s, "spawn", amount.Format(),
			fmt.Sprintf("coin %s %d room=%d", amount.Format(), created, s.CurrentRoomID))
	}

	return announceSpawn(s, sessions, fmt.Sprintf("a small pile of coins (%s)", amount.Format()), created)
}

// announceSpawn echoes to the admin and broadcasts the room. The
// admin echo is the returned error path; the room broadcast goes
// through WriteAsync inside broadcastRoom and any per-peer failure
// is logged at that layer rather than surfaced to the actor.
func announceSpawn(s *telnet.Session, sessions *session.Registry, name string, count int) error {
	tag := name
	subject := name
	if count > 1 {
		tag = fmt.Sprintf("%d × %s", count, name)
		subject = fmt.Sprintf("%d copies of %s", count, name)
	}
	actor := safeActor(s)
	broadcastRoom(sessions, s.CurrentRoomID, s,
		fmt.Sprintf("{{%s gestures and summons %s.}}::cyan\r\n", actor, subject))
	return s.WriteString(fmt.Sprintf("{{Spawned %s here.}}::cyan\r\n", tag))
}

// completeSpawn offers `mob` / `item` on slot 0; on slot 1 routes to
// the matching ListExternalIDs (mob templates or item rows). Slot 2
// (count) is numeric and not completed.
func completeSpawn(items repo.ItemRepo, mobTemplates repo.MobTemplateRepo) func(s *telnet.Session, args string) []telnet.Candidate {
	return func(s *telnet.Session, args string) []telnet.Candidate {
		slot, partial := completerSlot(args)
		if slot < 0 {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		switch slot {
		case 0:
			return prefixCandidates([]string{spawnKindMob, spawnKindItem, spawnKindCoin}, partial)
		case 1:
			kind := strings.ToLower(firstToken(args))
			switch kind {
			case spawnKindMob:
				ids, err := mobTemplates.ListExternalIDs(ctx)
				if err != nil {
					return nil
				}
				return prefixCandidates(ids, partial)
			case spawnKindItem:
				ids, err := items.ListExternalIDs(ctx)
				if err != nil {
					return nil
				}
				return prefixCandidates(ids, partial)
			case spawnKindCoin:
				return nil
			}
		}
		return nil
	}
}

// prefixCandidates filters values by case-insensitive prefix match
// against partial and wraps each into a Candidate.
func prefixCandidates(values []string, partial string) []telnet.Candidate {
	p := strings.ToLower(partial)
	out := make([]telnet.Candidate, 0, len(values))
	for _, v := range values {
		if !strings.HasPrefix(strings.ToLower(v), p) {
			continue
		}
		out = append(out, telnet.Candidate{Text: v})
	}
	return out
}

// firstToken returns the first whitespace-separated token of s
// (trimmed). Used to peek at slot-0 of the args string from inside a
// later-slot completer.
func firstToken(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}
