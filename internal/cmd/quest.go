package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/quest"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewQuest wires the `quest` / `quests` verb. Subcommands:
//
//	quest                — list active + completed entries
//	quest info <id>      — render the quest's full step list with progress
//	quest abandon <id>   — drop an active entry (no-op if not on it)
//
// Player-facing only. Audited only on `abandon`.
func NewQuest(chars repo.CharacterRepo, cat *quest.Catalog,
	engine *quest.Engine, audits repo.AdminAuditRepo,
) *telnet.Command {
	return &telnet.Command{
		Name:    "quest",
		Aliases: []string{"quests"},
		Help:    "Quest — show your active and completed quests",
		Long: "Usage: quest\n" +
			"       quest info <id>\n" +
			"       quest abandon <id>",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			char, err := chars.GetByID(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("quest: char lookup", "char", s.CharacterID, "error", err)
				return s.WriteString("{{Your records are unavailable.}}::red\r\n")
			}

			if len(c.Args) == 0 {
				return renderQuestList(s, char, cat)
			}
			sub := strings.ToLower(c.Args[0])
			rest := strings.TrimSpace(strings.Join(c.Args[1:], " "))
			switch sub {
			case "info":
				if rest == "" {
					return s.WriteString("{{Usage: quest info <id>}}::yellow\r\n")
				}
				return renderQuestInfo(s, char, cat, rest)
			case "abandon":
				if rest == "" {
					return s.WriteString("{{Usage: quest abandon <id>}}::yellow\r\n")
				}
				return abandonQuest(c, engine, char, rest, audits)
			default:
				return s.WriteString(fmt.Sprintf(
					"{{Unknown subcommand %q. Use: quest, quest info <id>, quest abandon <id>.}}::yellow\r\n",
					defangCfmt(sub)))
			}
		},
	}
}

func renderQuestList(s *telnet.Session, char repo.Character, cat *quest.Catalog) error {
	if len(char.QuestLog) == 0 {
		return s.WriteString("{{You have no active quests.}}::gray\r\n")
	}
	var b strings.Builder
	var active, done []creature.QuestProgress
	for _, p := range char.QuestLog {
		if p.CompletedAt.IsZero() {
			active = append(active, p)
		} else {
			done = append(done, p)
		}
	}
	if len(active) > 0 {
		b.WriteString("{{Active quests:}}::cyan\r\n")
		for _, p := range active {
			q, ok := cat.Get(p.QuestID)
			if !ok {
				fmt.Fprintf(&b, "  %s {{(unknown quest)}}::red\r\n", defangCfmt(p.QuestID))
				continue
			}
			step := ""
			if int(p.StepIndex) < len(q.Steps) {
				step = q.Steps[p.StepIndex].Prompt
			}
			fmt.Fprintf(&b, "  {{%s}}::white — %s\r\n",
				defangCfmt(q.Name), defangCfmt(step))
			if int(p.StepIndex) < len(q.Steps) && q.Steps[p.StepIndex].Kind == quest.StepKillN {
				if remaining := killRemaining(p.StateJSON); remaining > 0 {
					fmt.Fprintf(&b, "    {{%d remaining}}::gray\r\n", remaining)
				}
			}
		}
	}
	if len(done) > 0 {
		b.WriteString("{{Completed:}}::gray\r\n")
		for _, p := range done {
			q, ok := cat.Get(p.QuestID)
			if !ok {
				fmt.Fprintf(&b, "  %s\r\n", defangCfmt(p.QuestID))
				continue
			}
			fmt.Fprintf(&b, "  {{%s}}::gray\r\n", defangCfmt(q.Name))
		}
	}
	return s.WritePagedWrapped(b.String())
}

func renderQuestInfo(s *telnet.Session, char repo.Character, cat *quest.Catalog, id string) error {
	q, ok := cat.Get(id)
	if !ok {
		return s.WriteString(fmt.Sprintf("{{No such quest: %s}}::yellow\r\n", defangCfmt(id)))
	}
	// Locate the player's progress (if any) so we can render markers.
	var progress *creature.QuestProgress
	for i, p := range char.QuestLog {
		if p.QuestID == id {
			progress = &char.QuestLog[i]
			break
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "{{%s}}::cyan|bold\r\n", defangCfmt(q.Name))
	if q.Summary != "" {
		fmt.Fprintf(&b, "  %s\r\n", defangCfmt(q.Summary))
	}
	b.WriteString("\r\n")
	for i, st := range q.Steps {
		marker := "·"
		switch {
		case progress != nil && !progress.CompletedAt.IsZero():
			marker = "✓" // entire quest done
		case progress != nil && int(progress.StepIndex) > i:
			marker = "✓"
		case progress != nil && int(progress.StepIndex) == i:
			marker = "▸"
		}
		fmt.Fprintf(&b, "  %s %s\r\n", marker, defangCfmt(st.Prompt))
		if progress != nil && int(progress.StepIndex) == i && st.Kind == quest.StepKillN {
			if remaining := killRemaining(progress.StateJSON); remaining > 0 {
				fmt.Fprintf(&b, "    {{%d remaining}}::gray\r\n", remaining)
			}
		}
	}
	if q.Rewards.XP > 0 || q.Rewards.Copper > 0 {
		b.WriteString("\r\n{{Rewards:}}::yellow\r\n")
		if q.Rewards.XP > 0 {
			fmt.Fprintf(&b, "  %d XP\r\n", q.Rewards.XP)
		}
		if q.Rewards.Copper > 0 {
			fmt.Fprintf(&b, "  %d copper\r\n", q.Rewards.Copper)
		}
	}
	return s.WritePagedWrapped(b.String())
}

func abandonQuest(c *telnet.Context, engine *quest.Engine, char repo.Character, id string, audits repo.AdminAuditRepo) error {
	s := c.Session
	if engine == nil {
		return s.WriteString("{{The quest log is read-only right now.}}::red\r\n")
	}
	// Confirm the player actually has an active entry before audit.
	on := false
	for _, p := range char.QuestLog {
		if p.QuestID == id && p.CompletedAt.IsZero() {
			on = true
			break
		}
	}
	if !on {
		return s.WriteString(fmt.Sprintf("{{You aren't on %s.}}::yellow\r\n", defangCfmt(id)))
	}
	if err := engine.AbandonQuest(c.Ctx, char.ID, id); err != nil {
		slog.Warn("quest abandon", "char", char.ID, "quest", id, "error", err)
		return s.WriteString("{{Couldn't abandon that quest.}}::red\r\n")
	}
	audit.Record(c.Ctx, audits, s, "quest_abandon", id, "")
	return s.WriteString(fmt.Sprintf("{{You abandon %s.}}::gray\r\n", defangCfmt(id)))
}

// killRemaining decodes a kill_n StateJSON without importing quest's
// internal type. Returns 0 on parse failure (treated as "completed" for
// rendering — the actual transition runs separately).
func killRemaining(stateJSON string) int {
	var st struct {
		Remaining int `json:"remaining"`
	}
	if stateJSON == "" {
		return 0
	}
	_ = json.Unmarshal([]byte(stateJSON), &st)
	return st.Remaining
}
