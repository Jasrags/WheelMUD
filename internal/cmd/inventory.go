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
			all, err := items.ListAllOwnedTransitive(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("inventory: list failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You feel disoriented and cannot focus on yourself.}}::red\r\n")
			}
			byID := make(map[int64]repo.Item, len(all))
			topLevel := make([]repo.Item, 0, len(all))
			for _, it := range all {
				byID[it.ID] = it
				if it.ParentItemID == 0 {
					topLevel = append(topLevel, it)
				}
			}
			ordered := orderInventory(char.Inventory, topLevel, byID)
			idx := childrenOf(all)

			var b strings.Builder
			b.WriteString("{{You are carrying:}}::green|bold\r\n")
			if len(ordered) == 0 {
				b.WriteString("  {{(nothing)}}::gray\r\n")
			} else {
				for _, it := range ordered {
					renderInventoryNodeWithEquip(&b, it, idx, 1, char.Equipment)
				}
			}

			carried := totalCarriedWeight(ordered, idx)
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
		Long: "Usage: get <item>                    - pick from the floor\n" +
			"       get <item> from <container>   - take from a container\n\n" +
			"<container> may be in your inventory or on the floor.",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeRoomItems(items),
		Run: func(c *telnet.Context) error {
			s := c.Session
			if s.CurrentRoomID == 0 {
				return s.WriteString("{{There is nothing here to take.}}::yellow\r\n")
			}
			itemArg, containerArg, fromForm := splitFromArgs(c.Args)
			if fromForm {
				return getFromContainer(c, items, characters, sessions, itemArg, containerArg)
			}
			target := strings.ToLower(strings.TrimSpace(itemArg))
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
			carried, err := carriedWeight(c.Ctx, items, s.CharacterID)
			if err != nil {
				slog.Error("get: list inv failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
			}
			str := int(char.Core.Abilities.Str.Current)
			if str <= 0 {
				str = 10
			}
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
		Name:      "drop",
		Help:      "Drop <item> — set something down",
		MinArgs:   1,
		Auth:      telnet.AuthPlayer,
		Completer: completeInventoryItems(items),
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
			autoUnequipIfHeld(c, characters, it.ID)
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
		Name:      "give",
		Help:      "Give <item|amount> <name> — hand something to another player",
		MinArgs:   2,
		Auth:      telnet.AuthPlayer,
		Completer: completeGive(items, sessions),
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
	if err := characters.RecordCoin(c.Ctx, actor.ID, newActor, actor.BankBalance, actor.CoinVersion); err != nil {
		if errors.Is(err, repo.ErrCoinConflict) {
			return s.WriteString("{{Your purse just changed — try again.}}::yellow\r\n")
		}
		slog.Warn("give: record actor coin failed", "char", actor.ID, "error", err)
		return s.WriteString("{{The coin slips from your fingers.}}::red\r\n")
	}
	if err := characters.RecordCoin(c.Ctx, target.ID, newTarget, target.BankBalance, target.CoinVersion); err != nil {
		slog.Warn("give: record target coin failed", "char", target.ID, "error", err)
		// Compensating write must NOT use c.Ctx — if the player
		// disconnected mid-command it is already cancelled, and a
		// cancelled ctx would silently skip the rollback and leave the
		// actor permanently debited. Fresh background ctx with a tight
		// timeout keeps the rollback bounded but reachable. Errors are
		// loud — a swallowed rollback failure is real player coin lost.
		// Rollback uses actor.CoinVersion+1 because the first
		// RecordCoin succeeded and bumped the version.
		rbCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if rbErr := characters.RecordCoin(rbCtx, actor.ID, actor.Coin, actor.BankBalance, actor.CoinVersion+1); rbErr != nil {
			slog.Error("give: ROLLBACK FAILED — actor permanently debited",
				"char", actor.ID, "amount_cp", int64(amount),
				"original_error", err, "rollback_error", rbErr)
		}
		return s.WriteString("{{They drop the coin and you scoop it back up.}}::red\r\n")
	}

	short := amount.Format()
	_ = peer.WriteAsync("{{" + safeActor(s) + " hands you " + short + ".}}::cyan")
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
	autoUnequipIfHeld(c, characters, it.ID)

	// Splice both JSON ordering lists. Best-effort — SQL is the truth.
	if actor, err := characters.FindByName(c.Ctx, s.CharacterName); err == nil {
		_ = characters.RecordInventory(c.Ctx, actor.ID, removeID(actor.Inventory, it.ID))
	}
	_ = characters.RecordInventory(c.Ctx, peerChar.ID, appendOnce(peerChar.Inventory, it.ID))

	actorName := safeActor(s)
	_ = peer.WriteAsync("{{" + actorName + " gives you " + it.Name + ".}}::cyan")
	// Skip the room broadcast for actor and peer; both already got
	// targeted lines above. broadcastRoom only excludes one session,
	// so spell out the loop here to skip both.
	roomMsg := "{{" + actorName + " gives " + it.Name + " to " + peer.CharacterName + ".}}::cyan"
	for _, other := range sessions.Snapshot() {
		if other == s || other == peer || other.CurrentRoomID != s.CurrentRoomID {
			continue
		}
		if err := other.WriteAsync(roomMsg); err != nil {
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

// totalWeight sums the bare Weight field of a flat slice. Used by
// callers (currently only the give-recipient gate) that haven't
// fetched a transitive view of the carrier's inventory. Containers
// they're holding aren't recursed here — that's a separate slice of
// work for the give path; for now we err on the side of "let them
// receive it" because a give already passed the giver's checks.
func totalWeight(items []repo.Item) float64 {
	var w float64
	for _, it := range items {
		w += it.Weight
	}
	return w
}

// totalCarriedWeight sums the effective burden on a carrier given
// their top-level items and a nested-children index. Each top-level
// item contributes its recursiveWeight (which folds in container
// WeightMult). Use this anywhere encumbrance matters and a
// transitive item slice is in hand.
func totalCarriedWeight(topLevel []repo.Item, idx map[int64][]repo.Item) float64 {
	var w float64
	for _, it := range topLevel {
		w += recursiveWeight(it, idx)
	}
	return w
}

// renderInventoryNode writes one item plus its container contents
// (recursively) into b. depth is the indent level (1 == "  ", 2 ==
// "    ") so the inventory listing reads as a small tree.
func renderInventoryNode(b *strings.Builder, it repo.Item, idx map[int64][]repo.Item, depth int) {
	renderInventoryNodeWithEquip(b, it, idx, depth, creature.Equipment{})
}

// renderInventoryNodeWithEquip mirrors renderInventoryNode but appends
// a `(worn)` / `(wielded)` / `(offhand)` annotation when the item is
// currently in one of the single-occupancy slots on eq. Equipped
// items remain owned by the carrier (equipment_json is an overlay,
// not a relocation), so they still appear in the inventory tree.
func renderInventoryNodeWithEquip(b *strings.Builder, it repo.Item, idx map[int64][]repo.Item, depth int, eq creature.Equipment) {
	for i := 0; i < depth; i++ {
		b.WriteString("  ")
	}
	b.WriteString("{{")
	b.WriteString(it.Name)
	b.WriteString("}}::green")
	if slot, ok := eq.FindByItem(it.ID); ok {
		b.WriteString(" {{(")
		b.WriteString(slot.Label())
		b.WriteString(")}}::gray")
	}
	b.WriteString("\r\n")
	if it.Type != repo.ItemTypeContainer {
		return
	}
	children := idx[it.ID]
	if len(children) == 0 {
		for i := 0; i < depth+1; i++ {
			b.WriteString("  ")
		}
		b.WriteString("{{(empty)}}::gray\r\n")
		return
	}
	for _, child := range children {
		renderInventoryNodeWithEquip(b, child, idx, depth+1, eq)
	}
}

// NewPut builds the `put <item> in|into <container>` command. The
// container can be in the actor's inventory or on the room floor;
// the item must be in the actor's top-level inventory (move it out
// of one container before moving into another). Capacity, depth,
// liquid-only, and self/cycle checks live in canPut.
func NewPut(items repo.ItemRepo, characters repo.CharacterRepo, sessions *session.Registry) *telnet.Command {
	return &telnet.Command{
		Name: "put",
		Help: "Put <item> in <container> — stash it inside",
		Long: "Usage: put <item> in <container>\n" +
			"       put <item> into <container>\n\n" +
			"<container> may be in your inventory or on the floor.\n" +
			"Capacity, depth, and self/cycle checks apply.",
		MinArgs:   3,
		Auth:      telnet.AuthPlayer,
		Completer: completePut(items),
		Run: func(c *telnet.Context) error {
			s := c.Session
			itemArg, containerArg, ok := splitInArgs(c.Args)
			if !ok {
				return s.WriteString("{{Usage: put <item> in <container>}}::yellow\r\n")
			}
			itemTarget := strings.ToLower(strings.TrimSpace(itemArg))
			containerTarget := strings.ToLower(strings.TrimSpace(containerArg))

			held, err := items.ListInInventory(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("put: list inv failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
			}
			it, ok := MatchItem(itemTarget, held)
			if !ok {
				return s.WriteString("{{You aren't carrying that.}}::yellow\r\n")
			}

			// Container scope: top-level inventory first, then room
			// floor. Only top-level — putting into a nested-deeper
			// container would already need the player to fetch it
			// out by hand for the next put, so allowing it here
			// muddies the depth check semantics.
			container, found := MatchItem(containerTarget, held)
			containerOnFloor := false
			if !found {
				if s.CurrentRoomID != 0 {
					floor, err := items.ListInRoom(c.Ctx, s.CurrentRoomID)
					if err != nil {
						slog.Error("put: list room failed", "room", s.CurrentRoomID, "error", err)
						return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
					}
					container, found = MatchItem(containerTarget, floor)
					containerOnFloor = found
				}
			}
			if !found {
				return s.WriteString("{{You don't see that container here.}}::yellow\r\n")
			}
			if it.ID == container.ID {
				return s.WriteString("{{You can't put it in itself.}}::yellow\r\n")
			}
			// NoDrop: if the destination container is on the floor,
			// putting the item in is effectively dropping it. Refuse.
			if containerOnFloor && it.HasFlag(repo.FlagNoDrop) {
				return s.WriteString("{{It is stuck to your hand and won't budge.}}::yellow\r\n")
			}

			// Build a transitive view so canPut sees the whole tree
			// the carrier is responsible for. If the container is on
			// the floor, also fetch its contents so depth + capacity
			// reflect real state.
			allKnown, err := items.ListAllOwnedTransitive(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("put: list owned failed", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
			}
			if containerOnFloor {
				floorContainerView, err := transitiveContainerView(c.Ctx, items, container.ID)
				if err != nil {
					slog.Error("put: list container failed", "container", container.ID, "error", err)
					return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
				}
				allKnown = append(allKnown, floorContainerView...)
				// container itself isn't in either slice yet — need it
				// in byID for depthOf.
				allKnown = append(allKnown, container)
			}
			byID := make(map[int64]repo.Item, len(allKnown))
			for _, x := range allKnown {
				byID[x.ID] = x
			}
			switch canPut(container, it, allKnown, byID) {
			case putOK:
				// fall through
			case putNotAContainer:
				return s.WriteString("{{You can't put things in that.}}::yellow\r\n")
			case putNoStats:
				return s.WriteString("{{It isn't shaped to hold anything.}}::yellow\r\n")
			case putLiquidContainer:
				return s.WriteString("{{It's not the right shape for that.}}::yellow\r\n")
			case putSelf:
				return s.WriteString("{{You can't put it in itself.}}::yellow\r\n")
			case putCycle:
				return s.WriteString("{{That would loop back on itself.}}::yellow\r\n")
			case putTooDeep:
				return s.WriteString("{{It's already nested too deeply.}}::yellow\r\n")
			case putTooHeavy:
				return s.WriteString("{{It can't hold any more.}}::yellow\r\n")
			}

			if err := items.TransferOwnerToContainer(c.Ctx, it.ID, s.CharacterID, container.ID); err != nil {
				if errors.Is(err, repo.ErrItemMoved) || errors.Is(err, repo.ErrItemNotFound) {
					return s.WriteString("{{It's not where you thought it was.}}::yellow\r\n")
				}
				slog.Warn("put: transfer failed", "item", it.ID, "container", container.ID, "error", err)
				return s.WriteString("{{It slips from your grasp.}}::red\r\n")
			}
			autoUnequipIfHeld(c, characters, it.ID)

			// Maintain inventory_json ordering — the item is no longer
			// at top level. Best-effort.
			if char, err := characters.FindByName(c.Ctx, s.CharacterName); err == nil {
				_ = characters.RecordInventory(c.Ctx, s.CharacterID, removeID(char.Inventory, it.ID))
			}

			actor := safeActor(s)
			if s.CurrentRoomID != 0 {
				broadcastRoom(sessions, s.CurrentRoomID, s,
					"{{"+actor+" puts "+it.Name+" in "+container.Name+".}}::cyan\r\n")
			}
			return s.WriteString("{{You put " + it.Name + " in " + container.Name + ".}}::cyan\r\n")
		},
	}
}

// getFromContainer is the `get <item> from <container>` branch,
// reachable from NewGet when the args contain a "from" token.
func getFromContainer(c *telnet.Context, items repo.ItemRepo, characters repo.CharacterRepo, sessions *session.Registry, itemArg, containerArg string) error {
	s := c.Session
	itemTarget := strings.ToLower(strings.TrimSpace(itemArg))
	containerTarget := strings.ToLower(strings.TrimSpace(containerArg))

	held, err := items.ListInInventory(c.Ctx, s.CharacterID)
	if err != nil {
		slog.Error("get-from: list inv failed", "char", s.CharacterID, "error", err)
		return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
	}
	container, found := MatchItem(containerTarget, held)
	if !found && s.CurrentRoomID != 0 {
		floor, err := items.ListInRoom(c.Ctx, s.CurrentRoomID)
		if err != nil {
			slog.Error("get-from: list room failed", "room", s.CurrentRoomID, "error", err)
			return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
		}
		container, found = MatchItem(containerTarget, floor)
	}
	if !found {
		return s.WriteString("{{You don't see that container here.}}::yellow\r\n")
	}
	if container.Type != repo.ItemTypeContainer {
		return s.WriteString("{{That isn't a container.}}::yellow\r\n")
	}

	contents, err := items.ListInContainer(c.Ctx, container.ID)
	if err != nil {
		slog.Error("get-from: list container failed", "container", container.ID, "error", err)
		return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
	}
	it, ok := MatchItem(itemTarget, contents)
	if !ok {
		return s.WriteString("{{There is nothing like that inside.}}::yellow\r\n")
	}
	if it.HasFlag(repo.FlagNoTake) {
		return s.WriteString("{{You can't take that.}}::yellow\r\n")
	}

	char, err := characters.FindByName(c.Ctx, s.CharacterName)
	if err != nil {
		slog.Error("get-from: char lookup failed", "char", s.CharacterID, "error", err)
		return s.WriteString("{{You feel disoriented.}}::red\r\n")
	}
	carried, err := carriedWeight(c.Ctx, items, s.CharacterID)
	if err != nil {
		slog.Error("get-from: weight failed", "char", s.CharacterID, "error", err)
		return s.WriteString("{{You can't seem to focus on anything.}}::red\r\n")
	}
	str := int(char.Core.Abilities.Str.Current)
	if str <= 0 {
		str = 10
	}
	if load, _ := LoadFor(str, carried+it.Weight); load == creature.LoadOverloaded {
		return s.WriteString("{{It's too heavy — you can't carry that much.}}::yellow\r\n")
	}

	if err := items.TransferContainerToOwner(c.Ctx, it.ID, container.ID, s.CharacterID); err != nil {
		if errors.Is(err, repo.ErrItemMoved) || errors.Is(err, repo.ErrItemNotFound) {
			return s.WriteString("{{It isn't there anymore.}}::yellow\r\n")
		}
		slog.Warn("get-from: transfer failed", "item", it.ID, "char", s.CharacterID, "error", err)
		return s.WriteString("{{It slips from your grasp.}}::red\r\n")
	}
	newInv := appendOnce(char.Inventory, it.ID)
	if err := characters.RecordInventory(c.Ctx, s.CharacterID, newInv); err != nil {
		slog.Warn("get-from: record inventory failed", "char", s.CharacterID, "error", err)
	}

	actor := safeActor(s)
	if s.CurrentRoomID != 0 {
		broadcastRoom(sessions, s.CurrentRoomID, s,
			"{{"+actor+" takes "+it.Name+" from "+container.Name+".}}::cyan\r\n")
	}
	return s.WriteString("{{You take " + it.Name + " from " + container.Name + ".}}::cyan\r\n")
}

// splitFromArgs walks args looking for a literal "from" token. If
// found, returns (itemPart, containerPart, true). The item part is
// every token before "from" joined by spaces; the container part is
// every token after.
func splitFromArgs(args []string) (item, container string, ok bool) {
	for i, tok := range args {
		if strings.EqualFold(tok, "from") && i > 0 && i < len(args)-1 {
			return strings.Join(args[:i], " "), strings.Join(args[i+1:], " "), true
		}
	}
	return strings.Join(args, " "), "", false
}

// splitInArgs is the put-side equivalent: looks for "in" or "into".
func splitInArgs(args []string) (item, container string, ok bool) {
	for i, tok := range args {
		if (strings.EqualFold(tok, "in") || strings.EqualFold(tok, "into")) && i > 0 && i < len(args)-1 {
			return strings.Join(args[:i], " "), strings.Join(args[i+1:], " "), true
		}
	}
	return "", "", false
}

// carriedWeight returns the carrier's total effective burden via the
// transitive owned slice + recursiveWeight. Container WeightMult is
// honored. Used by every encumbrance gate so a bag-of-holding eases
// the carrier's load consistently.
func carriedWeight(ctx context.Context, items repo.ItemRepo, ownerID int64) (float64, error) {
	all, err := items.ListAllOwnedTransitive(ctx, ownerID)
	if err != nil {
		return 0, err
	}
	idx := childrenOf(all)
	var w float64
	for _, it := range all {
		if it.ParentItemID == 0 {
			w += recursiveWeight(it, idx)
		}
	}
	return w, nil
}

// transitiveContainerView returns the container's contents plus
// every nested descendant. Used by `put` when the destination is a
// floor container so depth/capacity see the full subtree without
// needing carrier ownership. We don't have a single repo method
// that walks an arbitrary item subtree, so do it iteratively here:
// fetch direct children, then their children, until the frontier is
// empty.
func transitiveContainerView(ctx context.Context, items repo.ItemRepo, parentID int64) ([]repo.Item, error) {
	// canPut + isAncestor block cycles at write time, but the DB has
	// no CHECK constraint enforcing acyclicity, so a stray admin
	// SQL or a future bypass could leave a loop in the data. Guard
	// the BFS with a visited set so a corrupt row can't stall the
	// dispatcher goroutine on an unbounded frontier expansion.
	var all []repo.Item
	seen := map[int64]bool{parentID: true}
	frontier := []int64{parentID}
	for len(frontier) > 0 {
		var next []int64
		for _, id := range frontier {
			kids, err := items.ListInContainer(ctx, id)
			if err != nil {
				return nil, err
			}
			for _, k := range kids {
				if seen[k.ID] {
					continue
				}
				seen[k.ID] = true
				all = append(all, k)
				next = append(next, k.ID)
			}
		}
		frontier = next
	}
	return all, nil
}

// completeRoomItems suggests item keywords from the floor of the
// actor's current room. Used by `get`. Returns nil past slot 0 so
// "get sword from chest" 3-arg forms (when containers land) don't
// surprise players with a stale completer.
func completeRoomItems(items repo.ItemRepo) func(s *telnet.Session, args string) []telnet.Candidate {
	return func(s *telnet.Session, args string) []telnet.Candidate {
		slot, partial := completerSlot(args)
		if slot != 0 || s.CurrentRoomID == 0 {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		floor, err := items.ListInRoom(ctx, s.CurrentRoomID)
		if err != nil {
			return nil
		}
		return itemKeywordCandidates(floor, partial)
	}
}

// completeInventoryItems suggests item keywords from the actor's
// inventory. Used by `drop`.
func completeInventoryItems(items repo.ItemRepo) func(s *telnet.Session, args string) []telnet.Candidate {
	return func(s *telnet.Session, args string) []telnet.Candidate {
		slot, partial := completerSlot(args)
		if slot != 0 || s.CharacterID == 0 {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		held, err := items.ListInInventory(ctx, s.CharacterID)
		if err != nil {
			return nil
		}
		return itemKeywordCandidates(held, partial)
	}
}

// completePut offers inventory item keywords on slot 0; on slot 1
// suggests "in"; on slot 2 suggests inventory + room containers.
func completePut(items repo.ItemRepo) func(s *telnet.Session, args string) []telnet.Candidate {
	return func(s *telnet.Session, args string) []telnet.Candidate {
		slot, partial := completerSlot(args)
		if s.CharacterID == 0 {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		switch slot {
		case 0:
			held, err := items.ListInInventory(ctx, s.CharacterID)
			if err != nil {
				return nil
			}
			return itemKeywordCandidates(held, partial)
		case 1:
			out := []telnet.Candidate{{Text: "in"}, {Text: "into"}}
			return filterCandidates(out, partial)
		case 2:
			held, err := items.ListInInventory(ctx, s.CharacterID)
			if err != nil {
				return nil
			}
			held = onlyContainers(held)
			if s.CurrentRoomID != 0 {
				if floor, err := items.ListInRoom(ctx, s.CurrentRoomID); err == nil {
					held = append(held, onlyContainers(floor)...)
				}
			}
			return itemKeywordCandidates(held, partial)
		default:
			return nil
		}
	}
}

func onlyContainers(list []repo.Item) []repo.Item {
	out := list[:0:0]
	for _, it := range list {
		if it.Type == repo.ItemTypeContainer {
			out = append(out, it)
		}
	}
	return out
}

func filterCandidates(in []telnet.Candidate, partial string) []telnet.Candidate {
	if partial == "" {
		return in
	}
	p := strings.ToLower(partial)
	out := in[:0:0]
	for _, c := range in {
		if strings.HasPrefix(strings.ToLower(c.Text), p) {
			out = append(out, c)
		}
	}
	return out
}

// completeGive offers inventory item keywords on slot 0 and online
// character names on slot 1. The Run handler treats the LAST arg as
// the recipient (so multi-word item names work), but tab completion
// only sees a positional view of typing — a 1-token-then-name flow
// is by far the common case, so slot 1 = recipient is the right
// default. Players who quote multi-word items still complete on slot
// 0 because Tokenize counts a quoted blob as a single token.
func completeGive(items repo.ItemRepo, sessions *session.Registry) func(s *telnet.Session, args string) []telnet.Candidate {
	return func(s *telnet.Session, args string) []telnet.Candidate {
		slot, partial := completerSlot(args)
		switch slot {
		case 0:
			if s.CharacterID == 0 {
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			held, err := items.ListInInventory(ctx, s.CharacterID)
			if err != nil {
				return nil
			}
			return itemKeywordCandidates(held, partial)
		case 1:
			return onlineNameCandidates(s, sessions, partial)
		default:
			return nil
		}
	}
}
