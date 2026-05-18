// Package emote houses the YAML-driven social-verb catalog (§M.3).
//
// A "social" is a short physical-action verb (smile, wave, bow, ...)
// authored once in YAML and registered as a real telnet command at
// boot. Each social has up to five lines:
//
//   - Self        — what the actor sees when invoking the verb alone
//   - Other       — what other peers in the room see
//   - TargetSelf  — what the actor sees when targeting someone
//   - TargetView  — what the target sees
//   - TargetOther — what bystanders see
//
// Targeted forms are optional: a social with all three blank is
// untargeted-only and refuses a target argument at dispatch time.
//
// Wizinvis (visibility.CanSee) applies to socials exactly as it does
// to say/shout — a hidden admin's social is invisible to non-admin
// peers in the room. The room `silent` flag does NOT gag socials;
// it covers speech only (physical pantomime still carries).
package emote

import (
	"fmt"
	"strings"
)

// Social is one entry in the catalog. ID is the canonical command
// name and must match `[a-z][a-z0-9_]*`. Aliases register as
// additional command names (case-insensitive collision check inside
// the catalog; cross-catalog uniqueness is enforced when the entries
// are handed to telnet.Registry.Register at boot).
type Social struct {
	ID          string   `yaml:"id"`
	Aliases     []string `yaml:"aliases"`
	Help        string   `yaml:"help"`
	Self        string   `yaml:"self"`
	Other       string   `yaml:"other"`
	TargetSelf  string   `yaml:"target_self"`
	TargetView  string   `yaml:"target_view"`
	TargetOther string   `yaml:"target_other"`
}

// Targetable reports whether the social accepts an optional target
// argument. True iff all three targeted templates are non-empty.
func (s Social) Targetable() bool {
	return s.TargetSelf != "" && s.TargetView != "" && s.TargetOther != ""
}

// RenderSelf returns the actor-perspective line for an untargeted
// invocation, terminated with CRLF.
func (s Social) RenderSelf(actor string) string {
	return socialLine(expand(s.Self, actor, ""))
}

// RenderOther returns the bystander line for an untargeted invocation.
func (s Social) RenderOther(actor string) string {
	return socialLine(expand(s.Other, actor, ""))
}

// RenderTargetSelf returns the actor's line when targeting `target`.
func (s Social) RenderTargetSelf(actor, target string) string {
	return socialLine(expand(s.TargetSelf, actor, target))
}

// RenderTargetView returns the target's line when receiving the social.
func (s Social) RenderTargetView(actor, target string) string {
	return socialLine(expand(s.TargetView, actor, target))
}

// RenderTargetOther returns the bystander line for a targeted social.
func (s Social) RenderTargetOther(actor, target string) string {
	return socialLine(expand(s.TargetOther, actor, target))
}

// validate enforces the rules called out in the package doc:
//
//   - non-empty ID matching the verb shape
//   - both Self and Other present
//   - the three Target* fields all set or all blank
//   - every template token is in the {actor,target} whitelist
//   - target-flavored templates are forbidden when the social is
//     untargeted-only (catches a builder typo of putting {target}
//     into Self/Other and never registering the targeted templates)
func (s Social) validate() error {
	if s.ID == "" {
		return fmt.Errorf("blank id")
	}
	if !validID(s.ID) {
		return fmt.Errorf("id %q: must match [a-z][a-z0-9_]*", s.ID)
	}
	for _, a := range s.Aliases {
		if !validID(a) {
			return fmt.Errorf("%s: alias %q must match [a-z][a-z0-9_]*", s.ID, a)
		}
	}
	if strings.TrimSpace(s.Self) == "" {
		return fmt.Errorf("%s: self is required", s.ID)
	}
	if strings.TrimSpace(s.Other) == "" {
		return fmt.Errorf("%s: other is required", s.ID)
	}
	tHaves := 0
	for _, t := range []string{s.TargetSelf, s.TargetView, s.TargetOther} {
		if strings.TrimSpace(t) != "" {
			tHaves++
		}
	}
	if tHaves != 0 && tHaves != 3 {
		return fmt.Errorf("%s: target_self/target_view/target_other must all be set or all blank", s.ID)
	}
	targetable := tHaves == 3
	// Token whitelist + scope rules.
	if err := checkTokens(s.ID, "self", s.Self, false); err != nil {
		return err
	}
	if err := checkTokens(s.ID, "other", s.Other, false); err != nil {
		return err
	}
	if targetable {
		if err := checkTokens(s.ID, "target_self", s.TargetSelf, true); err != nil {
			return err
		}
		if err := checkTokens(s.ID, "target_view", s.TargetView, true); err != nil {
			return err
		}
		if err := checkTokens(s.ID, "target_other", s.TargetOther, true); err != nil {
			return err
		}
	}
	return nil
}

// expand performs the {actor}/{target} substitution. Names are
// trusted (they're CharacterName values which the create flow
// constrains); the sanitization sweep that defangs cfmt tokens
// happens at command-dispatch time on the assembled line.
func expand(tmpl, actor, target string) string {
	out := strings.ReplaceAll(tmpl, "{actor}", actor)
	if target != "" {
		out = strings.ReplaceAll(out, "{target}", target)
	}
	return out
}

// socialLine wraps the rendered body in the canonical magenta colour
// used by every social (distinct from say's cyan and shout's red),
// and appends CRLF.
func socialLine(body string) string {
	return "{{" + body + "}}::magenta\r\n"
}

// checkTokens rejects unknown `{...}` tokens. When allowTarget is
// false, `{target}` is also rejected (used by untargeted slots).
//
// Index math: `j` is the offset of `{` from `i`; `start` is the first
// byte after that `{`; `end` is the offset of the matching `}` from
// `start`. So `tmpl[start:start+end]` is the bare token name and
// `start+end+1` is the byte after `}` — the resume point for the
// next scan. An empty token (`{}`) lands in the default arm and
// fails as "unknown token {}".
func checkTokens(id, field, tmpl string, allowTarget bool) error {
	i := 0
	for i < len(tmpl) {
		j := strings.IndexByte(tmpl[i:], '{')
		if j < 0 {
			return nil
		}
		start := i + j + 1
		end := strings.IndexByte(tmpl[start:], '}')
		if end < 0 {
			return fmt.Errorf("%s: %s has unterminated '{'", id, field)
		}
		token := tmpl[start : start+end]
		switch token {
		case "actor":
		case "target":
			if !allowTarget {
				return fmt.Errorf("%s: %s references {target} but social is untargeted-only", id, field)
			}
		default:
			return fmt.Errorf("%s: %s references unknown token {%s}", id, field, token)
		}
		i = start + end + 1
	}
	return nil
}

// validID enforces the verb shape (matches help-id and registry-name
// conventions). Lowercase ASCII letters, digits, and underscores;
// must start with a letter.
func validID(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		case r == '_' && i > 0:
		default:
			return false
		}
	}
	return true
}
