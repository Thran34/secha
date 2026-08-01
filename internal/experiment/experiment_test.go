package experiment

import (
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func parse(t *testing.T, doc string) *Experiment {
	t.Helper()
	var exp Experiment
	if err := yaml.Unmarshal([]byte(doc), &exp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return &exp
}

const minimalExperiment = `
kind: Experiment
metadata:
  name: minimal
fault:
  type: k8s-networkpolicy
probe:
  type: redis-exfil
`

func TestCheckUnmarshalKeepsProbeAndExpectSideBySide(t *testing.T) {
	exp := parse(t, `
kind: Experiment
metadata:
  name: steady
steadyState:
  checks:
    - type: redis-key-present
      key: secret:flag
      target:
        namespace: target
        selector: app=redis
      expect:
        dataExposed: true
fault:
  type: k8s-networkpolicy
probe:
  type: redis-exfil
`)

	if len(exp.SteadyState.Checks) != 1 {
		t.Fatalf("got %d checks, want 1", len(exp.SteadyState.Checks))
	}
	c := exp.SteadyState.Checks[0]

	if c.Probe.Type != "redis-key-present" {
		t.Errorf("probe type = %q, want redis-key-present", c.Probe.Type)
	}
	if c.Expect.DataExposed == nil || !*c.Expect.DataExposed {
		t.Errorf("expect.dataExposed = %v, want true", c.Expect.DataExposed)
	}
	if c.Expect.Reachable != nil {
		t.Errorf("expect.reachable = %v, want nil when the field is absent", *c.Expect.Reachable)
	}

	var cfg struct {
		Key    string `json:"key"`
		Target struct {
			Namespace string `json:"namespace"`
			Selector  string `json:"selector"`
		} `json:"target"`
	}
	if err := c.Probe.Decode(&cfg); err != nil {
		t.Fatalf("decode probe config: %v", err)
	}
	if cfg.Key != "secret:flag" {
		t.Errorf("key = %q, want secret:flag", cfg.Key)
	}
	if cfg.Target.Namespace != "target" || cfg.Target.Selector != "app=redis" {
		t.Errorf("target = %+v, want namespace target and selector app=redis", cfg.Target)
	}
}

func TestCheckUnmarshalDistinguishesFalseFromAbsent(t *testing.T) {
	exp := parse(t, `
kind: Experiment
metadata:
  name: steady
steadyState:
  checks:
    - type: redis-exfil
      expect:
        reachable: false
fault:
  type: k8s-networkpolicy
probe:
  type: redis-exfil
`)

	c := exp.SteadyState.Checks[0]

	if c.Expect.Reachable == nil {
		t.Fatal("expect.reachable is nil, an explicit false must not collapse into absent")
	}
	if *c.Expect.Reachable {
		t.Error("expect.reachable = true, want false")
	}
	if c.Expect.DataExposed != nil {
		t.Error("expect.dataExposed must stay nil when absent")
	}
}

func TestExpectIsEmpty(t *testing.T) {
	yes := true
	no := false

	cases := []struct {
		name string
		in   Expect
		want bool
	}{
		{"no assertion", Expect{}, true},
		{"reachable set", Expect{Reachable: &yes}, false},
		{"reachable set to false", Expect{Reachable: &no}, false},
		{"dataExposed set", Expect{DataExposed: &no}, false},
		{"both set", Expect{Reachable: &yes, DataExposed: &no}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.in.IsEmpty(); got != c.want {
				t.Errorf("IsEmpty() = %t, want %t", got, c.want)
			}
		})
	}
}

func TestValidateAcceptsMinimalExperiment(t *testing.T) {
	if err := parse(t, minimalExperiment).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidateRejectsIncompleteExperiments(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{
			name: "wrong kind",
			doc: `
kind: Comparison
metadata:
  name: x
fault:
  type: k8s-rbac
probe:
  type: k8s-secret-read
`,
			wantErr: "unsupported kind",
		},
		{
			name: "missing name",
			doc: `
kind: Experiment
fault:
  type: k8s-rbac
probe:
  type: k8s-secret-read
`,
			wantErr: "metadata.name is required",
		},
		{
			name: "missing fault type",
			doc: `
kind: Experiment
metadata:
  name: x
probe:
  type: k8s-secret-read
`,
			wantErr: "fault.type is required",
		},
		{
			name: "missing probe type",
			doc: `
kind: Experiment
metadata:
  name: x
fault:
  type: k8s-rbac
`,
			wantErr: "probe.type is required",
		},
		{
			name: "steady-state check without a type",
			doc: `
kind: Experiment
metadata:
  name: x
steadyState:
  checks:
    - expect:
        reachable: true
fault:
  type: k8s-rbac
probe:
  type: k8s-secret-read
`,
			wantErr: "steadyState.checks[0].type is required",
		},
		{
			name: "steady-state check without an assertion",
			doc: `
kind: Experiment
metadata:
  name: x
steadyState:
  checks:
    - type: redis-key-present
      key: secret:flag
fault:
  type: k8s-rbac
probe:
  type: k8s-secret-read
`,
			wantErr: "expect must set reachable and/or dataExposed",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := parse(t, c.doc).Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want an error containing %q", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("Validate() = %q, want it to contain %q", err, c.wantErr)
			}
		})
	}
}

