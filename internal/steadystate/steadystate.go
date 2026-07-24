package steadystate

import (
	"context"
	"fmt"
	"strings"

	"secha/internal/experiment"
	"secha/internal/probe"
)

type Violation struct {
	Check string
	Field string
	Want  bool
	Got   bool
}

func (v Violation) String() string {
	return fmt.Sprintf("%s: %s is %t, want %t", v.Check, v.Field, v.Got, v.Want)
}

func Verify(ctx context.Context, checks []experiment.Check) ([]Violation, error) {
	var violations []Violation
	for i, c := range checks {
		p, err := probe.New(c.Probe)
		if err != nil {
			return nil, fmt.Errorf("check %d (%s): %w", i, c.Probe.Type, err)
		}
		res, err := p.Run(ctx)
		if err != nil {
			return nil, fmt.Errorf("check %d (%s): %w", i, p.Describe(), err)
		}
		if want := c.Expect.Reachable; want != nil && *want != res.Reachable {
			violations = append(violations, Violation{p.Describe(), "reachable", *want, res.Reachable})
		}
		if want := c.Expect.DataExposed; want != nil && *want != res.DataExposed {
			violations = append(violations, Violation{p.Describe(), "dataExposed", *want, res.DataExposed})
		}
	}
	return violations, nil
}

func Format(violations []Violation) string {
	parts := make([]string, len(violations))
	for i, v := range violations {
		parts[i] = v.String()
	}
	return strings.Join(parts, "; ")
}
