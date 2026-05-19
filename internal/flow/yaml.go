package flow

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// rawFlow is the wire-format struct that mirrors a flow YAML file
// 1:1. The loader unmarshals into this, then projects to the
// engine-facing `*Flow` (with rawStep wrappers replaced by their
// concrete Step impls). Keeping rawFlow distinct from Flow keeps
// the YAML schema decoupled from the runtime API — a future schema
// change (kind aliasing, default fields, etc.) lands here without
// touching the engine.
type rawFlow struct {
	ID        string    `yaml:"id"`
	Entry     StepID    `yaml:"entry"`
	Resumable bool      `yaml:"resumable"`
	Steps     []rawStep `yaml:"steps"`
}

// rawStep is the tagged-union wrapper. UnmarshalYAML reads the `kind`
// field first, then re-decodes the same node into the matching Step
// impl. New step kinds extend the switch and require nothing else.
type rawStep struct {
	Step Step
}

// UnmarshalYAML implements yaml.Unmarshaler. Reads the `kind` field
// and dispatches the second decode pass into the concrete step type.
// Unknown kinds fail loudly with the offending value in the error
// message; missing kind is treated as "unknown" (the empty string).
func (r *rawStep) UnmarshalYAML(node *yaml.Node) error {
	var probe struct {
		Kind string `yaml:"kind"`
	}
	if err := node.Decode(&probe); err != nil {
		return fmt.Errorf("decode kind probe: %w", err)
	}
	switch probe.Kind {
	case "text":
		var s TextStep
		if err := node.Decode(&s); err != nil {
			return fmt.Errorf("decode text step: %w", err)
		}
		r.Step = &s
	case "choice":
		var s ChoiceStep
		if err := node.Decode(&s); err != nil {
			return fmt.Errorf("decode choice step: %w", err)
		}
		r.Step = &s
	case "confirm":
		var s ConfirmStep
		if err := node.Decode(&s); err != nil {
			return fmt.Errorf("decode confirm step: %w", err)
		}
		r.Step = &s
	case "":
		return fmt.Errorf("step missing required `kind:` field")
	default:
		return fmt.Errorf("unknown step kind %q (want one of: text, choice, confirm)", probe.Kind)
	}
	return nil
}
