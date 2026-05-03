package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewInventory builds the `inventory` command. Renders held items
// (ordered by Character.Inventory JSON for stable display), the carry
// weight + load band, and the coin purse.
func NewInventory(items repo.ItemRepo, characters repo.CharacterRepo) *telnet.Command {
	return &telnet.Command{
		Name:    "inventory",
		Aliases: []string{"i", "inv"},
		Help:    "Show what you are carrying",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("inventory: character lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You feel disoriented and cannot focus on yourself.}}::red\r\n")
			}
			held, err := items.ListInInventory(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("inventory: list failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You feel disoriented and cannot focus on yourself.}}::red\r\n")
			}
			byID := make(map[int64]repo.Item, len(held))
			for _, it := range held {
				byID[it.ID] = it
			}
			ordered := orderInventory(char.Inventory, held, byID)

			var b strings.Builder
			b.WriteString("{{You are carrying:}}::green|bold\r\n")
			if len(ordered) == 0 {
				b.WriteString("  {{(nothing)}}::gray\r\n")
			} else {
				for _, it := range ordered {
					b.WriteString("  {{")
					b.WriteString(it.Name)
					b.WriteString("}}::green\r\n")
				}
			}

			carried := totalWeight(ordered)
			str := int(char.Core.Abilities.Str.Current)
			if str <= 0 {
				str = 10 // safe default until char-create stamps abilities
			}
			load, heavy := LoadFor(str, carried)
			b.WriteString(fmt.Sprintf("{{Carrying:}}::yellow|bold {{%g lbs / %g lbs}}::white {{(%s)}}::gray\r\n",
				carried, heavy, loadName(load)))

			b.WriteString("{{Coin:}}::yellow|bold ")
			if char.Coin == 0 {
				b.WriteString("{{(empty purse)}}::gray\r\n")
			} else {
				b.WriteString("{{")
				b.WriteString(char.Coin.Format())
				b.WriteString("}}::white\r\n")
			}
			return s.WriteString(b.String())
		},
	}
}

// NewGet builds the `get <item>` command. Picks an item up off the
// floor of the actor's room, blocks for NoTake / weight cap, then
// flips ownership and persists.
func NewGet(items repo.ItemRepo, characters repo.CharacterRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:    "get",
		Aliases: []string{"take"},
		Help:    "Get <item> — pick something up off the floor",
		MinArgs: 1,
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CurrentRoomID == 0 {
				return s.WriteString("{{There is nothing here to take.}}::yellow\r\n")
			}
			target := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
			floor, err := items.ListInRoom(c.Ctx, s.CurrentRoomID)
			if err != nil {
				slog.Error("get: list room failed", "room", s.CurrentRoomID, "error", err)
				return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
			}
			it, ok := MatchItem(target, floor)
			if !ok {
				return s.WriteString("{{You don't see that here.}}::yellow\r\n")
			}
			if it.HasFlag(repo.FlagNoTake) {
				return s.WriteString("{{You can't take that.}}::yellow\r\n")
			}

			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("get: char lookup failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You feel disoriented.}}::red\r\n")
			}
			held, err := items.ListInInventory(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("get: list inv failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
			}
			str := int(char.Core.Abilities.Str.Current)
			if str <= 0 {
				str = 10
			}
			carried := totalWeight(held)
			if load, _ := LoadFor(str, carried+it.Weight); load == creature.LoadOverloaded {
				return s.WriteString("{{It's too heavy — you can't carry that much.}}::yellow\r\n")
			}

			if err := items.TransferRoomToOwner(c.Ctx, it.ID, s.CurrentRoomID, s.CharacterID); err != nil {
				if errors.Is(err, repo.ErrItemMoved) || errors.Is(err, repo.ErrItemNotFound) {
					return s.WriteString("{{Someone else got there first.}}::yellow\r\n")
				}
				slog.Warn("get: transfer failed", "item", it.ID, "char", s.CharacterID, "error", err)
				return s.WriteString("{{It slips from your grasp.}}::red\r\n")
			}
			newInv := appendOnce(char.Inventory, it.ID)
			if err := characters.RecordInventory(c.Ctx, s.CharacterID, newInv); err != nil {
				slog.Warn("get: record inventory failed", "char", s.CharacterID, "error", err)
				// Item already moved; the next `inventory` will still
				// show it via the SQL truth even if the JSON ordering
				// is stale.
			}

			actor := safeActor(s)
			broadcastRoom(sessions, s.CurrentRoomID, s,
				"{{"+actor+" picks up "+it.Name+".}}::cyan\r\n")
			return s.WriteString("{{You pick up " + it.Name + ".}}::cyan\r\n")
		},
	}
}

