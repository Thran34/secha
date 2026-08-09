package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Thran34/secha/internal/detector"
	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/fault"
	"github.com/Thran34/secha/internal/probe"
)

type recordingFault struct {
	injected      int
	restored      int
	restoreCtxErr []error
	injectErr     error
}

func (f *recordingFault) Inject(ctx context.Context) error {
	f.injected++
	return f.injectErr
}

func (f *recordingFault) Restore(ctx context.Context) error {
	f.restored++
	f.restoreCtxErr = append(f.restoreCtxErr, ctx.Err())
	return nil
}

func (f *recordingFault) Describe() string { return "recording fault" }

type scriptedProbe struct {
	run func(ctx context.Context) (probe.Result, error)
}

func (p *scriptedProbe) Run(ctx context.Context) (probe.Result, error) { return p.run(ctx) }

func (p *scriptedProbe) Describe() string { return "scripted probe" }

type silentDetector struct{}

func (silentDetector) Observe(ctx context.Context, window time.Duration) (detector.Signal, error) {
	return detector.Signal{}, nil
}

func (silentDetector) Describe() string { return "silent detector" }

func register(t *testing.T, f *recordingFault, p *scriptedProbe) *experiment.Experiment {
	t.Helper()
	fault.Register("test-fault", func(experiment.Spec) (fault.Fault, error) { return f, nil })
	probe.Register("test-probe", func(experiment.Spec) (probe.Probe, error) { return p, nil })
	detector.Register("test-detector", func(experiment.Spec) (detector.Detector, error) {
		return silentDetector{}, nil
	})
	return &experiment.Experiment{
		Metadata: experiment.Metadata{Name: "test"},
		Fault:    experiment.Spec{Type: "test-fault"},
		Probe:    experiment.Spec{Type: "test-probe"},
		Observe: experiment.Observe{
			Window:    "10ms",
			Detectors: []experiment.Spec{{Type: "test-detector"}},
		},
	}
}

func okProbe() *scriptedProbe {
	return &scriptedProbe{run: func(context.Context) (probe.Result, error) {
		return probe.Result{Reachable: true}, nil
	}}
}

func TestRunRestoresTheControlOnceOnTheHappyPath(t *testing.T) {
	f := &recordingFault{}
	exp := register(t, f, okProbe())

	report, err := Run(context.Background(), exp)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if f.injected != 1 {
		t.Errorf("injected %d times, want 1", f.injected)
	}
	if f.restored != 1 {
		t.Errorf("restored %d times, want exactly 1 on the happy path", f.restored)
	}
	if !report.FaultApplied {
		t.Error("FaultApplied = false, want true")
	}
}

func TestRunRestoresTheControlWhenTheProbeFails(t *testing.T) {
	f := &recordingFault{}
	exp := register(t, f, &scriptedProbe{run: func(context.Context) (probe.Result, error) {
		return probe.Result{}, errors.New("probe exploded")
	}})

	if _, err := Run(context.Background(), exp); err == nil {
		t.Fatal("Run returned no error for a failing probe")
	}

	if f.restored != 1 {
		t.Fatalf("restored %d times, want 1; a failed probe must not leave the control disabled", f.restored)
	}
}

func TestRunRestoresWithALiveContextAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f := &recordingFault{}
	exp := register(t, f, &scriptedProbe{run: func(ctx context.Context) (probe.Result, error) {
		cancel()
		return probe.Result{}, ctx.Err()
	}})

	if _, err := Run(ctx, exp); err == nil {
		t.Fatal("Run returned no error after cancellation")
	}

	if f.restored != 1 {
		t.Fatalf("restored %d times, want 1; Ctrl-C must not leave the security control disabled", f.restored)
	}
	if f.restoreCtxErr[0] != nil {
		t.Errorf("Restore ran with a cancelled context (%v); it would fail immediately and leave the control disabled",
			f.restoreCtxErr[0])
	}
}

func TestRunRestoresWhenTheFaultFailsHalfwayThrough(t *testing.T) {
	f := &recordingFault{injectErr: errors.New("half applied, then the api server said no")}
	exp := register(t, f, okProbe())

	if _, err := Run(context.Background(), exp); err == nil {
		t.Fatal("Run returned no error for a failing injection")
	}

	if f.restored != 1 {
		t.Errorf("restored %d times, want 1; a partially applied fault must still be rolled back", f.restored)
	}
}

func TestRunKeepsMeasuringWhenTheClusterRejectsTheFault(t *testing.T) {
	f := &recordingFault{injectErr: fault.Rejected("denied by admission policy")}
	exp := register(t, f, okProbe())

	report, err := Run(context.Background(), exp)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.FaultApplied {
		t.Error("FaultApplied = true, want false when the cluster rejected the fault")
	}
	if report.RejectionDetail == "" {
		t.Error("RejectionDetail is empty, the reason the fault was refused is the measurement")
	}
	if f.restored != 1 {
		t.Errorf("restored %d times, want 1; a policy may reject the second object after the first landed", f.restored)
	}
}
