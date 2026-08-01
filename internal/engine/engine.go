package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"secha/internal/detector"
	"secha/internal/experiment"
	"secha/internal/fault"
	"secha/internal/indicator"
	"secha/internal/probe"
	"secha/internal/steadystate"
)

const defaultWindow = 10 * time.Second

func Run(ctx context.Context, exp *experiment.Experiment) (*indicator.Report, error) {
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

	report := &indicator.Report{Experiment: exp.Metadata.Name}

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

	report.FaultApplied = true
	if err := f.Inject(ctx); err != nil {
		if !errors.Is(err, fault.ErrRejected) {
			return nil, fmt.Errorf("inject fault: %w", err)
		}
		report.FaultApplied = false
		report.RejectionDetail = err.Error()
	}
	faultAt := time.Now()
	defer f.Restore(ctx)

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
	if err := f.Restore(ctx); err != nil {
		return nil, fmt.Errorf("restore fault: %w", err)
	}
	report.RecoveryLatency = time.Since(restoreStart)

	if len(checks) > 0 {
		violations, err := steadystate.Verify(ctx, checks)
		if err != nil {
			return nil, fmt.Errorf("verify recovery integrity: %w", err)
		}
		intact := len(violations) == 0
		report.RecoveryIntegrity = &intact
		report.RecoveryDetail = steadystate.Format(violations)
	}

	return report, nil
}
