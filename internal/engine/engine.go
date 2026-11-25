package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Thran34/secha/internal/detector"
	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/fault"
	"github.com/Thran34/secha/internal/indicator"
	"github.com/Thran34/secha/internal/probe"
	"github.com/Thran34/secha/internal/steadystate"
)

const (
	defaultWindow    = 10 * time.Second
	restoreTimeout   = 30 * time.Second
	recoveryAttempts = 3
	recoveryBackoff  = 750 * time.Millisecond
)

var ErrControlLeftDisabled = errors.New("THE SECURITY CONTROL IS STILL DISABLED, restore it by hand")

var ErrSteadyStateViolated = errors.New("steady state not satisfied")

func Run(ctx context.Context, exp *experiment.Experiment) (report *indicator.Report, err error) {
	f, err := fault.New(exp.Fault)
	if err != nil {
		return nil, err
	}
	p, err := probe.New(exp.Probe)
	if err != nil {
		return nil, err
	}
	detectors, err := detector.NewAll(exp.Observe.Detectors)
	if err != nil {
		return nil, err
	}

	window := defaultWindow
	if exp.Observe.Window != "" {
		window, err = time.ParseDuration(exp.Observe.Window)
		if err != nil {
			return nil, fmt.Errorf("observe.window: %w", err)
		}
	}

	report = &indicator.Report{Experiment: exp.Metadata.Name}

	checks := exp.SteadyState.Checks
	if len(checks) > 0 {
		violations, err := steadystate.Verify(ctx, checks)
		if err != nil {
			return nil, fmt.Errorf("verify steady state: %w", err)
		}
		if len(violations) > 0 {
			return nil, fmt.Errorf("%w, aborting before fault injection: %s",
				ErrSteadyStateViolated, steadystate.Format(violations))
		}
	}

	restored := false
	defer func() {
		if restored {
			return
		}
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreTimeout)
		defer cancel()
		if rerr := f.Restore(rctx); rerr != nil {
			err = errors.Join(err, fmt.Errorf("%w: %s: %v", ErrControlLeftDisabled, f.Describe(), rerr))
		}
	}()

	report.FaultApplied = true
	if ierr := f.Inject(ctx); ierr != nil {
		if !errors.Is(ierr, fault.ErrRejected) {
			return nil, fmt.Errorf("inject fault: %w", ierr)
		}
		report.FaultApplied = false
		report.RejectionDetail = ierr.Error()
	}
	faultAt := time.Now()

	for _, d := range detectors {
		if r, ok := d.(detector.Resettable); ok {
			if err := r.Reset(ctx); err != nil {
				return nil, fmt.Errorf("reset detector %s: %w", d.Describe(), err)
			}
		}
	}

	octx, stopObserving := context.WithCancel(ctx)
	defer stopObserving()
	observations := observe(octx, detectors, window)

	res, err := p.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("run probe: %w", err)
	}
	report.Reachable = res.Reachable
	report.DataExposed = res.DataExposed

	first, err := earliest(observations, len(detectors))
	if err != nil {
		return nil, err
	}
	if first.Detected {
		report.Detected = true
		mttd := first.At.Sub(faultAt)
		report.MTTD = &mttd
	}

	restoreStart := time.Now()
	if rerr := f.Restore(ctx); rerr != nil {
		return nil, fmt.Errorf("restore fault: %w", rerr)
	}
	restored = true
	report.RecoveryLatency = time.Since(restoreStart)

	if len(checks) > 0 {
		violations, err := verifyRecovery(ctx, checks)
		if err != nil {
			return nil, fmt.Errorf("verify recovery integrity: %w", err)
		}
		intact := len(violations) == 0
		report.RecoveryIntegrity = &intact
		report.RecoveryDetail = steadystate.Format(violations)
	}

	return report, nil
}

type observation struct {
	signal detector.Signal
	err    error
	source string
}

func observe(ctx context.Context, detectors []detector.Detector, window time.Duration) <-chan observation {
	out := make(chan observation, len(detectors))
	var watching sync.WaitGroup
	watching.Add(len(detectors))
	for _, d := range detectors {
		go func(d detector.Detector) {
			watching.Done()
			sig, err := d.Observe(ctx, window)
			out <- observation{signal: sig, err: err, source: d.Describe()}
		}(d)
	}
	watching.Wait()
	return out
}

func earliest(observations <-chan observation, n int) (detector.Signal, error) {
	var first detector.Signal
	var errs error
	for i := 0; i < n; i++ {
		o := <-observations
		if o.err != nil {
			errs = errors.Join(errs, fmt.Errorf("observe %s: %w", o.source, o.err))
			continue
		}
		if !o.signal.Detected {
			continue
		}
		if !first.Detected || o.signal.At.Before(first.At) {
			first = o.signal
		}
	}
	if errs != nil {
		return detector.Signal{}, errs
	}
	return first, nil
}

func verifyRecovery(ctx context.Context, checks []experiment.Check) ([]steadystate.Violation, error) {
	var violations []steadystate.Violation
	for attempt := 0; attempt < recoveryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(recoveryBackoff):
			}
		}
		var err error
		violations, err = steadystate.Verify(ctx, checks)
		if err != nil {
			return nil, err
		}
		if len(violations) == 0 {
			return nil, nil
		}
	}
	return violations, nil
}