// NewDrop builds the `drop <item>` command. Mirror of `get`.
func NewDrop(items repo.ItemRepo, characters repo.CharacterRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:    "drop",
		Help:    "Drop <item> — set something down",
		MinArgs: 1,
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CurrentRoomID == 0 {
				return s.WriteString("{{There is nowhere to drop it.}}::yellow\r\n")
			}
			target := strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " ")))
			held, err := items.ListInInventory(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("drop: list inv failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
			}
			it, ok := MatchItem(target, held)
			if !ok {
				return s.WriteString("{{You aren't carrying that.}}::yellow\r\n")
			}
			if it.HasFlag(repo.FlagNoDrop) {
				return s.WriteString("{{It is stuck to your hand and won't budge.}}::yellow\r\n")
			}

			if err := items.TransferOwnerToRoom(c.Ctx, it.ID, s.CharacterID, s.CurrentRoomID); err != nil {
				if errors.Is(err, repo.ErrItemMoved) || errors.Is(err, repo.ErrItemNotFound) {
					return s.WriteString("{{You aren't carrying that.}}::yellow\r\n")
				}
				slog.Warn("drop: transfer failed", "item", it.ID, "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't drop it right now.}}::red\r\n")
			}
			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Warn("drop: char lookup failed", "char", s.CharacterID, "error", err)
			} else {
				newInv := removeID(char.Inventory, it.ID)
				if err := characters.RecordInventory(c.Ctx, s.CharacterID, newInv); err != nil {
					slog.Warn("drop: record inventory failed", "char", s.CharacterID, "error", err)
				}
			}

			actor := safeActor(s)
			broadcastRoom(sessions, s.CurrentRoomID, s,
				"{{"+actor+" drops "+it.Name+".}}::cyan\r\n")
			return s.WriteString("{{You drop " + it.Name + ".}}::cyan\r\n")
		},
	}
}

// NewGive builds the `give <item|amount> <name>` command. The first
// arg is tried as a currency.Parse — on success it's a coin transfer;
// on parse error it falls through to item lookup. Both forms require
// the recipient to be online and in the same room.
func NewGive(items repo.ItemRepo, characters repo.CharacterRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name:    "give",
		Help:    "Give <item|amount> <name> — hand something to another player",
		MinArgs: 2,
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			recipient := c.Args[len(c.Args)-1]
			subject := strings.Join(c.Args[:len(c.Args)-1], " ")

			peer := sessions.FindByCharacterName(recipient)
			if peer == nil {
				return s.WriteString("{{There is no one by that name here.}}::yellow\r\n")
			}
			if peer == s {
				return s.WriteString("{{You hand it to yourself. Nothing happens.}}::yellow\r\n")
			}
			if peer.CurrentRoomID != s.CurrentRoomID || s.CurrentRoomID == 0 {
				return s.WriteString("{{They aren't here.}}::yellow\r\n")
			}

			// Try currency parse first. Bare-int "5" parses as 5cp,
			// which is a valid (if tiny) gift, so coin form takes
			// precedence over an item literally named "5".
			if amount, err := currency.Parse(subject); err == nil {
				return giveCoin(c, characters, s, peer, amount)
			}
			return giveItem(c, items, characters, sessions, s, peer, subject)
		},
	}
}

