package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/cmd"
	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/dialogue"
	"github.com/Jasrags/WheelMUD/internal/effects"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/group"
	"github.com/Jasrags/WheelMUD/internal/help"
	luaeng "github.com/Jasrags/WheelMUD/internal/lua"
	"github.com/Jasrags/WheelMUD/internal/mode"
	"github.com/Jasrags/WheelMUD/internal/news"
	"github.com/Jasrags/WheelMUD/internal/quest"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/tick"
	"github.com/Jasrags/WheelMUD/internal/trigger"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"

	luastd "github.com/yuin/gopher-lua"
)

func buildRegistry(rooms repo.RoomRepo, exits repo.ExitRepo, items repo.ItemRepo, mobs repo.MobInstanceRepo, mobTemplates repo.MobTemplateRepo, zones repo.ZoneRepo, characters repo.CharacterRepo, audits repo.AdminAuditRepo, shops repo.ShopRepo, bankers repo.BankerRepo, trainers repo.TrainerRepo, weaveTeachers repo.WeaveTeacherRepo, builderZones repo.BuilderZoneRepo, sessions *session.Registry, bus *eventbus.Bus, channels []repo.Channel, clock *world.Clock, newsCatalog *news.Catalog, helpCatalog *help.Catalog, chargenCatalog *chargen.Catalog, effectsCatalog *effects.Catalog, combatMgr *combat.Manager, groups *group.Manager, questCatalog *quest.Catalog, questEngine *quest.Engine, luaRunner *luaeng.Runner, scheduler *tick.Scheduler, srvShutdownCtxPtr *atomic.Pointer[context.Context], shutdownCtl cmd.ShutdownController) (*telnet.Registry, error) {
	r := telnet.NewRegistry()
	if err := r.Register(cmd.Quit, cmd.Colors); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewWho(sessions, characters),
		cmd.NewSay(sessions, rooms, bus),
		cmd.NewShout(sessions, rooms),
		cmd.NewYell(sessions, rooms),
		cmd.NewTell(sessions, bus),
		cmd.NewReply(sessions, bus),
	); err != nil {
		return nil, err
	}
	for _, ch := range channels {
		if err := r.Register(cmd.NewChannel(ch, sessions, characters, bus)); err != nil {
			return nil, err
		}
	}
	if err := r.Register(cmd.NewChannelsList(channels)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewAlias(), cmd.NewUnalias()); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewPrompt(characters, defaultPromptTemplate)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewPvP(characters)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewBind(characters, rooms)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewHelp(r, helpCatalog)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewLook(rooms, exits, items, mobs, clock)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewExamine(items, mobs)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewMoveFamily(rooms, exits, items, mobs, characters, bus, clock, sessions)...); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewOpen(exits, sessions),
		cmd.NewClose(exits, sessions),
		cmd.NewLock(exits, items, sessions),
		cmd.NewUnlock(exits, items, sessions),
		cmd.NewPick(exits, sessions),
	); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewTeleport(rooms, exits, items, mobs, characters, sessions, clock, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewGoto(rooms, exits, items, mobs, characters, sessions, clock, audits),
		cmd.NewTransfer(rooms, exits, items, mobs, characters, sessions, clock, audits),
		cmd.NewSummon(rooms, exits, items, mobs, characters, sessions, clock, audits),
		cmd.NewWizinvis(audits),
		cmd.NewShutdown(shutdownCtl, audits),
		cmd.NewReboot(shutdownCtl, audits),
	); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewInventory(items, characters),
		cmd.NewGet(items, characters, sessions),
		cmd.NewDrop(items, characters, sessions),
		cmd.NewGive(items, characters, sessions),
		cmd.NewPut(items, characters, sessions),
		cmd.NewQuaff(items, characters, effectsCatalog, sessions),
		cmd.NewWear(items, characters, sessions),
		cmd.NewWield(items, characters, sessions),
		cmd.NewRemove(items, characters, sessions),
		cmd.NewEquipment(items, characters),
		cmd.NewSpawn(items, mobTemplates, mobs, characters, sessions, audits),
	); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewAttack(combatMgr, rooms, mobs, characters, sessions, groups),
		cmd.NewPower(combatMgr, rooms, mobs, characters, sessions, groups),
		cmd.NewJab(combatMgr, rooms, mobs, characters, sessions, groups),
		cmd.NewThrow(combatMgr, rooms, mobs, characters, items, sessions, groups),
	); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewGroup(groups, sessions)); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewFollow(groups, sessions),
		cmd.NewUnfollow(),
	); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewFlee(combatMgr),
		cmd.NewParry(combatMgr, characters, sessions),
		cmd.NewDodge(combatMgr, sessions),
		cmd.NewSidestep(combatMgr, mobs, sessions),
	); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewList(items, mobs, mobTemplates, shops, clock),
		cmd.NewBuy(items, characters, mobs, mobTemplates, shops, clock, sessions),
		cmd.NewSell(items, characters, mobs, mobTemplates, shops, clock, sessions),
		cmd.NewValue(items, mobs, mobTemplates, shops, clock),
		cmd.NewBalance(characters, mobs, mobTemplates, bankers, clock),
		cmd.NewDeposit(characters, mobs, mobTemplates, bankers, clock, audits),
		cmd.NewWithdraw(characters, mobs, mobTemplates, bankers, clock, audits),
	); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewMap(rooms, exits, zones)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewWhereAmI(rooms, clock)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewTime(clock)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewTrack(mobs, rooms, exits)); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewGrant(builderZones, characters, zones, sessions, audits),
		cmd.NewRevoke(builderZones, characters, zones, sessions, audits),
		cmd.NewGrants(builderZones, characters, zones),
	); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewZones(zones, rooms)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewCoords(rooms, exits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewZoneMap(rooms, exits, zones)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewNews(newsCatalog, characters)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewScore(characters, items, chargenCatalog)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewXP(characters)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewTrain(characters, mobs, mobTemplates, trainers, chargenCatalog, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewLearn(characters, chargenCatalog, audits, mobs, mobTemplates, weaveTeachers)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewFeat(characters, chargenCatalog, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewBump(characters, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewEmbrace(characters)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewRelease(characters)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewStill(characters, sessions, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewUnstill(characters, sessions, audits)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewAffects(characters)); err != nil {
		return nil, err
	}
	if err := r.Register(
		cmd.NewAffect(characters, sessions, audits),
		cmd.NewDispel(characters, sessions, audits),
	); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewCooldowns(characters, chargenCatalog)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewCooldown(characters, sessions, audits, chargenCatalog)); err != nil {
		return nil, err
	}
	// Phase F #30: NPC dialogue trees. main.go is the only place that
	// can hand cmd a closure that builds a *mode.Dialogue, since cmd
	// can't import internal/mode without risking an import cycle
	// through chargen.
	hooks := mode.DialogueHooks{}
	if questEngine != nil {
		// Honor the dispatcher's ctx so a stalled repo write under
		// shutdown drain or session teardown unblocks instead of
		// holding the dialogue handler open. Engine repo calls
		// already accept the propagated ctx.
		hooks.AcceptQuest = func(ctx context.Context, s *telnet.Session, questID string) error {
			return questEngine.AcceptQuest(ctx, s.CharacterID, questID)
		}
		hooks.AdvanceQuest = func(ctx context.Context, s *telnet.Session, questID, npcExternalID string) error {
			return questEngine.AdvanceTalkTo(ctx, s.CharacterID, questID, npcExternalID)
		}
	}
	// Phase F #32 slice 2: dialogue `script` effect runs a Lua
	// catalog script with the V2 mutation surface. Hook is unbound
	// when no Lua runner is wired (test harness, malformed boot) —
	// applyEffects logs and continues so a misconfigured boot
	// doesn't lock players inside the dialogue.
	if luaRunner != nil {
		// Phase F #32 slice 5c: hoist the wait factories OUT of
		// RunScript so they're built once at registry setup time
		// rather than re-allocating per dialogue script fire. The
		// returned closures snapshot the scheduler + shutdown-ctx
		// pointer; they're immutable, safe to share across calls.
		dialogueWaitHook := makeLuaWait(scheduler, luaRunner, srvShutdownCtxPtr)
		dialogueWaitMsHook := makeLuaWaitMs(scheduler, luaRunner, srvShutdownCtxPtr)
		dialogueInventoryHook := makeLuaInventory(items)
		dialogueInventoryAllHook := makeLuaInventoryAll(items)
		hooks.RunScript = func(ctx context.Context, s *telnet.Session, name string) error {
			bindings := luaeng.APIBindings{
				Logger: slog.Default(),
				Ctx: luaeng.CtxView{
					Event:     "dialogue.script",
					ActorID:   s.CharacterID,
					ActorKind: "character",
					RoomID:    s.CurrentRoomID,
					Text:      name,
				},
			}
			if questEngine != nil {
				bindings.QuestAccept = func(id string) error {
					return questEngine.AcceptQuest(ctx, s.CharacterID, id)
				}
				bindings.QuestAdvance = func(id string) error {
					return questEngine.Advance(ctx, s.CharacterID, id)
				}
			}
			// V3 surface (Phase F #32 slice 3). Reuse the same
			// closure constructors the trigger path uses; the
			// dialogue actor is always the calling character so no
			// extra actor-kind guard is needed here.
			applyAffect := makeLuaApplyAffect(characters, effectsCatalog)
			giveItem := makeLuaGiveItem(items)
			targetHP := makeLuaTargetHP(characters)
			targetLevel := makeLuaTargetLevel(characters)
			targetClasses := makeLuaTargetClasses(characters, chargenCatalog)
			roomPlayers := makeLuaRoomPlayers(sessions)
			roomMobs := makeLuaRoomMobs(mobs)
			bindings.ApplyAffect = func(targetID int64, effectID string, durationOverride int32) error {
				return applyAffect(ctx, targetID, effectID, durationOverride)
			}
			// Per-invocation give_item cap (mirrors trigger path).
			// Counter is captured per RunScript call so each
			// dialogue script fire gets a fresh budget.
			giveCount := 0
			bindings.GiveItem = func(targetID int64, externalID string) error {
				giveCount++
				if giveCount > trigger.MaxGiveItemsPerInvocation {
					return fmt.Errorf("give_item exceeded per-invocation cap of %d", trigger.MaxGiveItemsPerInvocation)
				}
				return giveItem(ctx, targetID, externalID)
			}
			bindings.TargetHP = func(targetID int64) (int32, int32, error) {
				return targetHP(ctx, targetID)
			}
			bindings.TargetLevel = func(targetID int64) (int, error) {
				return targetLevel(ctx, targetID)
			}
			bindings.TargetClasses = func(targetID int64) (map[string]int, error) {
				return targetClasses(ctx, targetID)
			}
			// V4 surface (Phase F #32 slice 4) — room queries
			// resolve from the dialogue session's CurrentRoomID,
			// not a Lua-side argument.
			bindings.RoomPlayers = func() ([]int64, error) {
				return roomPlayers(ctx, s.CurrentRoomID)
			}
			bindings.RoomMobs = func() ([]int64, error) {
				return roomMobs(ctx, s.CurrentRoomID)
			}
			bindings.ClockHour = clock.HourOfDay
			bindings.ClockDay = clock.Day
			// V5a surface (Phase F #32 slice 5a) — combat +
			// inventory mutations. Dialogue scripts always fire in
			// character context so target resolution / room
			// context come straight from the session.
			dealDamage := makeLuaDealDamage(combatMgr, characters, mobs)
			heal := makeLuaHeal(combatMgr, characters, mobs)
			transferItem := makeLuaTransferItem(items)
			dropItem := makeLuaDropItem(items)
			bindings.DealDamage = func(targetID int64, amount int32, source string) error {
				return dealDamage(ctx, targetID, amount, source)
			}
			bindings.Heal = func(targetID int64, amount int32) error {
				return heal(ctx, targetID, amount)
			}
			bindings.TransferItem = func(itemID, toOwnerID int64) error {
				return transferItem(ctx, itemID, toOwnerID)
			}
			bindings.DropItem = func(itemID int64) error {
				if s.CurrentRoomID == 0 {
					return fmt.Errorf("drop_item requires a room context (session not in a room)")
				}
				return dropItem(ctx, itemID, s.CurrentRoomID)
			}
			// V5b + V5c surface for dialogue (Phase F #32 slice 5c).
			// wait / wait_ms are wired through a synthetic
			// EventCtx so the deferred run inherits the dialogue
			// session's actor + room context. Authors writing
			// "after a beat, the NPC follows up" patterns now
			// stay inside the dialogue path without the trigger
			// detour. Note: the deferred script fires AFTER the
			// dialogue mode may have popped — it gets the minimal
			// binding surface (ctx + logger), same as triggers.
			bindings.Inventory = func(targetID int64) ([]luaeng.InventoryEntry, error) {
				return dialogueInventoryHook(ctx, targetID)
			}
			bindings.InventoryAll = func(targetID int64) ([]luaeng.InventoryEntry, error) {
				return dialogueInventoryAllHook(ctx, targetID)
			}
			waitEv := trigger.EventCtx{
				Event:     "dialogue.script",
				ActorID:   s.CharacterID,
				ActorKind: "character",
				RoomID:    s.CurrentRoomID,
				Text:      name,
			}
			bindings.Wait = func(seconds int32, scriptName string) error {
				return dialogueWaitHook(ctx, waitEv, seconds, scriptName)
			}
			bindings.WaitMs = func(ms int32, scriptName string) error {
				return dialogueWaitMsHook(ctx, waitEv, ms, scriptName)
			}
			// PushMode is intentionally nil for dialogue scripts: V2
			// has no concrete cross-mode push targets and the
			// classified Lua error makes the unbound state visible
			// to authors.
			return luaRunner.Run(ctx, name, func(l *luastd.LState) { bindings.Bind(l) })
		}
	}
	pushDialogue := func(s *telnet.Session, npcName, npcExternalID string, tree *dialogue.Tree) error {
		dm, err := mode.NewDialogue(npcName, npcExternalID, tree, hooks)
		if err != nil {
			return err
		}
		return s.PushMode(dm)
	}
	if err := r.Register(cmd.NewTalk(mobs, mobTemplates, pushDialogue)); err != nil {
		return nil, err
	}
	pushREdit := func(s *telnet.Session, room repo.Room) error {
		return s.PushMode(mode.NewREdit(rooms, exits, audits, rooms.FindByExternalID, room))
	}
	if err := r.Register(cmd.NewREdit(rooms, pushREdit)); err != nil {
		return nil, err
	}
	if err := r.Register(cmd.NewQuest(characters, questCatalog, questEngine, audits)); err != nil {
		return nil, err
	}
	return r, nil
}
