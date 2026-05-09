package quest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// killNState is the per-character per-step JSON shape for StepKillN.
// Marshaled into QuestProgress.StateJSON. talk_to and reach_room
// store "{}" — they have no per-step counter.
type killNState struct {
	Remaining int `json:"remaining"`
}

// Engine owns quest state mutation. It subscribes to combat /
// world events at Start, processes one event per goroutine call
// (eventbus delivery is serialized per subscription), and persists
// state changes via CharacterRepo.RecordQuestProgress.
//
// All event handlers reload the character's QuestLog before
// mutating — we accept the small read overhead in exchange for not
// having to track per-character cache coherence.
//
// Concurrency caveat: two CombatDeath events for the same character
// firing back-to-back can both load the same `remaining` count
// before either persists. The second write overwrites the first,
// silently losing one decrement. This is tolerated for V1 because
// (a) the eventbus serializes per-subscription, so true
// simultaneity requires an event-publishing race upstream, and
// (b) single-session-per-account means the same character isn't
// fighting in two places at once. When multi-session lands the
// engine should adopt the same optimistic-lock token pattern as
// `RecordCoin`. Tracked in the shared `optimistic_lock_followups`
// memory.
type Engine struct {
	cat      *Catalog
	chars    repo.CharacterRepo
	rooms    repo.RoomRepo
	audits   repo.AdminAuditRepo
	bus      *eventbus.Bus
	sessions *session.Registry

	subs []*eventbus.Subscription
}

// NewEngine constructs a quest Engine. cat must be non-nil and
// validated; nil bus disables event subscriptions (useful for
// effects-only unit tests).
func NewEngine(cat *Catalog, chars repo.CharacterRepo, rooms repo.RoomRepo,
	audits repo.AdminAuditRepo, bus *eventbus.Bus, sessions *session.Registry,
) *Engine {
	return &Engine{
		cat:      cat,
		chars:    chars,
		rooms:    rooms,
		audits:   audits,
		bus:      bus,
		sessions: sessions,
	}
}

// Start installs eventbus subscriptions for kill_n and reach_room.
// talk_to is driven directly via AdvanceTalkTo (called by the
// dialogue advance_quest hook), not by an event subscription.
func (e *Engine) Start(ctx context.Context) {
	if e == nil || e.bus == nil {
		return
	}
	e.subs = append(e.subs,
		eventbus.Subscribe[combat.CombatDeath](e.bus, e.onCombatDeath),
		eventbus.Subscribe[world.PlayerEntered](e.bus, e.onPlayerEntered),
	)
}

// Stop cancels every subscription. Safe to call multiple times.
// Mirrors trigger.Dispatcher.Stop.
func (e *Engine) Stop() {
	if e == nil {
		return
	}
	for _, s := range e.subs {
		s.Cancel()
	}
	e.subs = nil
}

// --- Public effect entry points (called by dialogue closures) ----

// AcceptQuest adds the quest to the character's log at step 0.
// No-op (returns nil) if:
//
//   - The quest id isn't in the catalog.
//   - The character already has the quest in their log (active or
//     completed). Re-clicking the giver is safe.
//
// The dialogue layer treats errors as warnings; this method only
// returns errors for genuine repo failures.
func (e *Engine) AcceptQuest(ctx context.Context, charID int64, questID string) error {
	q, ok := e.cat.Get(questID)
	if !ok {
		slog.Warn("quest accept: unknown quest id", "char", charID, "quest", questID)
		return nil
	}
	char, err := e.chars.GetByID(ctx, charID)
	if err != nil {
		return fmt.Errorf("get character: %w", err)
	}
	for _, p := range char.QuestLog {
		if p.QuestID == questID {
			return nil // already known — accept is idempotent
		}
	}

	state, _ := stepInitState(q.Steps[0])
	log := append(char.QuestLog, creature.QuestProgress{
		QuestID:   questID,
		StepIndex: 0,
		StateJSON: state,
	})
	if err := e.chars.RecordQuestProgress(ctx, charID, log); err != nil {
		return fmt.Errorf("record quest progress: %w", err)
	}
	e.notifyAccepted(charID, q)
	return nil
}

