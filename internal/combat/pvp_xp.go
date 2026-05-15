package combat

// PvP XP awards (Phase D §19 closer). When a character kills another
// character via the attack verb, the killer earns a modest XP pool
// scaled to the victim's level. Verb-layer gates (nopvp room, newbie
// cap, mutual opt-in, same-group block) have already refused
// illegitimate attacks; the only anti-abuse guard at this layer is
// the level-differential clamp that zeroes the reward when the
// attacker dramatically outlevels the victim.

// PvPXPPerVictimLevel is the base reward per level of the victim.
// Deliberately small — a level-12 kill grants 600 XP, well below
// the same-tier mob curve (e.g. C-tier mob = 600 XP) — so PvP
// doesn't replace mob grinding as the primary XP source. Tunable
// based on play-test feedback; widening this is a one-line knob.
const PvPXPPerVictimLevel int64 = 50

// PvPLevelDiffCap is the maximum (attackerLevel - victimLevel) that
// still awards XP. Above this delta the kill is "farming low alts"
// and returns 0. Same-direction asymmetry is intentional: weak
// attackers killing strong victims always get full credit, since
// that's a meaningful achievement (and rare under the d20 hit math).
const PvPLevelDiffCap = 5

// pvpXPForKill returns the gross XP pool for a successful player
// kill. Returns 0 when the level differential is too wide or the
// victim has no levels (chargen-incomplete corner case). The
// attacker's pool is then split across the damage tally via the
// same allocateXP path used by mob kills, so per-share group
// expansion and debt drain apply normally.
func pvpXPForKill(attackerLevel, victimLevel int) int64 {
	if victimLevel <= 0 {
		return 0
	}
	if attackerLevel-victimLevel > PvPLevelDiffCap {
		return 0
	}
	return PvPXPPerVictimLevel * int64(victimLevel)
}
