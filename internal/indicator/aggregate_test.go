package indicator

import (
	"math"
	"testing"
	"time"
)

const epsilon = 1e-9

func ptrDur(d time.Duration) *time.Duration { return &d }

func ptrBool(b bool) *bool { return &b }

func closeTo(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > epsilon {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestComputeStatsUsesSampleStddev(t *testing.T) {
	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}

	got := computeStats(xs)

	if got.N != 8 {
		t.Errorf("N = %d, want 8", got.N)
	}
	closeTo(t, "Mean", got.Mean, 5)
	closeTo(t, "Min", got.Min, 2)
	closeTo(t, "Max", got.Max, 9)
	closeTo(t, "Stddev", got.Stddev, math.Sqrt(32.0/7.0))

	if math.Abs(got.Stddev-2.0) < epsilon {
		t.Error("Stddev equals the population stddev, denominator must be N-1")
	}
}

func TestComputeStatsSingleSampleHasNoSpread(t *testing.T) {
	got := computeStats([]float64{42})

	if got.N != 1 {
		t.Errorf("N = %d, want 1", got.N)
	}
	closeTo(t, "Mean", got.Mean, 42)
	closeTo(t, "Min", got.Min, 42)
	closeTo(t, "Max", got.Max, 42)
	closeTo(t, "Stddev", got.Stddev, 0)
}

func TestComputeStatsIdenticalSamples(t *testing.T) {
	got := computeStats([]float64{7, 7, 7})

	closeTo(t, "Mean", got.Mean, 7)
	closeTo(t, "Stddev", got.Stddev, 0)
}

func TestComputeStatsUnorderedInput(t *testing.T) {
	got := computeStats([]float64{9, 1, 5})

	closeTo(t, "Min", got.Min, 1)
	closeTo(t, "Max", got.Max, 9)
	closeTo(t, "Mean", got.Mean, 5)
}

func TestComputeStatsNegativeValues(t *testing.T) {
	got := computeStats([]float64{-4, -2})

	closeTo(t, "Mean", got.Mean, -3)
	closeTo(t, "Min", got.Min, -4)
	closeTo(t, "Max", got.Max, -2)
	closeTo(t, "Stddev", got.Stddev, math.Sqrt(2))
}

func TestAggregateRates(t *testing.T) {
	reports := []*Report{
		{Reachable: true, DataExposed: false, Detected: true},
		{Reachable: true, DataExposed: true, Detected: true},
		{Reachable: false, DataExposed: false, Detected: false},
		{Reachable: true, DataExposed: false, Detected: true},
	}

	got := Aggregate(reports)

	if got.Runs != 4 {
		t.Errorf("Runs = %d, want 4", got.Runs)
	}
	closeTo(t, "ReachableRate", got.ReachableRate, 0.75)
	closeTo(t, "DataExposedRate", got.DataExposedRate, 0.25)
	closeTo(t, "DetectedRate", got.DetectedRate, 0.75)
}

func TestAggregateFaultAppliedRate(t *testing.T) {
	reports := []*Report{
		{FaultApplied: true},
		{FaultApplied: false},
		{FaultApplied: false},
		{FaultApplied: false},
	}

	got := Aggregate(reports)

	closeTo(t, "FaultAppliedRate", got.FaultAppliedRate, 0.25)
}

func TestAggregateFaultAppliedRateZeroWhenPolicyAlwaysRejects(t *testing.T) {
	reports := []*Report{{FaultApplied: false}, {FaultApplied: false}}

	got := Aggregate(reports)

	closeTo(t, "FaultAppliedRate", got.FaultAppliedRate, 0)
	closeTo(t, "DataExposedRate", got.DataExposedRate, 0)
}

func TestAggregateMTTDOnlyCoversDetectedRuns(t *testing.T) {
	reports := []*Report{
		{Detected: true, MTTD: ptrDur(100 * time.Millisecond)},
		{Detected: false},
		{Detected: true, MTTD: ptrDur(300 * time.Millisecond)},
	}

	got := Aggregate(reports)

	if got.MTTD == nil {
		t.Fatal("MTTD is nil, want stats over the two detected runs")
	}
	if got.MTTD.N != 2 {
		t.Errorf("MTTD.N = %d, want 2 (undetected runs must not count)", got.MTTD.N)
	}
	closeTo(t, "MTTD.Mean", got.MTTD.Mean, 200)
	closeTo(t, "MTTD.Min", got.MTTD.Min, 100)
	closeTo(t, "MTTD.Max", got.MTTD.Max, 300)
}

func TestAggregateMTTDNilWhenNothingDetected(t *testing.T) {
	reports := []*Report{{Detected: false}, {Detected: false}}

	got := Aggregate(reports)

	if got.MTTD != nil {
		t.Errorf("MTTD = %+v, want nil when no run detected", got.MTTD)
	}
	closeTo(t, "DetectedRate", got.DetectedRate, 0)
}

func TestAggregateRecoveryIntegrityRateSkipsUncheckedRuns(t *testing.T) {
	reports := []*Report{
		{RecoveryIntegrity: ptrBool(true)},
		{RecoveryIntegrity: ptrBool(false)},
		{RecoveryIntegrity: nil},
		{RecoveryIntegrity: ptrBool(true)},
	}

	got := Aggregate(reports)

	if got.RecoveryIntegrityRate == nil {
		t.Fatal("RecoveryIntegrityRate is nil, want a rate over the three checked runs")
	}
	closeTo(t, "RecoveryIntegrityRate", *got.RecoveryIntegrityRate, 2.0/3.0)
}

func TestAggregateRecoveryIntegrityRateNilWhenNeverChecked(t *testing.T) {
	reports := []*Report{{}, {}}

	got := Aggregate(reports)

	if got.RecoveryIntegrityRate != nil {
		t.Errorf("RecoveryIntegrityRate = %v, want nil when no run ran steady-state checks", *got.RecoveryIntegrityRate)
	}
}

func TestAggregateRecoveryCoversEveryRun(t *testing.T) {
	reports := []*Report{
		{RecoveryLatency: 10 * time.Millisecond},
		{RecoveryLatency: 20 * time.Millisecond},
	}

	got := Aggregate(reports)

	if got.Recovery.N != 2 {
		t.Errorf("Recovery.N = %d, want 2", got.Recovery.N)
	}
	closeTo(t, "Recovery.Mean", got.Recovery.Mean, 15)
}

func TestAggregateEmptyInput(t *testing.T) {
	got := Aggregate(nil)

	if got.Runs != 0 {
		t.Errorf("Runs = %d, want 0", got.Runs)
	}
	if got.MTTD != nil {
		t.Error("MTTD must be nil for an empty run set")
	}
	if got.RecoveryIntegrityRate != nil {
		t.Error("RecoveryIntegrityRate must be nil for an empty run set")
	}
	if got.Recovery.N != 0 {
		t.Errorf("Recovery.N = %d, want 0", got.Recovery.N)
	}
}

func TestStatsOfToleratesAnEmptySample(t *testing.T) {
	got := StatsOf(nil)

	if got.N != 0 {
		t.Errorf("N = %d, want 0", got.N)
	}
	closeTo(t, "Mean", got.Mean, 0)
	closeTo(t, "Stddev", got.Stddev, 0)
}

func TestStatsOfMatchesComputeStats(t *testing.T) {
	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}

	if StatsOf(xs) != computeStats(xs) {
		t.Error("StatsOf must delegate to the same maths as the internal aggregation")
	}
}

func TestDurMsConvertsToMilliseconds(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want float64
	}{
		{0, 0},
		{time.Millisecond, 1},
		{1500 * time.Microsecond, 1.5},
		{time.Second, 1000},
		{564400 * time.Microsecond, 564.4},
	}

	for _, c := range cases {
		closeTo(t, "durMs("+c.in.String()+")", durMs(c.in), c.want)
	}
}