// AdvanceTalkTo advances the active step of a quest IFF the step
// is StepTalkTo and matches npcExternalID. Called by the dialogue
// advance_quest effect hook. Mismatches log + no-op so authoring
// errors don't lock players out.
func (e *Engine) AdvanceTalkTo(ctx context.Context, charID int64, questID, npcExternalID string) error {
	q, ok := e.cat.Get(questID)
	if !ok {
		slog.Warn("quest advance: unknown quest id", "char", charID, "quest", questID)
		return nil
	}
	char, err := e.chars.GetByID(ctx, charID)
	if err != nil {
		return fmt.Errorf("get character: %w", err)
	}
	idx, found := findActive(char.QuestLog, questID)
	if !found {
		slog.Debug("quest advance: not on quest", "char", charID, "quest", questID)
		return nil
	}
	step := q.Steps[char.QuestLog[idx].StepIndex]
	if step.Kind != StepTalkTo || step.Mob != npcExternalID {
		slog.Debug("quest advance: step mismatch",
			"char", charID, "quest", questID,
			"step_kind", step.Kind, "step_mob", step.Mob, "npc", npcExternalID)
		return nil
	}
	return e.advanceStep(ctx, char, q, idx)
}

// Advance is the kind-agnostic advance entry point used by the V2
// Lua API (`quest.advance`). It looks up the active step for
// (charID, questID) and advances iff the step kind is one of the
// "passive" kinds whose progress isn't event-counted: StepTalkTo
// or StepScript. StepKillN / StepReachRoom have their own per-event
// counters and ignore Advance — calling Advance on them is logged
// as a content bug and treated as a no-op so the player isn't
// surprised by an early skip.
//
// Returns nil even on no-op cases (unknown quest, not on quest,
// non-advanceable step kind). Returning an error here would propagate
// up through the Lua binding and surface in the script as a runtime
// error — but the script can't tell the difference between "your
// dialogue called advance on the wrong quest" and "the repo failed",
// so we keep behavior aligned with AdvanceTalkTo: log + nil.
func (e *Engine) Advance(ctx context.Context, charID int64, questID string) error {
	q, ok := e.cat.Get(questID)
	if !ok {
		slog.Warn("quest advance: unknown quest id", "char", charID, "quest", questID)
		return nil
	}
	char, err := e.chars.GetByID(ctx, charID)
	if err != nil {
		return fmt.Errorf("get character: %w", err)
	}
	idx, found := findActive(char.QuestLog, questID)
	if !found {
		slog.Debug("quest advance: not on quest", "char", charID, "quest", questID)
		return nil
	}
	step := q.Steps[char.QuestLog[idx].StepIndex]
	switch step.Kind {
	case StepTalkTo, StepScript:
		return e.advanceStep(ctx, char, q, idx)
	default:
		slog.Warn("quest advance: non-advanceable step kind",
			"char", charID, "quest", questID, "step_kind", step.Kind)
		return nil
	}
}

// AbandonQuest removes the quest entry from the character's log.
// Used by the `quest abandon <id>` verb. Returns nil if the quest
// isn't in the log (idempotent).
func (e *Engine) AbandonQuest(ctx context.Context, charID int64, questID string) error {
	char, err := e.chars.GetByID(ctx, charID)
	if err != nil {
		return fmt.Errorf("get character: %w", err)
	}
	idx, found := findActive(char.QuestLog, questID)
	if !found {
		return nil
	}
	log := append([]creature.QuestProgress{}, char.QuestLog[:idx]...)
	log = append(log, char.QuestLog[idx+1:]...)
	return e.chars.RecordQuestProgress(ctx, charID, log)
}

// --- Event handlers ----------------------------------------------

func (e *Engine) onCombatDeath(ctx context.Context, ev combat.CombatDeath) {
	if ev.Killer.Kind != combat.ActorKindCharacter || ev.Killer.ID == 0 {
		return
	}
	if ev.Victim.Kind != combat.ActorKindMob {
		return
	}
	tplExt := ev.MobTemplateExternalID
	if tplExt == "" {
		// Pre-#31 publish path or non-mob death — engine has nothing
		// to do without the template id.
		return
	}
	if err := e.handleKill(ctx, ev.Killer.ID, tplExt); err != nil {
		slog.Warn("quest engine: handle kill",
			"char", ev.Killer.ID, "tpl", tplExt, "error", err)
	}
}

func (e *Engine) onPlayerEntered(ctx context.Context, ev world.PlayerEntered) {
	if ev.CharacterID == 0 || ev.ToRoomID == 0 {
		return
	}
	if err := e.handleEnter(ctx, ev.CharacterID, ev.ToRoomID); err != nil {
		slog.Warn("quest engine: handle enter",
			"char", ev.CharacterID, "room", ev.ToRoomID, "error", err)
	}
}

