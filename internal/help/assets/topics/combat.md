---
id: combat
title: Combat
keywords: fight, attack, kill
---
Combat in WheelMUD is round-based and resolves in the
{{combat}}::yellow tick bucket. Each round, every active fighter
makes one attack roll (d20 + base attack bonus + ability) against
the target's Defense; on a hit, weapon damage is reduced by the
target's damage reduction and resists.

Combat is not yet wired in this build — see ROADMAP.md §11 for the
full plan. The stat blocks, weapon stats, and DR/resist columns
already exist on the creature core; only the round-tick and roll
plumbing remain.

Related: {{help weapons}}::cyan, {{help damage}}::cyan once those
topics land.
