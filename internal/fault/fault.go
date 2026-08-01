package fault

import (
	"context"
	"errors"
	"fmt"

	"secha/internal/experiment"
)

var ErrRejected = errors.New("fault rejected by the cluster")

func Rejected(detail string) error {
	return fmt.Errorf("%w: %s", ErrRejected, detail)
}

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