func (e *Engine) handleKill(ctx context.Context, charID int64, victimTplExternalID string) error {
	char, err := e.chars.GetByID(ctx, charID)
	if err != nil {
		return fmt.Errorf("get character: %w", err)
	}
	for i, p := range char.QuestLog {
		if !p.CompletedAt.IsZero() {
			continue
		}
		q, ok := e.cat.Get(p.QuestID)
		if !ok {
			continue
		}
		if int(p.StepIndex) >= len(q.Steps) {
			continue
		}
		step := q.Steps[p.StepIndex]
		if step.Kind != StepKillN || step.Mob != victimTplExternalID {
			continue
		}
		var st killNState
		_ = json.Unmarshal([]byte(p.StateJSON), &st)
		if st.Remaining <= 0 {
			// Already met but transition didn't fire — defensive: advance now.
			return e.advanceStep(ctx, char, q, i)
		}
		st.Remaining--
		if st.Remaining > 0 {
			b, _ := json.Marshal(st)
			char.QuestLog[i].StateJSON = string(b)
			if err := e.chars.RecordQuestProgress(ctx, charID, char.QuestLog); err != nil {
				return fmt.Errorf("record kill_n decrement: %w", err)
			}
			e.notifyKillProgress(charID, q, st.Remaining)
			return nil
		}
		// Hit zero — advance.
		return e.advanceStep(ctx, char, q, i)
	}
	return nil
}

func (e *Engine) handleEnter(ctx context.Context, charID, toRoomID int64) error {
	char, err := e.chars.GetByID(ctx, charID)
	if err != nil {
		return fmt.Errorf("get character: %w", err)
	}
	// Resolve the room's ExternalID once if any active quest has a
	// reach_room step (lazy: skip the lookup if no candidate matches).
	var roomExt string
	for i, p := range char.QuestLog {
		if !p.CompletedAt.IsZero() {
			continue
		}
		q, ok := e.cat.Get(p.QuestID)
		if !ok {
			continue
		}
		if int(p.StepIndex) >= len(q.Steps) {
			continue
		}
		step := q.Steps[p.StepIndex]
		if step.Kind != StepReachRoom {
			continue
		}
		if roomExt == "" {
			r, err := e.rooms.FindByID(ctx, toRoomID)
			if errors.Is(err, repo.ErrRoomNotFound) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("find room: %w", err)
			}
			roomExt = r.ExternalID
		}
		if step.Room != roomExt {
			continue
		}
		return e.advanceStep(ctx, char, q, i)
	}
	return nil
}

// advanceStep moves the QuestLog[idx] entry to the next step; if
// already on the final step, completes the quest and grants rewards.
// Caller passes the in-memory char + quest so we don't re-fetch.
func (e *Engine) advanceStep(ctx context.Context, char repo.Character, q *Quest, idx int) error {
	curStep := int(char.QuestLog[idx].StepIndex)
	if curStep+1 >= len(q.Steps) {
		return e.completeQuest(ctx, char, q, idx)
	}
	char.QuestLog[idx].StepIndex = int16(curStep + 1)
	state, _ := stepInitState(q.Steps[curStep+1])
	char.QuestLog[idx].StateJSON = state
	if err := e.chars.RecordQuestProgress(ctx, char.ID, char.QuestLog); err != nil {
		return fmt.Errorf("record step advance: %w", err)
	}
	e.notifyStepAdvanced(char.ID, q, q.Steps[curStep+1])
	return nil
}

func (e *Engine) completeQuest(ctx context.Context, char repo.Character, q *Quest, idx int) error {
	char.QuestLog[idx].CompletedAt = time.Now().UTC()
	char.QuestLog[idx].StateJSON = "{}"
	if err := e.chars.RecordQuestProgress(ctx, char.ID, char.QuestLog); err != nil {
		return fmt.Errorf("record quest complete: %w", err)
	}
	// Reward grants run after the completion is durable so a crash
	// between persist and grant leaves the quest visibly done; on
	// next login the player sees the completion in their log.
	if err := e.grantRewards(ctx, char, q); err != nil {
		slog.Warn("quest reward grant failed",
			"char", char.ID, "quest", q.ID, "error", err)
		// Do not bubble up — the quest is logged complete.
	}
	if e.audits != nil {
		audit.Record(ctx, e.audits, e.lookupSession(char.ID), "quest_complete", q.ID, "")
	}
	e.notifyCompleted(char.ID, q)
	return nil
}

// grantRewards applies XP and coin rewards. XP-debt drain is bypassed
// (quest XP is narrative-locked; combat XP already drains debt). Coin
// is optimistic-lock-aware via a single retry on ErrCoinConflict.
func (e *Engine) grantRewards(ctx context.Context, char repo.Character, q *Quest) error {
	if q.Rewards.XP > 0 {
		if err := e.chars.RecordXP(ctx, char.ID, char.XP+q.Rewards.XP); err != nil {
			return fmt.Errorf("record xp: %w", err)
		}
	}
	if q.Rewards.Copper > 0 {
		if err := e.grantCoin(ctx, char, q.Rewards.Copper); err != nil {
			return fmt.Errorf("grant coin: %w", err)
		}
	}
	return nil
}