// giveCoin transfers the amount from actor to peer and persists both
// purses. ErrInsufficientFunds and overflow are surfaced cleanly.
func giveCoin(c *telnet.Context, characters repo.CharacterRepo, s, peer *telnet.Session, amount currency.Amount) error {
	if amount <= 0 {
		return s.WriteString("{{You can only give positive amounts.}}::yellow\r\n")
	}
	actor, err := characters.FindByName(c.Ctx, s.CharacterName)
	if err != nil {
		slog.Error("give: actor lookup failed", "char", s.CharacterID, "error", err)
		return s.WriteString("{{You can't seem to find your purse.}}::red\r\n")
	}
	target, err := characters.FindByName(c.Ctx, peer.CharacterName)
	if err != nil {
		slog.Error("give: target lookup failed", "char", peer.CharacterID, "error", err)
		return s.WriteString("{{They refuse the gift.}}::red\r\n")
	}
	newActor, err := actor.Coin.Sub(amount)
	if err != nil {
		if errors.Is(err, currency.ErrInsufficientFunds) {
			return s.WriteString("{{You don't have that much.}}::yellow\r\n")
		}
		return s.WriteString("{{Something went wrong with the coin.}}::red\r\n")
	}
	newTarget, err := target.Coin.Add(amount)
	if err != nil {
		return s.WriteString("{{They can't carry that much coin.}}::yellow\r\n")
	}
	if err := characters.RecordCoin(c.Ctx, actor.ID, newActor, actor.BankBalance); err != nil {
		slog.Warn("give: record actor coin failed", "char", actor.ID, "error", err)
		return s.WriteString("{{The coin slips from your fingers.}}::red\r\n")
	}
	if err := characters.RecordCoin(c.Ctx, target.ID, newTarget, target.BankBalance); err != nil {
		slog.Warn("give: record target coin failed", "char", target.ID, "error", err)
		// Compensating write must NOT use c.Ctx — if the player
		// disconnected mid-command it is already cancelled, and a
		// cancelled ctx would silently skip the rollback and leave the
		// actor permanently debited. Fresh background ctx with a tight
		// timeout keeps the rollback bounded but reachable. Errors are
		// loud — a swallowed rollback failure is real player coin lost.
		rbCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if rbErr := characters.RecordCoin(rbCtx, actor.ID, actor.Coin, actor.BankBalance); rbErr != nil {
			slog.Error("give: ROLLBACK FAILED — actor permanently debited",
				"char", actor.ID, "amount_cp", int64(amount),
				"original_error", err, "rollback_error", rbErr)
		}
		return s.WriteString("{{They drop the coin and you scoop it back up.}}::red\r\n")
	}

	short := amount.Format()
	_ = peer.WriteString("{{" + safeActor(s) + " hands you " + short + ".}}::cyan\r\n")
	return s.WriteString("{{You hand " + peer.CharacterName + " " + short + ".}}::cyan\r\n")
}

