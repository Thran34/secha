package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Thran34/secha/internal/detector"
	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/probe"
)

type clockedDetector struct {
	mu        sync.Mutex
	started   time.Time
	detectsAt time.Duration
	err       error
	name      string
}

func (d *clockedDetector) Observe(ctx context.Context, window time.Duration) (detector.Signal, error) {
	d.mu.Lock()
	d.started = time.Now()
	d.mu.Unlock()
	if d.err != nil {
		return detector.Signal{}, d.err
	}
	if d.detectsAt < 0 {
		return detector.Signal{}, nil
	}
	select {
	case <-ctx.Done():
		return detector.Signal{}, ctx.Err()
	case <-time.After(d.detectsAt):
	}
	return detector.Signal{Detected: true, At: time.Now(), Source: d.name}, nil
}

func (d *clockedDetector) Describe() string { return d.name }

func (d *clockedDetector) startedAt() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.started
}

func withDetectors(t *testing.T, p *scriptedProbe, ds ...detector.Detector) *experiment.Experiment {
	t.Helper()
	exp := register(t, &recordingFault{}, p)
	exp.Observe.Window = "500ms"
	exp.Observe.Detectors = nil
	for i, d := range ds {
		typ := "clocked-" + string(rune('a'+i))
		d := d
		detector.Register(typ, func(experiment.Spec) (detector.Detector, error) { return d, nil })
		exp.Observe.Detectors = append(exp.Observe.Detectors, experiment.Spec{Type: typ})
	}
	return exp
}

func TestDetectorsStartWatchingBeforeTheProbeRuns(t *testing.T) {
	var probeRan time.Time
	d := &clockedDetector{name: "posture", detectsAt: -1}
	exp := withDetectors(t, &scriptedProbe{run: func(context.Context) (probe.Result, error) {
		probeRan = time.Now()
		return probe.Result{Reachable: true}, nil
	}}, d)

	if _, err := Run(context.Background(), exp); err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	if started := d.startedAt(); started.IsZero() || started.After(probeRan) {
		t.Errorf("detector started at %v, probe ran at %v; observation must begin before the attack", started, probeRan)
	}
}

func TestMTTDDoesNotIncludeTheProbeDuration(t *testing.T) {
	probeTakes := 150 * time.Millisecond
	d := &clockedDetector{name: "posture", detectsAt: 0}
	exp := withDetectors(t, &scriptedProbe{run: func(context.Context) (probe.Result, error) {
		time.Sleep(probeTakes)
		return probe.Result{Reachable: true}, nil
	}}, d)

	report, err := Run(context.Background(), exp)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !report.Detected || report.MTTD == nil {
		t.Fatal("posture drift that exists from injection was not detected")
	}
	if *report.MTTD >= probeTakes {
		t.Errorf("MTTD = %v, want well under the %v probe; a posture signal exists the moment the fault lands", *report.MTTD, probeTakes)
	}
}

func TestEarliestSignalWinsAcrossDetectors(t *testing.T) {
	slow := &clockedDetector{name: "slow", detectsAt: 120 * time.Millisecond}
	fast := &clockedDetector{name: "fast", detectsAt: 20 * time.Millisecond}
	exp := withDetectors(t, okProbe(), slow, fast)

	report, err := Run(context.Background(), exp)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.MTTD == nil || *report.MTTD >= 120*time.Millisecond {
		t.Errorf("MTTD = %v, want the fast detector's signal, not the slow one's", report.MTTD)
	}
}

func TestADetectorErrorFailsTheRunAndStillRestores(t *testing.T) {
	broken := &clockedDetector{name: "broken", err: errors.New("falco pod gone")}
	fine := &clockedDetector{name: "fine", detectsAt: 0}
	exp := withDetectors(t, okProbe(), broken, fine)
	f := &recordingFault{}
	exp.Fault = experiment.Spec{Type: "test-fault"}
	register(t, f, okProbe())
	exp.Probe = experiment.Spec{Type: "test-probe"}

	_, err := Run(context.Background(), exp)

	if err == nil || !errors.Is(err, broken.err) {
		t.Fatalf("Run error = %v, want the detector's error surfaced", err)
	}
	if f.restored != 1 {
		t.Errorf("restored %d times, want 1 even when observation fails", f.restored)
	}
}

func TestNoDetectionMeansNoMTTD(t *testing.T) {
	exp := withDetectors(t, okProbe(), &clockedDetector{name: "quiet", detectsAt: -1})

	report, err := Run(context.Background(), exp)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Detected || report.MTTD != nil {
		t.Errorf("report = detected %t mttd %v, want neither", report.Detected, report.MTTD)
	}
}
