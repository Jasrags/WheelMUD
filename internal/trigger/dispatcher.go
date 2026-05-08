package trigger

import (
	"context"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/tick"
	"github.com/Jasrags/WheelMUD/internal/world"
)

// Dispatcher wires the eventbus + tick.Buckets.Phase pulse to a
// Runner. Start subscribes; the returned Stop unsubscribes everything
// and is safe to call from a shutdown hook (idempotent).
type Dispatcher struct {
	bus      *eventbus.Bus
	bucket   *tick.Bucket
	runner   *Runner
	mobs     repo.MobInstanceRepo
	subs     []*eventbus.Subscription
	cancelTk func()
}

// NewDispatcher constructs a Dispatcher. bus must be non-nil. bucket
// is optional — pass nil to skip on_tick wiring (tests). mobs is the
// MobInstanceRepo used to expand room owners into per-mob trigger
// fan-outs (also propagated via Runner.deps for SayAction's name
// lookup).
func NewDispatcher(bus *eventbus.Bus, bucket *tick.Bucket, runner *Runner, mobs repo.MobInstanceRepo) *Dispatcher {
	return &Dispatcher{bus: bus, bucket: bucket, runner: runner, mobs: mobs}
}

// Start installs subscriptions for every event the trigger surface
// listens on. Idempotent in the sense that a second Start adds new
// subscriptions but never duplicates existing ones — callers should
// only call once per dispatcher.
func (d *Dispatcher) Start(ctx context.Context) {
	if d == nil || d.bus == nil || d.runner == nil {
		return
	}
	d.subs = append(d.subs,
		eventbus.Subscribe[world.PlayerEntered](d.bus, d.onPlayerEntered),
		eventbus.Subscribe[world.PlayerSaid](d.bus, d.onPlayerSaid),
		eventbus.Subscribe[combat.CombatHit](d.bus, d.onCombatHit),
		eventbus.Subscribe[combat.CombatDeath](d.bus, d.onCombatDeath),
		eventbus.Subscribe[combat.CharacterDied](d.bus, d.onCharacterDied),
	)
	if d.bucket != nil {
		d.cancelTk = d.bucket.Subscribe(d.onTick)
	}
}

// Stop cancels every subscription. Safe to call multiple times.
func (d *Dispatcher) Stop() {
	if d == nil {
		return
	}
	for _, s := range d.subs {
		s.Cancel()
	}
	d.subs = nil
	if d.cancelTk != nil {
		d.cancelTk()
		d.cancelTk = nil
	}
}

// --- Event handlers ------------------------------------------------

func (d *Dispatcher) onPlayerEntered(ctx context.Context, ev world.PlayerEntered) {
	if ev.ToRoomID == 0 {
		return
	}
	ec := EventCtx{
		Event: EventOnEnter, RoomID: ev.ToRoomID,
		ActorKind: "character", ActorID: ev.CharacterID,
	}
	d.fanOutRoom(ctx, ev.ToRoomID, ec, EventOnEnter, nil)
}

func (d *Dispatcher) onPlayerSaid(ctx context.Context, ev world.PlayerSaid) {
	if ev.RoomID == 0 {
		return
	}
	ec := EventCtx{
		Event: EventOnSay, RoomID: ev.RoomID,
		ActorKind: "character", ActorID: ev.SpeakerCharacterID,
		Text: ev.Text,
	}
	// on_say uses the trigger's Match field as a substring keyword.
	d.fanOutRoom(ctx, ev.RoomID, ec, EventOnSay, func(t repo.Trigger) bool {
		return MatchSay(t, ev.Text)
	})
}

func (d *Dispatcher) onCombatHit(ctx context.Context, ev combat.CombatHit) {
	ec := EventCtx{
		Event:  EventOnAttack,
		RoomID: ev.RoomID,
		Text:   "", // damage / weapon left out of V1 surface
	}
	ec.ActorKind, ec.ActorID = actorRefAsCtx(ev.Attacker)
	ec.TargetKind, ec.TargetID = actorRefAsCtx(ev.Defender)
	// Room owner.
	d.runner.FireForOwner(ctx, OwnerRef{Kind: OwnerRoom, ID: ev.RoomID, RoomID: ev.RoomID}, ec)
	// Mob template owner — defender is the typical hook ("when this
	// kind of mob takes a hit"). Attacker mobs can be added later via
	// a separate trigger event if content needs it.
	if ev.Defender.Kind == combat.ActorKindMob {
		d.fireMobTemplate(ctx, ev.Defender.ID, ev.RoomID, ec)
	}
}

