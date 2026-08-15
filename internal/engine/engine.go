package engine

import (
	"context"
	"errors"
	"fmt"
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
			return nil, fmt.Errorf("steady state not satisfied, aborting before fault injection: %s",
				steadystate.Format(violations))
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

	res, err := p.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("run probe: %w", err)
	}
	report.Reachable = res.Reachable
	report.DataExposed = res.DataExposed

	for _, d := range detectors {
		sig, err := d.Observe(ctx, window)
		if err != nil {
			return nil, fmt.Errorf("observe %s: %w", d.Describe(), err)
		}
		if sig.Detected {
			report.Detected = true
			mttd := sig.At.Sub(faultAt)
			report.MTTD = &mttd
			break
		}
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