func (e *Engine) grantCoin(ctx context.Context, char repo.Character, copper int64) error {
	newCoin := char.Coin + currency.Amount(copper)
	err := e.chars.RecordCoin(ctx, char.ID, newCoin, char.BankBalance, char.CoinVersion)
	if !errors.Is(err, repo.ErrCoinConflict) {
		return err
	}
	// Optimistic-lock retry: re-read and try once. The quest is
	// already marked complete by the time we get here, so a
	// double-conflict means the player loses the coin reward — log
	// it loudly so the loss is observable in operations. A retry
	// queue (re-grant on next login or via an admin reconciler) is
	// V2 work.
	fresh, err2 := e.chars.GetByID(ctx, char.ID)
	if err2 != nil {
		return fmt.Errorf("re-read for coin retry: %w", err2)
	}
	newCoin = fresh.Coin + currency.Amount(copper)
	if err := e.chars.RecordCoin(ctx, char.ID, newCoin, fresh.BankBalance, fresh.CoinVersion); err != nil {
		if errors.Is(err, repo.ErrCoinConflict) {
			slog.Warn("quest reward coin lost — double conflict, no retry queue (V1)",
				"char", char.ID, "copper", copper)
		}
		return fmt.Errorf("coin retry: %w", err)
	}
	return nil
}

// --- Helpers -----------------------------------------------------

// findActive returns the (index, true) of the active QuestLog entry
// matching questID, or (-1, false) if absent or already completed.
func findActive(log []creature.QuestProgress, questID string) (int, bool) {
	for i, p := range log {
		if p.QuestID == questID && p.CompletedAt.IsZero() {
			return i, true
		}
	}
	return -1, false
}

// stepInitState returns the per-step state JSON for a freshly entered
// step. kill_n stores remaining count; talk_to and reach_room are
// stateless ("{}").
func stepInitState(s Step) (string, error) {
	switch s.Kind {
	case StepKillN:
		b, err := json.Marshal(killNState{Remaining: s.Count})
		if err != nil {
			return "{}", err
		}
		return string(b), nil
	default:
		return "{}", nil
	}
}

// lookupSession returns the bound session for charID, or nil if the
// character is offline. Engine writes go through this for both
// player-facing notifications and audit-row actor stamping.
func (e *Engine) lookupSession(charID int64) *telnet.Session {
	if e.sessions == nil {
		return nil
	}
	for _, s := range e.sessions.Snapshot() {
		if s != nil && s.CharacterID == charID {
			return s
		}
	}
	return nil
}

// --- Player-facing notifications ---------------------------------
//
// All cross-session writes go through Session.WriteAsync (engine
// runs on the eventbus goroutine — CLAUDE.md "cross-session output"
// rule). Offline players just don't see the message; their next
// `quest` invocation surfaces the new state.

func (e *Engine) notifyAccepted(charID int64, q *Quest) {
	s := e.lookupSession(charID)
	if s == nil {
		return
	}
	step := ""
	if len(q.Steps) > 0 {
		step = q.Steps[0].Prompt
	}
	_ = s.WriteAsync(fmt.Sprintf("\r\n[Quest accepted] %s\r\n  → %s\r\n",
		strings.ReplaceAll(q.Name, "{{", "{ {"), strings.ReplaceAll(step, "{{", "{ {")))
}

func (e *Engine) notifyStepAdvanced(charID int64, q *Quest, next Step) {
	s := e.lookupSession(charID)
	if s == nil {
		return
	}
	_ = s.WriteAsync(fmt.Sprintf("\r\n[Quest step] %s\r\n  → %s\r\n",
		strings.ReplaceAll(q.Name, "{{", "{ {"), strings.ReplaceAll(next.Prompt, "{{", "{ {")))
}

func (e *Engine) notifyKillProgress(charID int64, q *Quest, remaining int) {
	s := e.lookupSession(charID)
	if s == nil {
		return
	}
	_ = s.WriteAsync(fmt.Sprintf("\r\n[Quest progress] %s — %d remaining.\r\n",
		strings.ReplaceAll(q.Name, "{{", "{ {"), remaining))
}

func (e *Engine) notifyCompleted(charID int64, q *Quest) {
	s := e.lookupSession(charID)
	if s == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\r\n[Quest complete] %s\r\n",
		strings.ReplaceAll(q.Name, "{{", "{ {"))
	if q.Rewards.XP > 0 {
		fmt.Fprintf(&b, "  → %d XP\r\n", q.Rewards.XP)
	}
	if q.Rewards.Copper > 0 {
		fmt.Fprintf(&b, "  → %s\r\n", currency.Amount(q.Rewards.Copper).Short())
	}
	_ = s.WriteAsync(b.String())
}
