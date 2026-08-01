package coverage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"secha/internal/experiment"
	"secha/internal/indicator"
)

func exp(name string, fault, probe experiment.Spec) *experiment.Experiment {
	return &experiment.Experiment{
		Metadata: experiment.Metadata{Name: name},
		Fault:    fault,
		Probe:    probe,
	}
}

func spec(typ string, attack, d3fend []string) experiment.Spec {
	return experiment.Spec{Type: typ, Attack: attack, D3fend: d3fend}
}

func dur(d time.Duration) *time.Duration { return &d }

func techniqueByID(t *testing.T, techniques []Technique, id string) Technique {
	t.Helper()
	for _, tech := range techniques {
		if tech.ID == id {
			return tech
		}
	}
	t.Fatalf("technique %q not found in %+v", id, techniques)
	return Technique{}
}

func TestBuildSeparatesAttackFromD3fend(t *testing.T) {
	obs := []Observation{{
		Experiment: exp("e1",
			spec("k8s-rbac", []string{"T1098.006"}, []string{"D3-AMED"}),
			spec("k8s-secret-read", []string{"T1552.007"}, nil),
		),
		Reports: []*indicator.Report{{FaultApplied: true, DataExposed: true, Detected: true}},
	}}

	got := Build(obs)

	if len(got.Attack) != 2 {
		t.Errorf("got %d attack techniques, want 2: %+v", len(got.Attack), got.Attack)
	}
	if len(got.D3fend) != 1 {
		t.Errorf("got %d d3fend techniques, want 1: %+v", len(got.D3fend), got.D3fend)
	}
	if got.D3fend[0].ID != "D3-AMED" {
		t.Errorf("d3fend id = %q, want D3-AMED", got.D3fend[0].ID)
	}
}

func TestBuildSortsTechniquesByID(t *testing.T) {
	obs := []Observation{{
		Experiment: exp("e1",
			spec("f", []string{"T9999", "T1000"}, nil),
			spec("p", []string{"T5000"}, nil),
		),
		Reports: []*indicator.Report{{}},
	}}

	got := Build(obs)

	want := []string{"T1000", "T5000", "T9999"}
	for i, id := range want {
		if got.Attack[i].ID != id {
			t.Errorf("attack[%d] = %q, want %q", i, got.Attack[i].ID, id)
		}
	}
}

func TestBuildAggregatesTheSameTechniqueAcrossExperiments(t *testing.T) {
	obs := []Observation{
		{
			Experiment: exp("guarded", spec("f", []string{"T1098.006"}, nil), spec("p", nil, nil)),
			Reports: []*indicator.Report{
				{FaultApplied: false, DataExposed: false},
				{FaultApplied: false, DataExposed: false},
			},
		},
		{
			Experiment: exp("unguarded", spec("f", []string{"T1098.006"}, nil), spec("p", nil, nil)),
			Reports: []*indicator.Report{
				{FaultApplied: true, DataExposed: true, Detected: true, MTTD: dur(20 * time.Millisecond)},
				{FaultApplied: true, DataExposed: true, Detected: true, MTTD: dur(30 * time.Millisecond)},
			},
		},
	}

	got := techniqueByID(t, Build(obs).Attack, "T1098.006")

	if got.Runs != 4 {
		t.Errorf("Runs = %d, want 4", got.Runs)
	}
	if got.ExposedRate != 0.5 {
		t.Errorf("ExposedRate = %v, want 0.5", got.ExposedRate)
	}
	if got.PreventedRate != 0.5 {
		t.Errorf("PreventedRate = %v, want 0.5", got.PreventedRate)
	}
	if got.MTTD == nil || got.MTTD.N != 2 {
		t.Fatalf("MTTD = %+v, want stats over the two detected runs", got.MTTD)
	}
	if got.MTTD.Mean != 25 {
		t.Errorf("MTTD.Mean = %v, want 25", got.MTTD.Mean)
	}
	if len(got.Experiments) != 2 {
		t.Errorf("Experiments = %v, want both", got.Experiments)
	}
}

