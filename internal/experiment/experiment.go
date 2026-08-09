package experiment

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"sigs.k8s.io/yaml"

	"github.com/Thran34/secha/internal/safety"
)

type Experiment struct {
	APIVersion  string        `json:"apiVersion"`
	Kind        string        `json:"kind"`
	Metadata    Metadata      `json:"metadata"`
	Safety      safety.Policy `json:"safety"`
	SteadyState SteadyState   `json:"steadyState"`
	Fault       Spec          `json:"fault"`
	Probe       Spec          `json:"probe"`
	Observe     Observe       `json:"observe"`
	Indicators  []string      `json:"indicators"`
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
	Type   string
	Attack []string
	D3fend []string
	Raw    json.RawMessage
}

func (s *Spec) UnmarshalJSON(b []byte) error {
	var head struct {
		Type   string   `json:"type"`
		Attack []string `json:"attack"`
		D3fend []string `json:"d3fend"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return err
	}
	s.Type = head.Type
	s.Attack = head.Attack
	s.D3fend = head.D3fend
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
	for label, s := range e.AnnotatedSpecs() {
		if err := s.validateReferences(label); err != nil {
			return err
		}
	}
	return nil
}

func (e *Experiment) AnnotatedSpecs() map[string]Spec {
	specs := map[string]Spec{
		"fault": e.Fault,
		"probe": e.Probe,
	}
	for i, d := range e.Observe.Detectors {
		specs[fmt.Sprintf("observe.detectors[%d]", i)] = d
	}
	return specs
}

var (
	attackID = regexp.MustCompile(`^T\d{4}(\.\d{3})?$`)
	d3fendID = regexp.MustCompile(`^D3-[A-Za-z]+$`)
)

func (s Spec) validateReferences(label string) error {
	for _, id := range s.Attack {
		if !attackID.MatchString(id) {
			return fmt.Errorf("%s: %q is not an ATT&CK technique id (want T1234 or T1234.005)", label, id)
		}
	}
	for _, id := range s.D3fend {
		if !d3fendID.MatchString(id) {
			return fmt.Errorf("%s: %q is not a D3FEND technique id (want D3-XXX)", label, id)
		}
	}
	return nil
}
