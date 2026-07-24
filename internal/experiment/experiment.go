package experiment

import (
	"encoding/json"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

type Experiment struct {
	APIVersion  string      `json:"apiVersion"`
	Kind        string      `json:"kind"`
	Metadata    Metadata    `json:"metadata"`
	SteadyState SteadyState `json:"steadyState"`
	Fault       Spec        `json:"fault"`
	Probe       Spec        `json:"probe"`
	Observe     Observe     `json:"observe"`
	Indicators  []string    `json:"indicators"`
}

type Metadata struct {
	Name       string `json:"name"`
	Hypothesis string `json:"hypothesis"`
}

type SteadyState struct {
	Checks []Check `json:"checks"`
}

type Check struct {
	Probe  Spec
	Expect Expect
}

type Expect struct {
	Reachable   *bool `json:"reachable"`
	DataExposed *bool `json:"dataExposed"`
}

func (c *Check) UnmarshalJSON(b []byte) error {
	if err := c.Probe.UnmarshalJSON(b); err != nil {
		return err
	}
	var wrapper struct {
		Expect Expect `json:"expect"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return err
	}
	c.Expect = wrapper.Expect
	return nil
}

func (e Expect) IsEmpty() bool {
	return e.Reachable == nil && e.DataExposed == nil
}

type Observe struct {
	Window    string `json:"window"`
	Detectors []Spec `json:"detectors"`
}

type Spec struct {
	Type string
	Raw  json.RawMessage
}

func (s *Spec) UnmarshalJSON(b []byte) error {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return err
	}
	s.Type = head.Type
	s.Raw = append(json.RawMessage(nil), b...)
	return nil
}

func (s Spec) Decode(into any) error {
	return json.Unmarshal(s.Raw, into)
}

func Load(path string) (*Experiment, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read experiment: %w", err)
	}
	var exp Experiment
	if err := yaml.Unmarshal(b, &exp); err != nil {
		return nil, fmt.Errorf("parse experiment: %w", err)
	}
	if err := exp.Validate(); err != nil {
		return nil, err
	}
	return &exp, nil
}

func (e *Experiment) Validate() error {
	if e.Kind != "Experiment" {
		return fmt.Errorf("unsupported kind %q, want Experiment", e.Kind)
	}
	if e.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if e.Fault.Type == "" {
		return fmt.Errorf("fault.type is required")
	}
	if e.Probe.Type == "" {
		return fmt.Errorf("probe.type is required")
	}
	for i, c := range e.SteadyState.Checks {
		if c.Probe.Type == "" {
			return fmt.Errorf("steadyState.checks[%d].type is required", i)
		}
		if c.Expect.IsEmpty() {
			return fmt.Errorf("steadyState.checks[%d] (%s): expect must set reachable and/or dataExposed",
				i, c.Probe.Type)
		}
	}
	return nil
}
