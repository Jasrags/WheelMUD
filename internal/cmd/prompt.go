package cmd

import (
	"strings"

	"github.com/Jasrags/WheelMUD/internal/prompt"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// NewPrompt returns the `prompt` command — show / set / clear / help
// the session character's prompt template.
//
//	prompt                    — show current effective template
//	prompt show               — alias of bare prompt
//	prompt help               — placeholder + color reference
//	prompt set <template>     — persist a per-character override
//	prompt clear              — revert to the server default
//	prompt reset              — alias of prompt clear
//
// The template grammar comes from internal/prompt: `%h/%H/%r/%g/%%/...`
// Color is supported via cfmt tags (`{{...}}::red`); Game.Prompt runs
// the rendered output through cfmt.Sprint before write.
func NewPrompt(characters repo.CharacterRepo, serverDefault string) *telnet.Command {
	return &telnet.Command{
		Name: "prompt",
		Help: "Show, set, clear, or get help on your prompt template",
		Long: promptHelpBody,
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			if c.Session.CharacterID == 0 {
				return c.Session.WriteRaw([]byte("Prompt unavailable on this session.\r\n"))
			}
			if len(c.Args) == 0 || strings.EqualFold(c.Args[0], "show") {
				return showPrompt(c, characters, serverDefault)
			}
			switch strings.ToLower(c.Args[0]) {
			case "help":
				return c.Session.WriteRaw([]byte(promptHelpBody + "\r\n"))
			case "set":
				tmpl := strings.TrimSpace(strings.Join(c.Args[1:], " "))
				return setPrompt(c, characters, tmpl)
			case "clear", "reset":
				return clearPrompt(c, characters)
			default:
				return c.Session.WriteRaw([]byte("Usage: prompt [show|help|set <template>|clear|reset]\r\n"))
			}
		},
	}
}

const promptHelpBody = "Usage:\r\n" +
	"  prompt                  show your current prompt template\r\n" +
	"  prompt help             show this reference\r\n" +
	"  prompt set <template>   set a per-character override\r\n" +
	"  prompt clear            revert to the server default\r\n" +
	"  prompt reset            alias of prompt clear\r\n" +
	"\r\n" +
	"Placeholders:\r\n" +
	"  %h / %H   current / max hit points\r\n" +
	"  %m / %M   mana   (reserved — renders 0 today)\r\n" +
	"  %v / %V   move   (reserved — renders 0 today)\r\n" +
	"  %r        current room name\r\n" +
	"  %g        carried coin (e.g. \"5gc 2sp\")\r\n" +
	"  %t        combat target (reserved — empty today)\r\n" +
	"  %%        literal '%'\r\n" +
	"\r\n" +
	"Color via cfmt tags: {{red text}}::red, {{bold}}::bold, etc.\r\n" +
	"Example:\r\n" +
	"  prompt set [{{%h}}::red/%H] %r$ "

func showPrompt(c *telnet.Context, characters repo.CharacterRepo, serverDefault string) error {
	ch, err := characters.FindByName(c.Ctx, c.Session.CharacterName)
	if err != nil {
		return c.Session.WriteRaw([]byte("Could not load your character.\r\n"))
	}
	if ch.PromptTemplate == "" {
		return c.Session.WriteRaw([]byte("Prompt: (server default) " + serverDefault + "\r\n"))
	}
	return c.Session.WriteRaw([]byte("Prompt: " + ch.PromptTemplate + "\r\n"))
}

func setPrompt(c *telnet.Context, characters repo.CharacterRepo, tmpl string) error {
	clean, ok := prompt.SanitizeTemplate(tmpl)
	if !ok {
		return c.Session.WriteRaw([]byte("Invalid template (empty, too long, or contains control characters).\r\n"))
	}
	if err := characters.RecordPromptTemplate(c.Ctx, c.Session.CharacterID, clean); err != nil {
		return c.Session.WriteRaw([]byte("Could not save prompt.\r\n"))
	}
	return c.Session.WriteRaw([]byte("Prompt updated.\r\n"))
}

func clearPrompt(c *telnet.Context, characters repo.CharacterRepo) error {
	if err := characters.RecordPromptTemplate(c.Ctx, c.Session.CharacterID, ""); err != nil {
		return c.Session.WriteRaw([]byte("Could not clear prompt.\r\n"))
	}
	return c.Session.WriteRaw([]byte("Reverted to server default.\r\n"))
}

