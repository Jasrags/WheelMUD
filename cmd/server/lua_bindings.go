package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Jasrags/WheelMUD/internal/affects"
	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/cmd"
	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/effects"
	luaeng "github.com/Jasrags/WheelMUD/internal/lua"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/tick"
	"github.com/Jasrags/WheelMUD/internal/trigger"

	luastd "github.com/yuin/gopher-lua"
)

// makeLuaApplyAffect builds the Phase F #32 slice 3 apply_affect
// closure. Resolves effectID through the catalog, builds a
// creature.Affect via Effect.ToAffect (sentinel Source =
// cmd.LuaAffectSource → "script" label in the inspect verb), and
// persists via affects.Apply + RecordAffects.
//
// Slice 4: durationOverride > 0 overrides the catalog's authored
// DurationTicks; 0 means "use catalog default".
func makeLuaApplyAffect(characters repo.CharacterRepo, eff *effects.Catalog) func(context.Context, int64, string, int32) error {
	return func(ctx context.Context, targetID int64, effectID string, durationOverride int32) error {
		e, ok := eff.Get(effectID)
		if !ok {
			return fmt.Errorf("unknown effect %q", effectID)
		}
		ch, err := characters.GetByID(ctx, targetID)
		if err != nil {
			return err
		}
		affect := e.ToAffect(cmd.LuaAffectSource)
		if durationOverride > 0 {
			affect.DurationTicks = durationOverride
		}
		next := affects.Apply(ch.Core.Affects, affect)
		return characters.RecordAffects(ctx, targetID, next)
	}
}

// luaGiveItemSeq breaks ties when two give_item calls land in the
// same nanosecond on the same target — without it, the generated
// external_id collides on the items.external_id UNIQUE index and
// the second call trips the trigger fault budget. Process-global,
// monotonic, never reset.
var luaGiveItemSeq int64

// makeLuaGiveItem clones the YAML-seeded template at externalID and
// places the fresh row directly into the target's inventory. Mirrors
// the admin spawn path (internal/cmd/spawn.go::spawnItems) but skips
// admin auditing — Lua-driven spawns are content-author tools, not
// privileged operator actions.
func makeLuaGiveItem(items repo.ItemRepo) func(context.Context, int64, string) error {
	return func(ctx context.Context, targetID int64, externalID string) error {
		template, err := items.FindByExternalID(ctx, externalID)
		if err != nil {
			return err
		}
		seq := atomic.AddInt64(&luaGiveItemSeq, 1)
		spawn := repo.Item{
			ExternalID:       fmt.Sprintf("%s#lua-%d-%d-%d", externalID, time.Now().UnixNano(), targetID, seq),
			Name:             template.Name,
			NameLower:        template.NameLower,
			ShortDesc:        template.ShortDesc,
			OwnerCharacterID: targetID,
			Type:             template.Type,
			Weight:           template.Weight,
			Value:            template.Value,
			Quality:          template.Quality,
			Flags:            template.Flags,
			Stats:            repo.CloneItemStats(template.Stats),
		}
		_, err = items.Create(ctx, spawn)
		return err
	}
}

// makeLuaTargetHP returns a closure exposing a character's
// HPCurrent / HPMax via target.hp(id) in Lua scripts.
func makeLuaTargetHP(characters repo.CharacterRepo) func(context.Context, int64) (int32, int32, error) {
	return func(ctx context.Context, targetID int64) (int32, int32, error) {
		ch, err := characters.GetByID(ctx, targetID)
		if err != nil {
			return 0, 0, err
		}
		return ch.Core.HPCurrent, ch.Core.HPMax, nil
	}
}

// makeLuaTargetLevel sums ClassLevels into a single integer for
// target.level(id). Multiclassed characters return the sum of
// every class's level.
func makeLuaTargetLevel(characters repo.CharacterRepo) func(context.Context, int64) (int, error) {
	return func(ctx context.Context, targetID int64) (int, error) {
		ch, err := characters.GetByID(ctx, targetID)
		if err != nil {
			return 0, err
		}
		total := 0
		for _, lvl := range ch.ClassLevels {
			total += int(lvl)
		}
		return total, nil
	}
}

