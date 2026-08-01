package indicator

import (
	"testing"
	"time"
)

func fieldByName(t *testing.T, fields []Field, name string) Field {
	t.Helper()
	for _, f := range fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("field %q not present in %+v", name, fields)
	return Field{}
}

func hasField(fields []Field, name string) bool {
	for _, f := range fields {
		if f.Name == name {
			return true
		}
	}
	return false
}

func TestCompareDataExposedLowerIsBetter(t *testing.T) {
	cases := []struct {
		name     string
		baseline bool
		treated  bool
		want     string
	}{
		{"control leaks, treatment holds", true, false, "improved"},
		{"control holds, treatment leaks", false, true, "regressed"},
		{"neither leaks", false, false, "same"},
		{"both leak", true, true, "same"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmp := Compare(&Report{DataExposed: c.baseline}, &Report{DataExposed: c.treated})

			got := fieldByName(t, cmp.Fields(), "data exposed").Verdict
			if got != c.want {
				t.Errorf("verdict = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCompareDetectedHigherIsBetter(t *testing.T) {
	cases := []struct {
		name     string
		baseline bool
		treated  bool
		want     string
	}{
		{"detection gained", false, true, "improved"},
		{"detection lost", true, false, "regressed"},
		{"blind in both", false, false, "same"},
		{"detected in both", true, true, "same"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmp := Compare(
				&Report{FaultApplied: true, Detected: c.baseline},
				&Report{FaultApplied: true, Detected: c.treated},
			)

			got := fieldByName(t, cmp.Fields(), "detected").Verdict
			if got != c.want {
				t.Errorf("verdict = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCompareReachableCarriesNoVerdict(t *testing.T) {
	cmp := Compare(&Report{Reachable: false}, &Report{Reachable: true})

	got := fieldByName(t, cmp.Fields(), "reachable").Verdict
	if got != "—" {
		t.Errorf("verdict = %q, want %q: reachability alone does not carry resilience", got, "—")
	}
}

func TestCompareRecoveryLatencyCarriesNoVerdict(t *testing.T) {
	cmp := Compare(
		&Report{RecoveryLatency: 10 * time.Millisecond},
		&Report{RecoveryLatency: 90 * time.Millisecond},
	)

	got := fieldByName(t, cmp.Fields(), "recovery latency").Verdict
	if got != "—" {
		t.Errorf("verdict = %q, want %q", got, "—")
	}
}

func TestCompareMTTD(t *testing.T) {
	fast := 100 * time.Millisecond
	slow := 500 * time.Millisecond

	cases := []struct {
		name     string
		baseline *time.Duration
		treated  *time.Duration
		want     string
	}{
		{"never detected to detected", nil, &slow, "improved"},
		{"detected to never detected", &slow, nil, "regressed"},
		{"never detected in either", nil, nil, "same"},
		{"faster detection", &slow, &fast, "improved"},
		{"slower detection", &fast, &slow, "regressed"},
		{"unchanged", &fast, &fast, "same"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmp := Compare(
				&Report{FaultApplied: true, MTTD: c.baseline},
				&Report{FaultApplied: true, MTTD: c.treated},
			)

			got := fieldByName(t, cmp.Fields(), "mttd").Verdict
			if got != c.want {
				t.Errorf("verdict = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFormatMTTDRendersMissingDetectionAsInfinity(t *testing.T) {
	if got := FormatMTTD(nil); got != "∞ (never)" {
		t.Errorf("FormatMTTD(nil) = %q, want %q", got, "∞ (never)")
	}
	if got := FormatMTTD(ptrDur(250 * time.Millisecond)); got != "250ms" {
		t.Errorf("FormatMTTD(250ms) = %q, want %q", got, "250ms")
	}
}

func TestCompareOmitsRecoveryIntegrityWhenNeitherSideChecked(t *testing.T) {
	cmp := Compare(&Report{}, &Report{})

	if hasField(cmp.Fields(), "recovery integrity") {
		t.Error("recovery integrity must be omitted when no side ran steady-state checks")
	}
}

func TestCompareRecoveryIntegrity(t *testing.T) {
	cases := []struct {
		name        string
		baseline    *bool
		treated     *bool
		wantBase    string
		wantTreated string
		wantVerdict string
	}{
		{"restored in both", ptrBool(true), ptrBool(true), "true", "true", "same"},
		{"restoration fixed", ptrBool(false), ptrBool(true), "false", "true", "improved"},
		{"restoration broken", ptrBool(true), ptrBool(false), "true", "false", "regressed"},
		{"only treatment checked", nil, ptrBool(true), "n/a", "true", "—"},
		{"only baseline checked", ptrBool(true), nil, "true", "n/a", "—"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmp := Compare(
				&Report{RecoveryIntegrity: c.baseline},
				&Report{RecoveryIntegrity: c.treated},
			)

			got := fieldByName(t, cmp.Fields(), "recovery integrity")
			if got.Baseline != c.wantBase {
				t.Errorf("baseline = %q, want %q", got.Baseline, c.wantBase)
			}
			if got.Treatment != c.wantTreated {
				t.Errorf("treatment = %q, want %q", got.Treatment, c.wantTreated)
			}
			if got.Verdict != c.wantVerdict {
				t.Errorf("verdict = %q, want %q", got.Verdict, c.wantVerdict)
			}
		})
	}
}

func TestCompareFaultAppliedRejectionIsAnImprovement(t *testing.T) {
	cases := []struct {
		name     string
		baseline bool
		treated  bool
		want     string
	}{
		{"policy prevented the misconfiguration", true, false, "improved"},
		{"protection lost", false, true, "regressed"},
		{"fault landed in both", true, true, "same"},
		{"fault rejected in both", false, false, "same"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmp := Compare(&Report{FaultApplied: c.baseline}, &Report{FaultApplied: c.treated})

			got := fieldByName(t, cmp.Fields(), "fault applied").Verdict
			if got != c.want {
				t.Errorf("verdict = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCompareReproducesAdmissionPolicyDelta(t *testing.T) {
	mttd := 27 * time.Millisecond
	control := &Report{
		Experiment:   "rbac-secret-read-unguarded",
		FaultApplied: true,
		Reachable:    true,
		DataExposed:  true,
		Detected:     true,
		MTTD:         &mttd,
	}
	treatment := &Report{
		Experiment:   "rbac-secret-read",
		FaultApplied: false,
		Reachable:    true,
		DataExposed:  false,
		Detected:     false,
		MTTD:         nil,
	}

	fields := Compare(control, treatment).Fields()

	if got := fieldByName(t, fields, "fault applied").Verdict; got != "improved" {
		t.Errorf("fault applied verdict = %q, want improved: the policy rejected the misconfiguration", got)
	}
	if got := fieldByName(t, fields, "data exposed").Verdict; got != "improved" {
		t.Errorf("data exposed verdict = %q, want improved", got)
	}
	detected := fieldByName(t, fields, "detected")
	if detected.Verdict != "—" {
		t.Errorf("detected verdict = %q, want —: prevention leaves nothing to detect, so detection is not comparable", detected.Verdict)
	}
	if detected.Treatment != "n/a" {
		t.Errorf("detected treatment = %q, want n/a", detected.Treatment)
	}
	mttdField := fieldByName(t, fields, "mttd")
	if mttdField.Verdict != "—" || mttdField.Treatment != "n/a" {
		t.Errorf("mttd = %+v, want treatment n/a and verdict —", mttdField)
	}
}

func TestCompareSuppressesDetectionWhenFaultNeverLanded(t *testing.T) {
	mttd := 100 * time.Millisecond

	cases := []struct {
		name     string
		baseline *Report
		treated  *Report
	}{
		{
			name:     "treatment prevented the fault",
			baseline: &Report{FaultApplied: true, Detected: true, MTTD: &mttd},
			treated:  &Report{FaultApplied: false},
		},
		{
			name:     "baseline prevented the fault",
			baseline: &Report{FaultApplied: false},
			treated:  &Report{FaultApplied: true, Detected: true, MTTD: &mttd},
		},
		{
			name:     "neither fault landed",
			baseline: &Report{FaultApplied: false},
			treated:  &Report{FaultApplied: false},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fields := Compare(c.baseline, c.treated).Fields()

			for _, name := range []string{"detected", "mttd"} {
				if got := fieldByName(t, fields, name).Verdict; got != "—" {
					t.Errorf("%s verdict = %q, want — when a fault never landed", name, got)
				}
			}
		})
	}
}

func TestCompareKeepsDataExposedComparableWhenFaultRejected(t *testing.T) {
	fields := Compare(
		&Report{FaultApplied: true, DataExposed: true},
		&Report{FaultApplied: false, DataExposed: false},
	).Fields()

	got := fieldByName(t, fields, "data exposed")
	if got.Verdict != "improved" {
		t.Errorf("data exposed verdict = %q, want improved: not leaking is a real outcome regardless of why", got.Verdict)
	}
	if got.Treatment != "false" {
		t.Errorf("data exposed treatment = %q, want false, not n/a", got.Treatment)
	}
}

func TestCompareReproducesNetworkPolicyDelta(t *testing.T) {
	mttd := 564 * time.Millisecond
	control := &Report{
		Experiment:   "networkpolicy-web-to-db-noauth",
		FaultApplied: true,
		Reachable:    true,
		DataExposed:  true,
		Detected:     false,
		MTTD:         nil,
	}
	treatment := &Report{
		Experiment:   "networkpolicy-web-to-db",
		FaultApplied: true,
		Reachable:    true,
		DataExposed:  false,
		Detected:     true,
		MTTD:         &mttd,
	}

	fields := Compare(control, treatment).Fields()

	if got := fieldByName(t, fields, "data exposed").Verdict; got != "improved" {
		t.Errorf("data exposed verdict = %q, want improved: the AUTH layer blocks the leak", got)
	}
	if got := fieldByName(t, fields, "detected").Verdict; got != "improved" {
		t.Errorf("detected verdict = %q, want improved: the AUTH layer creates the detection signal", got)
	}
	if got := fieldByName(t, fields, "mttd"); got.Baseline != "∞ (never)" || got.Verdict != "improved" {
		t.Errorf("mttd = %+v, want baseline ∞ (never) and verdict improved", got)
	}
}

func TestFieldsOrderIsStable(t *testing.T) {
	cmp := Compare(&Report{RecoveryIntegrity: ptrBool(true)}, &Report{RecoveryIntegrity: ptrBool(true)})

	want := []string{"fault applied", "reachable", "data exposed", "detected", "mttd", "recovery latency", "recovery integrity"}
	fields := cmp.Fields()

	if len(fields) != len(want) {
		t.Fatalf("got %d fields, want %d: %+v", len(fields), len(want), fields)
	}
	for i, name := range want {
		if fields[i].Name != name {
			t.Errorf("field %d = %q, want %q", i, fields[i].Name, name)
		}
	}
}
