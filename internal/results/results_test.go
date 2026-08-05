package results

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Thran34/secha/internal/indicator"
)

func ptrDur(d time.Duration) *time.Duration { return &d }

func ptrBool(b bool) *bool { return &b }

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "results.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestBuildCarriesEveryRun(t *testing.T) {
	reports := []*indicator.Report{
		{FaultApplied: true, Detected: true, MTTD: ptrDur(120 * time.Millisecond), RecoveryLatency: 10 * time.Millisecond},
		{FaultApplied: true, Detected: false, RecoveryLatency: 20 * time.Millisecond},
	}

	f := Build("exp", reports)

	if f.Experiment != "exp" {
		t.Errorf("Experiment = %q, want exp", f.Experiment)
	}
	if len(f.Runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(f.Runs))
	}
	if f.Runs[0].MTTDMs == nil || *f.Runs[0].MTTDMs != 120 {
		t.Errorf("run 0 mttd = %v, want 120", f.Runs[0].MTTDMs)
	}
	if f.Runs[1].MTTDMs != nil {
		t.Errorf("run 1 mttd = %v, want nil for an undetected run", *f.Runs[1].MTTDMs)
	}
	if f.Summary.Runs != 2 {
		t.Errorf("summary runs = %d, want 2", f.Summary.Runs)
	}
}

func TestBuildSummaryMatchesAggregate(t *testing.T) {
	reports := []*indicator.Report{
		{Detected: true, MTTD: ptrDur(100 * time.Millisecond)},
		{Detected: true, MTTD: ptrDur(300 * time.Millisecond)},
		{Detected: false},
	}

	got := Build("exp", reports).Summary
	want := indicator.Aggregate(reports)

	if got.DetectedRate != want.DetectedRate {
		t.Errorf("DetectedRate = %v, want %v", got.DetectedRate, want.DetectedRate)
	}
	if got.MTTDMs == nil {
		t.Fatal("MTTDMs is nil, want stats over the detected runs")
	}
	if got.MTTDMs.N != want.MTTD.N || got.MTTDMs.Mean != want.MTTD.Mean {
		t.Errorf("MTTD = %+v, want %+v", got.MTTDMs, want.MTTD)
	}
}

func TestBuildLeavesMTTDNilWhenNothingWasDetected(t *testing.T) {
	f := Build("exp", []*indicator.Report{{Detected: false}, {Detected: false}})

	if f.Summary.MTTDMs != nil {
		t.Errorf("MTTDMs = %+v, want nil so a report can say 'never detected'", f.Summary.MTTDMs)
	}
}

func TestWriteThenReadRoundTrips(t *testing.T) {
	reports := []*indicator.Report{
		{FaultApplied: true, Reachable: true, DataExposed: true, Detected: true,
			MTTD: ptrDur(27400 * time.Microsecond), RecoveryLatency: 41 * time.Millisecond,
			RecoveryIntegrity: ptrBool(true)},
	}

	var buf bytes.Buffer
	if err := Write(&buf, "rbac-secret-read", reports); err != nil {
		t.Fatalf("Write: %v", err)
	}

	path := writeTemp(t, buf.String())
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	want := Build("rbac-secret-read", reports)
	if got.Experiment != want.Experiment {
		t.Errorf("Experiment = %q, want %q", got.Experiment, want.Experiment)
	}
	if len(got.Runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(got.Runs))
	}
	if *got.Runs[0].MTTDMs != *want.Runs[0].MTTDMs {
		t.Errorf("mttd = %v, want %v", *got.Runs[0].MTTDMs, *want.Runs[0].MTTDMs)
	}
	if got.Summary.DataExposedRate != want.Summary.DataExposedRate {
		t.Errorf("DataExposedRate = %v, want %v", got.Summary.DataExposedRate, want.Summary.DataExposedRate)
	}
	if got.Runs[0].RecoveryIntegrity == nil || !*got.Runs[0].RecoveryIntegrity {
		t.Error("RecoveryIntegrity did not survive the round trip")
	}
}

func TestReadRejectsAFileThatIsNotSechaOutput(t *testing.T) {
	path := writeTemp(t, `{"some":"other json"}`)

	_, err := Read(path)

	if err == nil {
		t.Fatal("Read accepted a file with no experiment name")
	}
	if !strings.Contains(err.Error(), "experiment name") {
		t.Errorf("error = %v, want it to explain the file is not secha output", err)
	}
}

func TestReadRejectsMalformedJSON(t *testing.T) {
	if _, err := Read(writeTemp(t, "{not json")); err == nil {
		t.Fatal("Read accepted malformed JSON")
	}
}

func TestReadReportsAMissingFile(t *testing.T) {
	if _, err := Read(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("Read accepted a path that does not exist")
	}
}

func TestMillisecondsConvertsFromDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want float64
	}{
		{0, 0},
		{time.Millisecond, 1},
		{1500 * time.Microsecond, 1.5},
		{time.Second, 1000},
	}
	for _, c := range cases {
		if got := Milliseconds(c.in); got != c.want {
			t.Errorf("Milliseconds(%s) = %v, want %v", c.in, got, c.want)
		}
	}
}