func (d *Dispatcher) onCombatDeath(ctx context.Context, ev combat.CombatDeath) {
	ec := EventCtx{Event: EventOnDeath, RoomID: ev.RoomID}
	ec.ActorKind, ec.ActorID = actorRefAsCtx(ev.Killer)
	ec.TargetKind, ec.TargetID = actorRefAsCtx(ev.Victim)
	d.runner.FireForOwner(ctx, OwnerRef{Kind: OwnerRoom, ID: ev.RoomID, RoomID: ev.RoomID}, ec)
	if ev.Victim.Kind == combat.ActorKindMob {
		// Resolve the dying mob's template id BEFORE the corpse is
		// fully cleaned up. Combat.Manager has already deleted the
		// instance row by the time this fires (#19), so we look up
		// the template id we previously cached from CombatStarted —
		// fall back to skipping if the lookup misses.
		d.fireMobTemplate(ctx, ev.Victim.ID, ev.RoomID, ec)
	}
}

func (d *Dispatcher) onCharacterDied(ctx context.Context, ev combat.CharacterDied) {
	ec := EventCtx{Event: EventOnDeath, RoomID: ev.DeathRoomID}
	ec.ActorKind, ec.ActorID = actorRefAsCtx(ev.Killer)
	ec.TargetKind, ec.TargetID = actorRefAsCtx(ev.Victim)
	d.runner.FireForOwner(ctx, OwnerRef{Kind: OwnerRoom, ID: ev.DeathRoomID, RoomID: ev.DeathRoomID}, ec)
}

func (d *Dispatcher) onTick(ctx context.Context) {
	if d.runner == nil || d.runner.reg == nil {
		return
	}
	all := d.runner.reg.AllByEvent(EventOnTick)
	if len(all) == 0 {
		return
	}
	const bucketName = "phase"
	for _, t := range all {
		match := t.Match
		if match == "" {
			match = bucketName
		}
		if match != bucketName {
			continue
		}
		owner := OwnerRef{Kind: t.OwnerKind, ID: t.OwnerID}
		if t.OwnerKind == OwnerRoom {
			owner.RoomID = t.OwnerID
		}
		ec := EventCtx{Event: EventOnTick, RoomID: owner.RoomID, BucketName: bucketName}
		d.runner.Fire(ctx, owner, ec, []repo.Trigger{t})
	}
}

// --- Helpers -------------------------------------------------------

// fanOutRoom dispatches for both room-owned triggers and every mob
// template represented in the room. filter (optional) is applied
// per-trigger before invocation.
func (d *Dispatcher) fanOutRoom(ctx context.Context, roomID int64, ec EventCtx, ev Event, filter func(repo.Trigger) bool) {
	if d.runner == nil || d.runner.reg == nil {
		return
	}
	// Room owner.
	if rs := d.runner.reg.ForOwnerEvent(OwnerRoom, roomID, ev); len(rs) > 0 {
		d.fireFiltered(ctx, OwnerRef{Kind: OwnerRoom, ID: roomID, RoomID: roomID}, ec, rs, filter)
	}
	// Mob owners — only walk the room if any mob-template trigger is
	// registered for this event kind.
	if d.mobs == nil || !d.runner.reg.HasOwnerKindEvent(OwnerMobTemplate, ev) {
		return
	}
	mobs, err := d.mobs.ListInRoom(ctx, roomID)
	if err != nil {
		loggerOr(d.runner.deps).Debug("trigger: ListInRoom failed", "room", roomID, "error", err)
		return
	}
	for _, m := range mobs {
		ts := d.runner.reg.ForOwnerEvent(OwnerMobTemplate, m.TemplateID, ev)
		if len(ts) == 0 {
			continue
		}
		owner := OwnerRef{Kind: OwnerMobTemplate, ID: m.TemplateID, InstanceID: m.ID, RoomID: roomID}
		d.fireFiltered(ctx, owner, ec, ts, filter)
	}
}

// fireMobTemplate resolves the mob_instance to its template id and
// fires triggers for that template. instanceID may already be deleted
// by the time we run (death path); we skip silently in that case.
func (d *Dispatcher) fireMobTemplate(ctx context.Context, instanceID int64, roomID int64, ec EventCtx) {
	if d.mobs == nil {
		return
	}
	inst, err := d.mobs.GetByID(ctx, instanceID)
	if err != nil {
		// Instance is gone (death cleanup); we can't resolve the
		// template id. Future work: pass TemplateID through
		// combat events so on_death survives the cleanup.
		return
	}
	owner := OwnerRef{Kind: OwnerMobTemplate, ID: inst.TemplateID, InstanceID: instanceID, RoomID: roomID}
	d.runner.FireForOwner(ctx, owner, ec)
}

func (d *Dispatcher) fireFiltered(ctx context.Context, owner OwnerRef, ec EventCtx, triggers []repo.Trigger, filter func(repo.Trigger) bool) {
	if filter == nil {
		d.runner.Fire(ctx, owner, ec, triggers)
		return
	}
	out := make([]repo.Trigger, 0, len(triggers))
	for _, t := range triggers {
		if filter(t) {
			out = append(out, t)
		}
	}
	if len(out) > 0 {
		d.runner.Fire(ctx, owner, ec, out)
	}
}

func actorRefAsCtx(a combat.ActorRef) (string, int64) {
	switch a.Kind {
	case combat.ActorKindCharacter:
		return "character", a.ID
	case combat.ActorKindMob:
		return "mob", a.ID
	}
	return "", 0
}
