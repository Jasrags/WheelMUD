package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/dialogue"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// errNoDialogueHere is the sentinel for "no NPC with a dialogue tree
// in the current room". Callers translate it into a player-facing
// line.
var errNoDialogueHere = errors.New("cmd: no dialogue partner here")

// resolvedDialogue pairs a decoded dialogue.Tree with the mob in the
// room that owns it.
type resolvedDialogue struct {
	tree          *dialogue.Tree
	keeper        creature.MobInstance
	tplName       string
	tplExternalID string
}

// PushDialogueFn is the closure NewTalk takes for "build and push the
// Dialogue mode against this tree". main.go provides it; the package
// doesn't import internal/mode directly to keep the import graph
// acyclic (mode → cmd would otherwise close a cycle through chargen).
//
// npcExternalID is the resolved mob_template ExternalID — passed
// through so the dialogue mode's quest hooks (Phase F #31) can match
// the active step's Mob field against the conversation partner.
type PushDialogueFn func(s *telnet.Session, npcName, npcExternalID string, tree *dialogue.Tree) error

// NewTalk wires the `talk <mob>` verb. Resolves a target NPC in the
// player's current room, decodes its dialogue tree, validates as
// defense-in-depth, and hands off to pushDialogue (which builds the
// Dialogue mode and pushes it onto the session's mode stack).
//
// Refusal paths emit a single line and do not log noise:
//   - no target arg                 → usage hint
//   - no matching mob in the room   → "X isn't here."
//   - mob has no dialogue_json      → "<mob> has nothing to say."
//   - corrupt JSON / invalid tree   → "<mob> mumbles something incoherent."
func NewTalk(mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	pushDialogue PushDialogueFn,
) *telnet.Command {
	return &telnet.Command{
		Name:    "talk",
		Help:    "Talk — speak with an NPC who has a dialogue tree",
		Long:    "Usage: talk <mob>\n\nLeave conversation with 'bye'.",
		MinArgs: 1,
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			target := strings.TrimSpace(strings.Join(c.Args, " "))
			if target == "" {
				return s.WriteString("{{Talk to whom?}}::yellow\r\n")
			}

			res, err := findDialoguePartner(c.Ctx, s.CurrentRoomID, target, mobs, templates)
			// target is raw player input; tplName is mob YAML data —
			// both untrusted from the cfmt renderer's perspective.
			// defangCfmt neutralizes `{{...}}::style` injection.
			switch {
			case errors.Is(err, errNoDialogueHere):
				return s.WriteString(fmt.Sprintf("{{%s isn't here.}}::yellow\r\n", defangCfmt(target)))
			case errors.Is(err, errNoDialogueTree):
				return s.WriteString(fmt.Sprintf("{{%s has nothing to say.}}::yellow\r\n", defangCfmt(res.tplName)))
			case errors.Is(err, errBadDialogueTree):
				slog.Warn("talk: invalid dialogue tree",
					"mob", res.keeper.ID, "template", res.keeper.TemplateID, "error", err)
				return s.WriteString(fmt.Sprintf("{{%s mumbles something incoherent.}}::red\r\n", defangCfmt(res.tplName)))
			case err != nil:
				slog.Error("talk: lookup", "error", err)
				return s.WriteString("{{They don't seem inclined to speak.}}::red\r\n")
			}

			if pushDialogue == nil {
				slog.Error("talk: pushDialogue unbound")
				return s.WriteString("{{They don't seem inclined to speak.}}::red\r\n")
			}
			if err := pushDialogue(s, res.tplName, res.tplExternalID, res.tree); err != nil {
				slog.Error("talk: push dialogue mode",
					"mob", res.keeper.ID, "error", err)
				return s.WriteString("{{They don't seem inclined to speak.}}::red\r\n")
			}
			return nil
		},
	}
}

// errNoDialogueTree means the mob template has no dialogue_json
// attached. errBadDialogueTree means the JSON decoded but failed
// validation (or didn't decode at all). Both surface to the player as
// terse refusals; only the bad-tree case logs at warn level.
var (
	errNoDialogueTree  = errors.New("cmd: mob has no dialogue tree")
	errBadDialogueTree = errors.New("cmd: dialogue tree invalid")
)

// findDialoguePartner resolves the target keyword against the mobs in
// roomID, fetches the matching template, and decodes its dialogue
// JSON. The returned resolvedDialogue has a populated tplName even on
// the errNoDialogueTree / errBadDialogueTree refusals so the caller
// can render a name-aware message.
func findDialoguePartner(ctx context.Context, roomID int64, target string,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
) (resolvedDialogue, error) {
	if roomID == 0 {
		return resolvedDialogue{}, errNoDialogueHere
	}
	occupants, err := mobs.ListInRoom(ctx, roomID)
	if err != nil {
		return resolvedDialogue{}, fmt.Errorf("list room mobs: %w", err)
	}
	mob, ok := MatchMob(target, occupants)
	if !ok {
		return resolvedDialogue{}, errNoDialogueHere
	}
	tpl, err := templates.GetByID(ctx, mob.TemplateID)
	if err != nil {
		return resolvedDialogue{}, fmt.Errorf("get template: %w", err)
	}
	res := resolvedDialogue{keeper: mob, tplName: tpl.Core.Name, tplExternalID: tpl.ExternalID}
	if len(tpl.DialogueJSON) == 0 {
		return res, errNoDialogueTree
	}
	var tree dialogue.Tree
	if err := json.Unmarshal(tpl.DialogueJSON, &tree); err != nil {
		return res, fmt.Errorf("%w: %v", errBadDialogueTree, err)
	}
	if err := dialogue.Validate(&tree); err != nil {
		return res, fmt.Errorf("%w: %v", errBadDialogueTree, err)
	}
	res.tree = &tree
	return res, nil
}