func TestValidateReportsTheOffendingCheckIndex(t *testing.T) {
	exp := parse(t, `
kind: Experiment
metadata:
  name: x
steadyState:
  checks:
    - type: redis-exfil
      expect:
        reachable: false
    - type: redis-key-present
      key: secret:flag
fault:
  type: k8s-rbac
probe:
  type: k8s-secret-read
`)

	err := exp.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error for the second check")
	}
	if !strings.Contains(err.Error(), "checks[1]") {
		t.Errorf("Validate() = %q, want it to point at checks[1]", err)
	}
	if !strings.Contains(err.Error(), "redis-key-present") {
		t.Errorf("Validate() = %q, want it to name the offending probe type", err)
	}
}

func TestSpecPreservesRawConfigForPlugins(t *testing.T) {
	exp := parse(t, `
kind: Experiment
metadata:
  name: x
fault:
  type: k8s-rbac
  action: grant
  subject:
    namespace: attacker
    name: attacker
  grant:
    namespace: target
    resources:
      - secrets
    verbs:
      - get
probe:
  type: k8s-secret-read
`)

	var cfg struct {
		Action  string `json:"action"`
		Subject struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"subject"`
		Grant struct {
			Namespace string   `json:"namespace"`
			Resources []string `json:"resources"`
			Verbs     []string `json:"verbs"`
		} `json:"grant"`
	}
	if err := exp.Fault.Decode(&cfg); err != nil {
		t.Fatalf("decode fault config: %v", err)
	}

	if cfg.Action != "grant" {
		t.Errorf("action = %q, want grant", cfg.Action)
	}
	if cfg.Subject.Namespace != "attacker" || cfg.Subject.Name != "attacker" {
		t.Errorf("subject = %+v, want attacker/attacker", cfg.Subject)
	}
	if len(cfg.Grant.Resources) != 1 || cfg.Grant.Resources[0] != "secrets" {
		t.Errorf("grant.resources = %v, want [secrets]", cfg.Grant.Resources)
	}
	if len(cfg.Grant.Verbs) != 1 || cfg.Grant.Verbs[0] != "get" {
		t.Errorf("grant.verbs = %v, want [get]", cfg.Grant.Verbs)
	}
}

func TestObserveParsesDetectorList(t *testing.T) {
	exp := parse(t, `
kind: Experiment
metadata:
  name: x
fault:
  type: k8s-networkpolicy
probe:
  type: redis-exfil
observe:
  window: 10s
  detectors:
    - type: redis-acl-log
    - type: k8s-access-review
`)

	if exp.Observe.Window != "10s" {
		t.Errorf("window = %q, want 10s", exp.Observe.Window)
	}
	if len(exp.Observe.Detectors) != 2 {
		t.Fatalf("got %d detectors, want 2", len(exp.Observe.Detectors))
	}
	if exp.Observe.Detectors[0].Type != "redis-acl-log" {
		t.Errorf("detector 0 = %q, want redis-acl-log", exp.Observe.Detectors[0].Type)
	}
	if exp.Observe.Detectors[1].Type != "k8s-access-review" {
		t.Errorf("detector 1 = %q, want k8s-access-review", exp.Observe.Detectors[1].Type)
	}
}

func TestLoadRejectsMissingFile(t *testing.T) {
	if _, err := Load("testdata/does-not-exist.yaml"); err == nil {
		t.Error("Load() = nil error, want a read failure")
	}
}
