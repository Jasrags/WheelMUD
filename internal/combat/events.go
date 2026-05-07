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
