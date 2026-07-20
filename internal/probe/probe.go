package probe

import (
	"context"
	"fmt"
	"time"

	"secha/internal/experiment"
)

type Result struct {
	Reachable   bool
	DataExposed bool
	Detail      string
	At          time.Time
}

type Probe interface {
	Run(ctx context.Context) (Result, error)
	Describe() string
}

type Constructor func(spec experiment.Spec) (Probe, error)

var registry = map[string]Constructor{}

func Register(typ string, c Constructor) {
	registry[typ] = c
}

func New(spec experiment.Spec) (Probe, error) {
	c, ok := registry[spec.Type]
	if !ok {
		return nil, fmt.Errorf("unknown probe type %q", spec.Type)
	}
	return c(spec)
}
