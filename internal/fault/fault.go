package fault

import (
	"context"
	"fmt"

	"secha/internal/experiment"
)

type Fault interface {
	Inject(ctx context.Context) error
	Restore(ctx context.Context) error
	Describe() string
}

type Constructor func(spec experiment.Spec) (Fault, error)

var registry = map[string]Constructor{}

func Register(typ string, c Constructor) {
	registry[typ] = c
}

func New(spec experiment.Spec) (Fault, error) {
	c, ok := registry[spec.Type]
	if !ok {
		return nil, fmt.Errorf("unknown fault type %q", spec.Type)
	}
	return c(spec)
}
