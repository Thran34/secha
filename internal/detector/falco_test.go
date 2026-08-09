package detector

import (
	"strings"
	"testing"
	"time"

	"github.com/Thran34/secha/internal/experiment"
)

const secretAlert = `{"hostname":"raspberrypi","output":"12:59:15.348677000: Error K8s Secret Get Successfully (user=system:admin secret=redis-auth ns=target-unguarded)","output_fields":{"ka.auth.decision":"allow","ka.response.code":"200","ka.target.name":"redis-auth","ka.target.namespace":"target-unguarded","ka.target.resource":"secrets","ka.user.name":"system:admin"},"priority":"Error","rule":"K8s Secret Get Successfully","source":"k8s_audit","tags":["k8s"],"time":"2026-08-09T12:59:15.348677000Z"}`

const execAlert = `{"hostname":"raspberrypi","output":"12:59:15.058380000: Notice Attach/Exec to pod","output_fields":{"ka.target.name":"attacker","ka.target.namespace":"attacker","ka.target.resource":"pods","ka.user.name":"system:admin"},"priority":"Notice","rule":"Attach/Exec Pod","source":"k8s_audit","tags":["k8s"],"time":"2026-08-09T12:59:15.058380000Z"}`

func detectorFor(rules []string, match map[string]string, since time.Time) *falcoAlertDetector {
	return &falcoAlertDetector{rules: rules, match: match, since: since}
}

func before() time.Time { return time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC) }

func after() time.Time { return time.Date(2026, 8, 9, 13, 0, 0, 0, time.UTC) }

func TestFirstMatchFindsTheNamedRule(t *testing.T) {
	d := detectorFor([]string{"K8s Secret Get Successfully"}, nil, before())

	alert, ok := d.firstMatch(execAlert + "\n" + secretAlert)

	if !ok {
		t.Fatal("no match, want the secret alert")
	}
	if alert.Rule != "K8s Secret Get Successfully" {
		t.Errorf("matched rule %q, want the secret rule", alert.Rule)
	}
}

func TestFirstMatchIgnoresOtherRules(t *testing.T) {
	d := detectorFor([]string{"K8s Secret Get Successfully"}, nil, before())

	if _, ok := d.firstMatch(execAlert); ok {
		t.Error("matched an unrelated rule; detection coverage would be overstated")
	}
}

func TestFirstMatchIgnoresAlertsFromBeforeTheReset(t *testing.T) {
	d := detectorFor([]string{"K8s Secret Get Successfully"}, nil, after())

	if _, ok := d.firstMatch(secretAlert); ok {
		t.Error("matched an alert older than the reset; a previous run would count as this run's detection")
	}
}

func TestFirstMatchRequiresEveryMatcherField(t *testing.T) {
	cases := map[string]struct {
		match map[string]string
		want  bool
	}{
		"exact namespace and name": {map[string]string{
			"ka.target.namespace": "target-unguarded", "ka.target.name": "redis-auth"}, true},
		"wrong namespace": {map[string]string{
			"ka.target.namespace": "target", "ka.target.name": "redis-auth"}, false},
		"one field right, one wrong": {map[string]string{
			"ka.target.namespace": "target-unguarded", "ka.target.name": "other-secret"}, false},
		"unknown field":     {map[string]string{"ka.nonexistent": "x"}, false},
		"no matcher at all": {nil, true},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			d := detectorFor([]string{"K8s Secret Get Successfully"}, c.match, before())
			if _, ok := d.firstMatch(secretAlert); ok != c.want {
				t.Errorf("match = %t, want %t", ok, c.want)
			}
		})
	}
}

func TestFirstMatchSkipsNonJSONLines(t *testing.T) {
	logs := strings.Join([]string{
		"Sun Aug 09 12:58:27 2026: Falco version: 0.44.1 (aarch64)",
		"Sun Aug 09 12:58:27 2026: Loaded event sources: syscall, k8s_audit",
		"{ this line is not valid json",
		secretAlert,
	}, "\n")
	d := detectorFor([]string{"K8s Secret Get Successfully"}, nil, before())

	if _, ok := d.firstMatch(logs); !ok {
		t.Error("startup chatter must not stop the detector finding the alert")
	}
}

func TestFirstMatchToleratesEmptyLogs(t *testing.T) {
	d := detectorFor([]string{"K8s Secret Get Successfully"}, nil, before())

	if _, ok := d.firstMatch(""); ok {
		t.Error("empty logs must not report a detection")
	}
}

func TestNewFalcoAlertDetectorRefusesAnUnfilteredRuleSet(t *testing.T) {
	_, err := newFalcoAlertDetector(specFrom(t, `{"type":"falco-alert"}`))

	if err == nil {
		t.Fatal("a detector with no rules was accepted; it would fire on unrelated cluster activity")
	}
	if !strings.Contains(err.Error(), "rules are required") {
		t.Errorf("error = %v, want it to explain why rules are mandatory", err)
	}
}

func TestNewFalcoAlertDetectorRejectsABadInterval(t *testing.T) {
	_, err := newFalcoAlertDetector(specFrom(t,
		`{"type":"falco-alert","rules":["X"],"interval":"soon"}`))

	if err == nil {
		t.Fatal("an unparseable interval was accepted")
	}
}

func specFrom(t *testing.T, raw string) experiment.Spec {
	t.Helper()
	var spec experiment.Spec
	if err := spec.UnmarshalJSON([]byte(raw)); err != nil {
		t.Fatalf("build spec: %v", err)
	}
	return spec
}
