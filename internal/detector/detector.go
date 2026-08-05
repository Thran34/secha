package detector

import (
	"context"
	"fmt"
	"time"

	"github.com/Thran34/secha/internal/experiment"
)

type Signal struct {
	Detected bool
	At       time.Time
	Source   string
	Detail   string
}

type Detector interface {
	Observe(ctx context.Context, window time.Duration) (Signal, error)
	Describe() string
}

type Resettable interface {
	Reset(ctx context.Context) error
}

type Constructor func(spec experiment.Spec) (Detector, error)

var registry = map[string]Constructor{}

func Register(typ string, c Constructor) {
	registry[typ] = c
}

func New(spec experiment.Spec) (Detector, error) {
	c, ok := registry[spec.Type]
	if !ok {
		return nil, fmt.Errorf("unknown detector type %q", spec.Type)
	}
	return c(spec)
}

func NewAll(specs []experiment.Spec) ([]Detector, error) {
	out := make([]Detector, 0, len(specs))
	for _, s := range specs {
		d, err := New(s)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}
