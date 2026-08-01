package coverage

import (
	"sort"

	"secha/internal/experiment"
	"secha/internal/indicator"
)

type Status int

const (
	Blocked Status = iota
	DetectedOnly
	Undetected
)

func (s Status) String() string {
	switch s {
	case Blocked:
		return "blocked"
	case DetectedOnly:
		return "detected"
	default:
		return "undetected"
	}
}

func (s Status) Score() int {
	switch s {
	case Blocked:
		return 100
	case DetectedOnly:
		return 50
	default:
		return 0
	}
}

type Technique struct {
	ID            string
	Experiments   []string
	Components    []string
	Runs          int
	ExposedRate   float64
	DetectedRate  float64
	PreventedRate float64
	MTTD          *indicator.Stats
}

func (t Technique) Status() Status {
	switch {
	case t.ExposedRate == 0:
		return Blocked
	case t.DetectedRate > 0:
		return DetectedOnly
	default:
		return Undetected
	}
}

type Result struct {
	Attack   []Technique
	D3fend   []Technique
	Unmapped []string
}

type Observation struct {
	Experiment *experiment.Experiment
	Reports    []*indicator.Report
}

func Build(observations []Observation) Result {
	attack := map[string]*accumulator{}
	d3fend := map[string]*accumulator{}
	var unmapped []string

	for _, o := range observations {
		annotated := false
		for label, spec := range o.Experiment.AnnotatedSpecs() {
			for _, id := range spec.Attack {
				annotated = true
				acc(attack, id).add(o, label)
			}
			for _, id := range spec.D3fend {
				annotated = true
				acc(d3fend, id).add(o, label)
			}
		}
		if !annotated {
			unmapped = append(unmapped, o.Experiment.Metadata.Name)
		}
	}

	sort.Strings(unmapped)
	return Result{
		Attack:   finalise(attack),
		D3fend:   finalise(d3fend),
		Unmapped: unmapped,
	}
}

type accumulator struct {
	id          string
	experiments map[string]bool
	components  map[string]bool
	runs        int
	exposed     int
	detected    int
	prevented   int
	mttdMs      []float64
}

func acc(m map[string]*accumulator, id string) *accumulator {
	if _, ok := m[id]; !ok {
		m[id] = &accumulator{
			id:          id,
			experiments: map[string]bool{},
			components:  map[string]bool{},
		}
	}
	return m[id]
}

func (a *accumulator) add(o Observation, component string) {
	a.experiments[o.Experiment.Metadata.Name] = true
	a.components[component] = true
	for _, r := range o.Reports {
		a.runs++
		if r.DataExposed {
			a.exposed++
		}
		if r.Detected {
			a.detected++
		}
		if !r.FaultApplied {
			a.prevented++
		}
		if r.MTTD != nil {
			a.mttdMs = append(a.mttdMs, float64(r.MTTD.Nanoseconds())/1e6)
		}
	}
}

func finalise(m map[string]*accumulator) []Technique {
	out := make([]Technique, 0, len(m))
	for _, a := range m {
		t := Technique{
			ID:          a.id,
			Experiments: keys(a.experiments),
			Components:  keys(a.components),
			Runs:        a.runs,
		}
		if a.runs > 0 {
			n := float64(a.runs)
			t.ExposedRate = float64(a.exposed) / n
			t.DetectedRate = float64(a.detected) / n
			t.PreventedRate = float64(a.prevented) / n
		}
		if len(a.mttdMs) > 0 {
			s := indicator.StatsOf(a.mttdMs)
			t.MTTD = &s
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
