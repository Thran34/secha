package safety

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Thran34/secha/internal/k8s"
)

var ErrContextNotAllowed = errors.New("refusing to run against this cluster")

type Policy struct {
	Contexts []string `json:"contexts"`
}

func (p Policy) Pinned() bool {
	return len(p.Contexts) > 0
}

func (p Policy) Check(target k8s.Target) error {
	if !p.Pinned() {
		return nil
	}
	for _, allowed := range p.Contexts {
		if allowed == target.Context {
			return nil
		}
	}
	return fmt.Errorf("%w: current kubeconfig context is %q, the experiment allows only %s",
		ErrContextNotAllowed, target.Context, quoteAll(p.Contexts))
}

func quoteAll(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf("%q", n))
	}
	return strings.Join(quoted, ", ")
}
