package combat

// Typed events published on the eventbus when combat state changes.
// Subscribers register via eventbus.Subscribe[combat.RoundStarted](bus, fn).
//
// All three events carry RoomID first so a single subscriber can
// route to the right Fight without consulting the Manager.

// CombatStarted fires once when a Fight is opened. Order is the
// resolved initiative slate at start time — a snapshot, not a live
// reference. Future mid-fight joins (slice 2) will publish a
// separate ParticipantJoined event rather than re-publishing
// CombatStarted.
type CombatStarted struct {
	RoomID int64
	Order  []ActorEntry
}

// RoundStarted fires every Combat-bucket pulse for an active fight.
// Active is the actor whose turn just opened. Round is 1-based.
type RoundStarted struct {
	RoomID int64
	Round  int
	Active ActorRef
}

// CombatEnded fires once when a Fight is removed from the Manager.
// Reason is a fixed-vocabulary token: "explicit" (caller invoked
// End), "no_participants" (auto-end at Tick when every combatant
// has left the room or vanished), "manager_stop" (Manager.Stop on
// shutdown). Subscribers should treat it as informational; the
// Manager has already cleaned up by the time this publishes.
type CombatEnded struct {
	RoomID int64
	Reason string
}

// Reason tokens. Kept as untyped strings rather than an enum because
// the set is tiny and the strings are what end up in slog kv-pairs
// for forensics.
const (
	ReasonExplicit       = "explicit"
	ReasonNoParticipants = "no_participants"
	ReasonManagerStop    = "manager_stop"
)

// CombatHit fires when a queued attack lands. Damage is the post-DR /
// post-resist amount actually subtracted from defender HP. Weapon is
// the item id of the wielded weapon at resolve time (0 = unarmed).
// Subscribers compose the room broadcast and the per-session feedback
// off this event so the resolver stays a pure stateless function.
type CombatHit struct {
	RoomID   int64
	Attacker ActorRef
	Defender ActorRef
	Damage   int32
	Weapon   int64
	IsCrit   bool
	// Variant carries the attack variant resolved this swing
	// (Normal / Power / Quick). Subscribers compose variant-flavored
	// echo lines off this field.
	Variant AttackVariant
}

// CombatMiss fires when a queued attack rolls below the defender's
// Defense. RollTotal is the d20 + bab + ability_mod; Defense is the
// value the roll was compared against.
type CombatMiss struct {
	RoomID    int64
	Attacker  ActorRef
	Defender  ActorRef
	RollTotal int
	Defense   int16
	// Variant carries the attack variant resolved this swing
	// (Normal / Power / Quick). Subscribers compose variant-flavored
	// echo lines off this field.
	Variant AttackVariant
}

// ActionResolved fires whether the queued action was an attack that
// hit, missed, or was a no-op (target gone). Mirrors RoundStarted —
// subscribers that just want "the active actor finished their turn"
// don't have to fan out across CombatHit / CombatMiss.
type ActionResolved struct {
	RoomID int64
	Round  int
	Actor  ActorRef
	Kind   ActionKind
}

// CombatDeath fires once when a participant's HP falls to zero or
// below and the death handler has run. CorpseID is the item id of
// the corpse spawned in the room (zero when the death path could
// not create one — e.g. item-repo failure). Killer is the ActorRef
// whose hit dropped HP to zero; ActorKindUnknown when the kill came
// from a non-combat source like environmental damage (#19 slice 1
// always passes the attacker).
type CombatDeath struct {
	RoomID   int64
	Victim   ActorRef
	Killer   ActorRef
	CorpseID int64

	// MobTemplateID and MobTemplateExternalID carry the dying mob's
	// template identity (Phase F #31 — quest kill_n step matching).
	// Only populated when Victim.Kind == ActorKindMob; both are zero
	// for character victims (CharacterDied carries that path). Filled
	// here at publish time so subscribers don't have to re-fetch the
	// instance row after combat cleanup deletes it.
	MobTemplateID         int64
	MobTemplateExternalID string

	// VictimName is the dying mob's display name at publish time.
	// Stamped here because handleMobDeath deletes the mob_instance
	// row before this event fires, so subscribers can't fall back to
	// the live repo lookup the way combatActorName does for live
	// participants. Empty for character victims (CharacterDied
	// carries the equivalent).
	VictimName string
}

