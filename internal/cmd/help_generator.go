package cmd

import (
	"strings"

	"github.com/Jasrags/WheelMUD/internal/help"
	"github.com/Jasrags/WheelMUD/telnet"
)

// GenerateCommandTopics walks the command registry and produces one
// help.Topic per registered command, drawing on the existing
// Command.Help (one-line summary) and Command.Long (optional multi-line
// body) fields. Aliases become topic keywords so `help <alias>`
// resolves through the same lookup pipeline as `help <name>`.
//
// Commands that have neither Help nor Long are skipped — emitting a
// stub topic with just "(command)" in the title would clutter the
// catalog without adding signal.
//
// The returned slice is intended to be merged into the help catalog
// via Catalog.MergeGenerated, which honors authored topics over
// generated ones on ID collision. So `help combat` will still resolve
// to the hand-written combat article rather than a thin per-command
// auto-stub (combat itself isn't a verb today, but the principle
// holds for any verb whose authored article exists).
func GenerateCommandTopics(r *telnet.Registry) []*help.Topic {
	if r == nil {
		return nil
	}
	cmds := r.All()
	out := make([]*help.Topic, 0, len(cmds))
	for _, c := range cmds {
		if c == nil {
			continue
		}
		summary := strings.TrimSpace(c.Help)
		long := strings.TrimSpace(c.Long)
		if summary == "" && long == "" {
			continue
		}
		body := long
		if body == "" {
			body = summary
		}
		// Keywords: aliases only. The topic's own ID (== c.Name) is
		// implicit through Catalog.byID and shouldn't be duplicated
		// into byKeyword.
		var keywords []string
		if len(c.Aliases) > 0 {
			keywords = make([]string, 0, len(c.Aliases))
			for _, a := range c.Aliases {
				a = strings.ToLower(strings.TrimSpace(a))
				if a != "" && a != c.Name {
					keywords = append(keywords, a)
				}
			}
		}
		out = append(out, generatedTopic(c.Name, summary, body, keywords))
	}
	return out
}

// generatedTopic builds a help.Topic with the canonical shape used by
// GenerateCommandTopics. Split out so a future caller (e.g. a pack
// loader that wants to emit synthetic topics for its own verbs) can
// reuse the title convention.
func generatedTopic(name, summary, body string, keywords []string) *help.Topic {
	title := name
	if summary != "" {
		title = name + " — " + summary
	}
	return &help.Topic{
		ID:       name,
		Title:    title,
		Keywords: keywords,
		Body:     body,
	}
}