// makeLuaTargetClasses returns the multiclass map keyed by the
// chargen catalog's canonical class id (e.g. "armsman", "initiate").
// Phase F #32 slice 4 — companion to target.level which sums into
// a single int. Empty map for a character with no class levels
// (defensive — chargen always stamps at least one). Falls back to
// "class_<int>" when a catalog row is missing for a given enum
// value (shouldn't happen in practice; defensive only).
func makeLuaTargetClasses(characters repo.CharacterRepo, cat *chargen.Catalog) func(context.Context, int64) (map[string]int, error) {
	enumToID := make(map[creature.Class]string)
	for _, c := range cat.Classes() {
		enumToID[c.Enum] = c.ID
	}
	return func(ctx context.Context, targetID int64) (map[string]int, error) {
		ch, err := characters.GetByID(ctx, targetID)
		if err != nil {
			return nil, err
		}
		out := make(map[string]int, len(ch.ClassLevels))
		for cls, lvl := range ch.ClassLevels {
			id, ok := enumToID[cls]
			if !ok {
				id = fmt.Sprintf("class_%d", int(cls))
			}
			out[id] = int(lvl)
		}
		return out, nil
	}
}

// makeLuaRoomPlayers returns the bound character ids in roomID,
// sorted ascending. Phase F #32 slice 4 — feeds room.players() in
// Lua. Returns an empty slice (never nil) so the Lua-side ipairs
// always sees a valid table.
func makeLuaRoomPlayers(sessions *session.Registry) func(context.Context, int64) ([]int64, error) {
	return func(_ context.Context, roomID int64) ([]int64, error) {
		out := make([]int64, 0, 4)
		for charID, s := range sessions.Snapshot() {
			if s == nil {
				continue
			}
			_, _, sRoom := s.InWorld()
			if sRoom == roomID {
				out = append(out, charID)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out, nil
	}
}

// makeLuaRoomMobs returns mob_instance ids in roomID via
// MobInstanceRepo.ListInRoom. Phase F #32 slice 4.
func makeLuaRoomMobs(mobs repo.MobInstanceRepo) func(context.Context, int64) ([]int64, error) {
	return func(ctx context.Context, roomID int64) ([]int64, error) {
		list, err := mobs.ListInRoom(ctx, roomID)
		if err != nil {
			return nil, err
		}
		out := make([]int64, 0, len(list))
		for _, m := range list {
			out = append(out, m.ID)
		}
		return out, nil
	}
}

// defangScriptSource neutralizes cfmt brace tokens that a content
// author might have embedded in a Lua deal_damage source string so
// the default narration can render it as plain text without
// hijacking the surrounding {{...}} tag. Mirrors internal/cmd's
// unexported defangCfmt; kept package-private here so the
// dependency arrow doesn't reverse.
var defangScriptSourceReplacer = strings.NewReplacer("{{", "{ {", "}}", "} }")

func defangScriptSource(s string) string {
	return defangScriptSourceReplacer.Replace(s)
}

// makeLuaDealDamage resolves targetID as either a character or a mob
// (character repo first, then mob repo) and routes through
// combat.Manager.ApplyDamageExternal. Killer attribution is anonymous
// in V1 — a script firing from an `on_say` row could mean the speaker,
// the room, or "nobody", and the binding signature doesn't carry an
// authored choice. Future slices may thread a script-supplied killer
// hint through the source string. Phase F #32 slice 5a.
func makeLuaDealDamage(combatMgr *combat.Manager, characters repo.CharacterRepo, mobs repo.MobInstanceRepo) func(context.Context, int64, int32, string) error {
	return func(ctx context.Context, targetID int64, amount int32, source string) error {
		ref, err := resolveLuaTarget(ctx, targetID, characters, mobs)
		if err != nil {
			return err
		}
		return combatMgr.ApplyDamageExternal(ctx, combat.ActorRef{}, ref, amount, source)
	}
}

// makeLuaHeal mirrors makeLuaDealDamage's target resolution path,
// routing through combat.Manager.ApplyHealing.
func makeLuaHeal(combatMgr *combat.Manager, characters repo.CharacterRepo, mobs repo.MobInstanceRepo) func(context.Context, int64, int32) error {
	return func(ctx context.Context, targetID int64, amount int32) error {
		ref, err := resolveLuaTarget(ctx, targetID, characters, mobs)
		if err != nil {
			return err
		}
		return combatMgr.ApplyHealing(ctx, ref, amount)
	}
}

// resolveLuaTarget tries the character repo first, then the mob
// instance repo. Returns a classified error when neither matches so
// the trigger fault budget can catch malformed authoring (e.g. a
// script that references a character id long since deleted).
//
// A genuine DB error (anything that is NOT ErrCharacterNotFound /
// ErrInstanceNotFound) is surfaced immediately rather than falling
// through to the next repo — otherwise a transient infra failure
// reads identically to "no such id", masking real problems behind a
// misleading "not found" Lua error.
func resolveLuaTarget(ctx context.Context, targetID int64, characters repo.CharacterRepo, mobs repo.MobInstanceRepo) (combat.ActorRef, error) {
	if targetID == 0 {
		return combat.ActorRef{}, fmt.Errorf("target id must be non-zero")
	}
	if characters != nil {
		_, err := characters.GetByID(ctx, targetID)
		switch {
		case err == nil:
			return combat.ActorRef{Kind: combat.ActorKindCharacter, ID: targetID}, nil
		case errors.Is(err, repo.ErrCharacterNotFound):
			// expected miss — fall through to the mob lookup
		default:
			return combat.ActorRef{}, fmt.Errorf("character lookup failed: %w", err)
		}
	}
	if mobs != nil {
		_, err := mobs.GetByID(ctx, targetID)
		switch {
		case err == nil:
			return combat.ActorRef{Kind: combat.ActorKindMob, ID: targetID}, nil
		case errors.Is(err, repo.ErrInstanceNotFound):
			// fall through to the "no match" return
		default:
			return combat.ActorRef{}, fmt.Errorf("mob lookup failed: %w", err)
		}
	}
	return combat.ActorRef{}, fmt.Errorf("no character or mob with id %d", targetID)
}

// makeLuaTransferItem moves itemID between two characters' inventories
// via the repo's optimistic-lock-aware TransferOwnerToOwner. Resolves
// the current owner via GetByID — the item must currently be in some
// character's inventory; items on the room floor or inside a
// container are refused (the trigger fault budget catches the
// classified error). Mirrors the encumbrance-free V1 give-verb path:
// inventory limits via the carry table are deferred (slice 5a
// followups). Phase F #32 slice 5a.
func makeLuaTransferItem(items repo.ItemRepo) func(context.Context, int64, int64) error {
	return func(ctx context.Context, itemID, toOwnerID int64) error {
		if itemID == 0 || toOwnerID == 0 {
			return fmt.Errorf("item id and target owner must be non-zero")
		}
		it, err := items.GetByID(ctx, itemID)
		if err != nil {
			return err
		}
		if it.OwnerCharacterID == 0 {
			return fmt.Errorf("item %d is not in a character's inventory (room=%d parent=%d)", itemID, it.RoomID, it.ParentItemID)
		}
		if it.OwnerCharacterID == toOwnerID {
			// Idempotent no-op — script handing item back to its
			// existing owner is harmless; surface no error.
			return nil
		}
		return items.TransferOwnerToOwner(ctx, itemID, it.OwnerCharacterID, toOwnerID)
	}
}

// makeLuaDropItem drops an owned item into the firing room. The room
// is supplied by the trigger-layer adapter (ev.RoomID); the dialogue
// hook closure passes s.CurrentRoomID. Mirrors makeLuaTransferItem's
// "must currently be in a character's inventory" guard. Phase F #32
// slice 5a.
func makeLuaDropItem(items repo.ItemRepo) func(context.Context, int64, int64) error {
	return func(ctx context.Context, itemID, currentRoomID int64) error {
		if itemID == 0 || currentRoomID == 0 {
			return fmt.Errorf("item id and target room must be non-zero")
		}
		it, err := items.GetByID(ctx, itemID)
		if err != nil {
			return err
		}
		if it.OwnerCharacterID == 0 {
			return fmt.Errorf("item %d is not in a character's inventory (room=%d parent=%d)", itemID, it.RoomID, it.ParentItemID)
		}
		return items.TransferOwnerToRoom(ctx, itemID, it.OwnerCharacterID, currentRoomID)
	}
}

// Phase F #32 slice 5b factories.

// MinWaitSeconds / MaxWaitSeconds bound the wait() binding's delay
// arg. Sub-1 second is rejected because authors who want "next
// tick" can use the existing on_tick surface; > 300s (5 min) is
// almost always an author mistake. Tune up later if a content
// author hits the cap legitimately.
const (
	MinWaitSeconds int32 = 1
	MaxWaitSeconds int32 = 300
)

// MinWaitMilliseconds / MaxWaitMilliseconds bound the wait_ms()
// binding's delay arg (Phase F #32 slice 5c). Sub-100ms is rejected
// because the scheduler's tick precision rounds finer values up to
// the next pulse anyway; > 5 min mirrors MaxWaitSeconds.
//
// Note: actual firing precision is bounded by the scheduler
// frequency (1 Hz today). Finer-grained scheduler buckets are a
// separate followup.
const (
	MinWaitMilliseconds int32 = 100
	MaxWaitMilliseconds int32 = 300_000
)

// MaxOutstandingWaits caps the global count of scheduled-but-not-
// yet-fired wait() / wait_ms() deferred runs. Defense in depth: an
// abusive content author scripting a tight loop of wait calls in a
// single script run would otherwise schedule arbitrarily many fires,
// each holding a runner.Run goroutine when it lands. The LState
// pool of 8 provides natural execution backpressure but does not
// gate scheduling itself.
//
// Cap is global (not per-trigger or per-script) — simplest shape
// that catches the "schedule N waits in a row" pattern. Refusal is
// surfaced as a classified Lua error so the trigger fault budget
// increments and the offending trigger auto-disables after 5
// refusals.
const MaxOutstandingWaits int32 = 64

// outstandingWaits is the global counter behind MaxOutstandingWaits.
// Acquired (incremented) before tick.AfterCtx is called and released
// (decremented) at the start of the fire closure so a deferred
// script can re-schedule a fresh wait inside its body without
// double-counting.
var outstandingWaits atomic.Int32

// buildWaitCtxView translates a trigger.EventCtx into the
// internal/lua CtxView the deferred script sees. Mirrors
// ctxViewFromEvent in internal/trigger/actions_lua.go but lives
// here because the cmd-layer is the only consumer with this
// translation need (the trigger layer's own ctx propagation goes
// through ctxViewFromEvent at bind time). Phase F #32 slice 5b.
func buildWaitCtxView(ev trigger.EventCtx) luaeng.CtxView {
	return luaeng.CtxView{
		Event:      string(ev.Event),
		RoomID:     ev.RoomID,
		ActorID:    ev.ActorID,
		ActorKind:  ev.ActorKind,
		TargetID:   ev.TargetID,
		TargetKind: ev.TargetKind,
		Text:       ev.Text,
		Bucket:     ev.BucketName,
	}
}

// resolveWaitShutdownCtx loads the late-bound shutdown ctx, falling
// back to context.Background() when the atomic pointer hasn't been
// populated yet (defensive — shouldn't happen post-main()).
// tick.AfterCtx tolerates a Background parent (degrades to s.After
// without auto-cancel), so the fallback is safe.
func resolveWaitShutdownCtx(p *atomic.Pointer[context.Context]) context.Context {
	if p == nil {
		return context.Background()
	}
	if c := p.Load(); c != nil && *c != nil {
		return *c
	}
	return context.Background()
}

// buildLuaWaitFactory is the shared implementation behind
// makeLuaWait (seconds) and makeLuaWaitMs (milliseconds). The two
// public factories differ only in their delay-arg unit + validation
// bounds; everything else (range check, script-name check,
// wait-slot acquire, ctx snapshot, shutdownCtx resolution, fire
// closure, AfterCtx scheduling) is identical and lives here.
//
// validateDelay returns an error message when the delay arg is out
// of range. toDuration converts the unit-typed delay arg to
// time.Duration. label is the wait/wait_ms prefix on the log line
// when the deferred script run fails — keeps the log filterable
// per binding kind.
//
// srvShutdownCtxPtr is an atomic pointer assigned by main() after
// signal.NotifyContext. The Store/Load barriers ensure every
// dispatch-goroutine read sees the boot-time write regardless of
// scheduler.Start ordering.
func buildLuaWaitFactory(
	scheduler *tick.Scheduler,
	runner *luaeng.Runner,
	srvShutdownCtxPtr *atomic.Pointer[context.Context],
	label string,
	validateDelay func(int32) error,
	toDuration func(int32) time.Duration,
) func(context.Context, trigger.EventCtx, int32, string) error {
	return func(_ context.Context, ev trigger.EventCtx, n int32, scriptName string) error {
		if err := validateDelay(n); err != nil {
			return err
		}
		if strings.TrimSpace(scriptName) == "" {
			return fmt.Errorf("%s script name must be non-empty", label)
		}
		if err := acquireWaitSlot(); err != nil {
			return err
		}
		ctxView := buildWaitCtxView(ev)
		shutdownCtx := resolveWaitShutdownCtx(srvShutdownCtxPtr)
		fire := func(_ context.Context) {
			// Release BEFORE the deferred Run so a chained
			// wait()/wait_ms() inside the fired script can
			// re-acquire a fresh slot; the cap targets "schedule
			// N waits in a tight loop" not "always-on retry".
			releaseWaitSlot()
			// The deferred Run inherits shutdownCtx (the
			// signal.NotifyContext) NOT the scheduler's per-pulse
			// ctx — shutdownCtx is the right parent for SIGINT/
			// SIGTERM cancellation; runner.Run wraps it with the
			// 50ms CallTimeout internally.
			//
			// Deferred scripts get NO mutation surface in V1 —
			// bindings stay minimal (ctx + logger). Authors who
			// need chained mutations fire a fresh trigger from
			// inside the deferred script, keeping the deferred
			// surface narrow.
			bindings := luaeng.APIBindings{Ctx: ctxView}
			if err := runner.Run(shutdownCtx, scriptName, func(l *luastd.LState) { bindings.Bind(l) }); err != nil {
				slog.Debug(label+": deferred script run failed",
					"script", scriptName, "error", err)
			}
		}
		tick.AfterCtx(scheduler, shutdownCtx, toDuration(n), fire)
		return nil
	}
}

// makeLuaWait returns the cmd-layer factory for the wait() Lua
// binding. See buildLuaWaitFactory for the shared mechanism.
func makeLuaWait(scheduler *tick.Scheduler, runner *luaeng.Runner, srvShutdownCtxPtr *atomic.Pointer[context.Context]) func(context.Context, trigger.EventCtx, int32, string) error {
	return buildLuaWaitFactory(scheduler, runner, srvShutdownCtxPtr, "wait",
		func(seconds int32) error {
			if seconds < MinWaitSeconds {
				return fmt.Errorf("wait seconds must be >= %d (got %d)", MinWaitSeconds, seconds)
			}
			if seconds > MaxWaitSeconds {
				return fmt.Errorf("wait seconds must be <= %d (got %d)", MaxWaitSeconds, seconds)
			}
			return nil
		},
		func(seconds int32) time.Duration { return time.Duration(seconds) * time.Second },
	)
}

// acquireWaitSlot increments the global outstanding-wait counter
// and refuses (decrementing back) when at MaxOutstandingWaits. The
// refusal surfaces as a classified Lua error so the trigger's
// fault budget bumps; persistent abuse auto-disables the trigger
// after 5 refusals.
//
// Note: the cap is soft under concurrent callers. Two goroutines
// racing the Add(1) can each observe a pre-increment value below
// the cap and both proceed past the gate; one will refuse, but the
// counter can briefly read MaxOutstandingWaits + (concurrency - 1)
// before the rollback lands. With the scheduler at 1 Hz and the
// LState pool of 8, real concurrency at this site is effectively
// zero — the soft-cap window is negligible. If a future
// finer-grained scheduler makes concurrent acquire common, swap to
// a CAS loop.
func acquireWaitSlot() error {
	if outstandingWaits.Add(1) > MaxOutstandingWaits {
		outstandingWaits.Add(-1)
		return fmt.Errorf("wait cap reached (max %d outstanding deferred runs)", MaxOutstandingWaits)
	}
	return nil
}

// releaseWaitSlot decrements the counter. Called at the start of
// the deferred fire closure so a chained wait inside the fired
// script can re-acquire.
func releaseWaitSlot() {
	outstandingWaits.Add(-1)
}

// makeLuaInventory returns the cmd-layer factory for the
// inventory(target_id) Lua binding. Wraps
// ItemRepo.ListInInventory (top-level items only — container
// contents excluded). Empty inventory returns an empty slice, not
// an error.
func makeLuaInventory(items repo.ItemRepo) func(context.Context, int64) ([]luaeng.InventoryEntry, error) {
	return func(ctx context.Context, targetID int64) ([]luaeng.InventoryEntry, error) {
		if targetID == 0 {
			return nil, fmt.Errorf("target id must be non-zero")
		}
		list, err := items.ListInInventory(ctx, targetID)
		if err != nil {
			return nil, err
		}
		return itemsToInventoryEntries(list), nil
	}
}

// makeLuaInventoryAll returns the cmd-layer factory for the
// inventory_all(target_id) Lua binding (Phase F #32 slice 5c).
// Wraps ItemRepo.ListAllOwnedTransitive — same shape as
// makeLuaInventory but walks the parent_item_id chain so items
// inside containers appear in the result. The Lua-side row is the
// same {id, name, external_id}; container hierarchy is not
// surfaced in V1.
func makeLuaInventoryAll(items repo.ItemRepo) func(context.Context, int64) ([]luaeng.InventoryEntry, error) {
	return func(ctx context.Context, targetID int64) ([]luaeng.InventoryEntry, error) {
		if targetID == 0 {
			return nil, fmt.Errorf("target id must be non-zero")
		}
		list, err := items.ListAllOwnedTransitive(ctx, targetID)
		if err != nil {
			return nil, err
		}
		return itemsToInventoryEntries(list), nil
	}
}

// itemsToInventoryEntries projects repo.Item rows onto the
// lightweight Lua-side entry shape. Shared between
// makeLuaInventory + makeLuaInventoryAll so the row schema stays
// in lock-step.
func itemsToInventoryEntries(list []repo.Item) []luaeng.InventoryEntry {
	out := make([]luaeng.InventoryEntry, 0, len(list))
	for _, it := range list {
		out = append(out, luaeng.InventoryEntry{
			ID:         it.ID,
			Name:       it.Name,
			ExternalID: it.ExternalID,
		})
	}
	return out
}

// makeLuaWaitMs returns the cmd-layer factory for the
// wait_ms(milliseconds, script_name) Lua binding (Phase F #32
// slice 5c). Same shape as makeLuaWait but milliseconds-granular;
// the firing precision is bounded by the scheduler's tick rate
// (currently 1 Hz, so sub-1s values round up to the next pulse).
func makeLuaWaitMs(scheduler *tick.Scheduler, runner *luaeng.Runner, srvShutdownCtxPtr *atomic.Pointer[context.Context]) func(context.Context, trigger.EventCtx, int32, string) error {
	return buildLuaWaitFactory(scheduler, runner, srvShutdownCtxPtr, "wait_ms",
		func(ms int32) error {
			if ms < MinWaitMilliseconds {
				return fmt.Errorf("wait_ms milliseconds must be >= %d (got %d)", MinWaitMilliseconds, ms)
			}
			if ms > MaxWaitMilliseconds {
				return fmt.Errorf("wait_ms milliseconds must be <= %d (got %d)", MaxWaitMilliseconds, ms)
			}
			return nil
		},
		func(ms int32) time.Duration { return time.Duration(ms) * time.Millisecond },
	)
}