// giveItem transfers an item from actor to peer.
func giveItem(c *telnet.Context, items repo.ItemRepo, characters repo.CharacterRepo, sessions *session.Registry, s, peer *telnet.Session, subject string) error {
	target := strings.ToLower(strings.TrimSpace(subject))
	held, err := items.ListInInventory(c.Ctx, s.CharacterID)
	if err != nil {
		slog.Error("give: list inv failed", "char", s.CharacterID, "error", err)
		return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
	}
	it, ok := MatchItem(target, held)
	if !ok {
		return s.WriteString("{{You aren't carrying that.}}::yellow\r\n")
	}
	if it.HasFlag(repo.FlagNoDrop) {
		return s.WriteString("{{It is stuck to your hand and won't budge.}}::yellow\r\n")
	}

	// Recipient encumbrance gate.
	peerChar, err := characters.FindByName(c.Ctx, peer.CharacterName)
	if err != nil {
		slog.Error("give: peer lookup failed", "char", peer.CharacterID, "error", err)
		return s.WriteString("{{They refuse the gift.}}::red\r\n")
	}
	peerHeld, err := items.ListInInventory(c.Ctx, peer.CharacterID)
	if err != nil {
		slog.Error("give: peer inv failed", "char", peer.CharacterID, "error", err)
		return s.WriteString("{{They refuse the gift.}}::red\r\n")
	}
	pStr := int(peerChar.Core.Abilities.Str.Current)
	if pStr <= 0 {
		pStr = 10
	}
	if load, _ := LoadFor(pStr, totalWeight(peerHeld)+it.Weight); load == creature.LoadOverloaded {
		return s.WriteString("{{They couldn't carry that.}}::yellow\r\n")
	}

	if err := items.TransferOwnerToOwner(c.Ctx, it.ID, s.CharacterID, peer.CharacterID); err != nil {
		if errors.Is(err, repo.ErrItemMoved) || errors.Is(err, repo.ErrItemNotFound) {
			return s.WriteString("{{You aren't carrying that.}}::yellow\r\n")
		}
		slog.Warn("give: transfer failed", "item", it.ID, "to", peer.CharacterID, "error", err)
		return s.WriteString("{{It slips between you.}}::red\r\n")
	}

	// Splice both JSON ordering lists. Best-effort — SQL is the truth.
	if actor, err := characters.FindByName(c.Ctx, s.CharacterName); err == nil {
		_ = characters.RecordInventory(c.Ctx, actor.ID, removeID(actor.Inventory, it.ID))
	}
	_ = characters.RecordInventory(c.Ctx, peerChar.ID, appendOnce(peerChar.Inventory, it.ID))

	actorName := safeActor(s)
	_ = peer.WriteString("{{" + actorName + " gives you " + it.Name + ".}}::cyan\r\n")
	// Skip the room broadcast for actor and peer; both already got
	// targeted lines above. broadcastRoom only excludes one session,
	// so spell out the loop here to skip both.
	roomMsg := "{{" + actorName + " gives " + it.Name + " to " + peer.CharacterName + ".}}::cyan\r\n"
	for _, other := range sessions.Snapshot() {
		if other == s || other == peer || other.CurrentRoomID != s.CurrentRoomID {
			continue
		}
		if err := other.WriteString(roomMsg); err != nil {
			slog.Debug("give: peer broadcast failed", "to", other.CharacterName, "error", err)
		}
	}
	return s.WriteString("{{You give " + it.Name + " to " + peer.CharacterName + ".}}::cyan\r\n")
}

// orderInventory walks the JSON ordering and emits items in that order,
// skipping any id whose item is not actually owned (self-healing for
// stale JSON), then appends any items present in SQL but missing from
// the JSON (e.g. admin-spawned, or first run after the upgrade).
func orderInventory(jsonIDs []int64, all []repo.Item, byID map[int64]repo.Item) []repo.Item {
	out := make([]repo.Item, 0, len(all))
	seen := make(map[int64]bool, len(all))
	for _, id := range jsonIDs {
		if it, ok := byID[id]; ok && !seen[id] {
			out = append(out, it)
			seen[id] = true
		}
	}
	for _, it := range all {
		if !seen[it.ID] {
			out = append(out, it)
		}
	}
	return out
}

func appendOnce(ids []int64, id int64) []int64 {
	for _, x := range ids {
		if x == id {
			return ids
		}
	}
	return append(append([]int64(nil), ids...), id)
}

func removeID(ids []int64, id int64) []int64 {
	out := ids[:0:0]
	for _, x := range ids {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

func totalWeight(items []repo.Item) float64 {
	var w float64
	for _, it := range items {
		w += it.Weight
	}
	return w
}