func TestStatusReflectsMeasuredOutcome(t *testing.T) {
	cases := []struct {
		name     string
		exposed  float64
		detected float64
		want     Status
	}{
		{"never exposed", 0, 0, Blocked},
		{"never exposed but detected", 0, 1, Blocked},
		{"exposed and detected", 1, 1, DetectedOnly},
		{"exposed, sometimes detected", 1, 0.5, DetectedOnly},
		{"exposed and blind", 1, 0, Undetected},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Technique{ExposedRate: c.exposed, DetectedRate: c.detected}.Status()
			if got != c.want {
				t.Errorf("Status() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestStatusScoresRankResilience(t *testing.T) {
	if Blocked.Score() <= DetectedOnly.Score() {
		t.Error("a blocked attack must score above one that succeeded but was detected")
	}
	if DetectedOnly.Score() <= Undetected.Score() {
		t.Error("a detected attack must score above an undetected one")
	}
}

func TestBuildReportsUnannotatedExperiments(t *testing.T) {
	obs := []Observation{
		{Experiment: exp("mapped", spec("f", []string{"T1098.006"}, nil), spec("p", nil, nil)),
			Reports: []*indicator.Report{{}}},
		{Experiment: exp("bare", spec("f", nil, nil), spec("p", nil, nil)),
			Reports: []*indicator.Report{{}}},
	}

	got := Build(obs)

	if len(got.Unmapped) != 1 || got.Unmapped[0] != "bare" {
		t.Errorf("Unmapped = %v, want [bare]", got.Unmapped)
	}
}

func TestBuildHandlesNoObservations(t *testing.T) {
	got := Build(nil)

	if len(got.Attack) != 0 || len(got.D3fend) != 0 || len(got.Unmapped) != 0 {
		t.Errorf("Build(nil) = %+v, want empty", got)
	}
}

func TestNavigatorLayerIsValidAndScored(t *testing.T) {
	result := Result{Attack: []Technique{
		{ID: "T1098.006", Runs: 2, ExposedRate: 0, PreventedRate: 1, Experiments: []string{"guarded"}},
		{ID: "T1552.007", Runs: 2, ExposedRate: 1, DetectedRate: 0, Experiments: []string{"unguarded"}},
	}}

	var buf strings.Builder
	if err := WriteNavigatorLayer(&buf, "test layer", result); err != nil {
		t.Fatalf("WriteNavigatorLayer: %v", err)
	}

	var layer struct {
		Name     string `json:"name"`
		Domain   string `json:"domain"`
		Versions struct {
			Navigator string `json:"navigator"`
			Layer     string `json:"layer"`
		} `json:"versions"`
		Techniques []struct {
			TechniqueID string `json:"techniqueID"`
			Score       int    `json:"score"`
			Comment     string `json:"comment"`
			Enabled     bool   `json:"enabled"`
		} `json:"techniques"`
	}
	if err := json.Unmarshal([]byte(buf.String()), &layer); err != nil {
		t.Fatalf("layer is not valid JSON: %v", err)
	}

	if layer.Name != "test layer" {
		t.Errorf("name = %q, want %q", layer.Name, "test layer")
	}
	if layer.Domain != "enterprise-attack" {
		t.Errorf("domain = %q, want enterprise-attack", layer.Domain)
	}
	if layer.Versions.Layer != "4.5" {
		t.Errorf("layer version = %q, want 4.5", layer.Versions.Layer)
	}
	if len(layer.Techniques) != 2 {
		t.Fatalf("got %d techniques, want 2", len(layer.Techniques))
	}
	if layer.Techniques[0].Score != 100 {
		t.Errorf("blocked technique scored %d, want 100", layer.Techniques[0].Score)
	}
	if layer.Techniques[1].Score != 0 {
		t.Errorf("undetected technique scored %d, want 0", layer.Techniques[1].Score)
	}
	if !strings.Contains(layer.Techniques[0].Comment, "guarded") {
		t.Errorf("comment = %q, want it to name the experiment", layer.Techniques[0].Comment)
	}
}

func TestNavigatorLayerOmitsD3fendTechniques(t *testing.T) {
	result := Result{
		Attack: []Technique{{ID: "T1098.006", Runs: 1}},
		D3fend: []Technique{{ID: "D3-ITF", Runs: 1}},
	}

	var buf strings.Builder
	if err := WriteNavigatorLayer(&buf, "test", result); err != nil {
		t.Fatalf("WriteNavigatorLayer: %v", err)
	}

	if strings.Contains(buf.String(), "D3-ITF") {
		t.Error("a Navigator layer must contain only ATT&CK techniques")
	}
}