// CombatXPAwarded fires once per attacker that earns a share of the
// kill XP. Subscribers (combat prompt, level-up watcher, audit log)
// can observe per-actor awards without re-deriving the tally. Amount
// is the net value added to the character's xp column AFTER any
// XP-debt offset (Phase D §19 player-death slice). DebtTaken is how
// much of the gross share went to draining xp_debt instead — zero
// when the awardee had no debt. The two add up to the gross share
// from `allocateXP`.
type CombatXPAwarded struct {
	RoomID    int64
	Awardee   ActorRef
	Amount    int64
	DebtTaken int64
	Killed    ActorRef
}

// CharacterDied fires when a player character's HP falls to zero or
// below and the death handler has run. Published before the room
// transfer so subscribers in the death room see the victim's
// CurrentRoomID still pointing at the original room. Cmd-layer
// subscribers broadcast the "X falls dead!" line via WriteAsync to
// peers in DeathRoomID, then ignore the dying player's own session
// (which gets the "You die!" line from the dispatcher path).
type CharacterDied struct {
	DeathRoomID int64
	Victim      ActorRef
	Killer      ActorRef
	BoundRoomID int64
	XPDebtAdded int64
}

// CharacterRespawned fires when a dead player has been moved to
// their BoundRoomID, healed, and cleared of death-related conditions.
// Cmd-layer subscribers broadcast "X appears, eyes hollow." to peers
// in RoomID via WriteAsync, and write "You die!" + the bound-room
// look to the victim's session.
type CharacterRespawned struct {
	PrevRoomID int64
	RoomID     int64
	Character  ActorRef
}

// CombatParry fires when a defender's parrying stance deflects an
// incoming attack. Parry / Attack are the two opposed-roll totals
// (d20 + BAB + ability mod). The defender's stance is consumed by
// the trigger; the attacker is set flat-footed for one round.
type CombatParry struct {
	RoomID   int64
	Defender ActorRef
	Attacker ActorRef
	Parry    int
	Attack   int
}

// CombatStance fires when an actor enters a combat stance ("parry",
// "dodge", or "sidestep"). Subscribers compose room broadcasts off
// this so the resolver doesn't need to know about cfmt or session
// plumbing. Target is the named actor for stances that aim at a
// specific peer (sidestep names the attacker to flat-foot) — zero
// ActorRef for self-only stances (parry, dodge).
type CombatStance struct {
	RoomID int64
	Actor  ActorRef
	Kind   string // "parry" | "dodge" | "sidestep"
	Target ActorRef
}

// CombatDodgeAvoided fires when a defender's dodge stance turns an
// otherwise-hitting swing into a miss via the +4 Defense / flat-foot
// immunity grant. Distinct from CombatMiss so subscribers can render
// the active "you twist aside" line rather than a passive "you miss".
// The stance is consumed by the trigger.
type CombatDodgeAvoided struct {
	RoomID   int64
	Defender ActorRef
	Attacker ActorRef
	Roll     int   // attacker total before dodge bonus
	Defense  int16 // defender Defense after +4 dodge bonus
}

// CombatThrow fires when an ActionThrow resolves, AFTER the wielded
// weapon has been cleared from the actor's primary wield slot and
// AFTER the hit/miss publish. Subscribers reading defender HP or the
// attacker's equipment slot can trust both reflect the post-throw
// state. RollHit is the bare hit/miss outcome of the roll; subscribers
// that need the actual damage should read CombatHit (fired immediately
// before this on a successful throw) — RollHit does NOT promise the
// item was successfully consumed (the equipment-clear path log-and-
// continues on a repo failure; see resolveThrow).
//
// ItemName is captured at resolve time so subscribers don't have to
// round-trip the item repo after the item has been moved.
type CombatThrow struct {
	RoomID   int64
	Attacker ActorRef
	Defender ActorRef
	ItemID   int64
	ItemName string
	RollHit  bool
}

// CombatFlee fires when an ActionFlee resolves — whether the actor
// successfully retreated or was caught. Direction / ToRoomID are
// populated only on Success; Reason carries a short token from the
// FleeMover ("moved", "no_exits", "all_blocked", "rolled_failure").
type CombatFlee struct {
	RoomID    int64
	Actor     ActorRef
	Success   bool
	Direction string
	ToRoomID  int64
	Reason    string
}

// CorpseDecayed fires once when the Decayer sweeps a corpse out of
// the world. RoomID is where the corpse was at decay time (and where
// the crumble line was broadcast); ItemID is the deleted corpse row.
// Subscribers can use this to gate "you missed the loot" hints when
// looting verbs land. Published whether or not the items.Delete
// succeeded — the in-memory queue entry is consumed regardless so a
// failing repo doesn't leak the schedule slot.
type CorpseDecayed struct {
	RoomID int64
	ItemID int64
}
