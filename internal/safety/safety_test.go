package safety

import (
	"errors"
	"strings"
	"testing"

	"github.com/Thran34/secha/internal/k8s"
)

func TestUnpinnedPolicyAcceptsAnyCluster(t *testing.T) {
	var p Policy

	if p.Pinned() {
		t.Error("a policy with no contexts must not report itself as pinned")
	}
	if err := p.Check(k8s.Target{Context: "anything"}); err != nil {
		t.Errorf("Check = %v, want nil for an unpinned policy", err)
	}
}

func TestPinnedPolicyAcceptsAnAllowedContext(t *testing.T) {
	p := Policy{Contexts: []string{"secha-lab", "staging"}}

	for _, ctx := range p.Contexts {
		if err := p.Check(k8s.Target{Context: ctx}); err != nil {
			t.Errorf("Check(%q) = %v, want nil", ctx, err)
		}
	}
}

func TestPinnedPolicyRefusesTheWrongCluster(t *testing.T) {
	p := Policy{Contexts: []string{"secha-lab"}}

	err := p.Check(k8s.Target{Context: "minikube", Server: "https://127.0.0.1:49433"})

	if err == nil {
		t.Fatal("Check accepted a context outside the allowlist")
	}
	if !errors.Is(err, ErrContextNotAllowed) {
		t.Errorf("error %v does not wrap ErrContextNotAllowed", err)
	}
	if !strings.Contains(err.Error(), "minikube") || !strings.Contains(err.Error(), "secha-lab") {
		t.Errorf("error %q must name both the current and the allowed context", err)
	}
}

func TestPinnedPolicyRefusesAnEmptyContext(t *testing.T) {
	p := Policy{Contexts: []string{"secha-lab"}}

	if err := p.Check(k8s.Target{}); err == nil {
		t.Error("an unknown current context must not satisfy an allowlist")
	}
}

func TestTargetStringPrefersTheContextName(t *testing.T) {
	withContext := k8s.Target{Context: "secha-lab", Server: "https://10.0.0.1:6443"}
	if got := withContext.String(); !strings.Contains(got, "secha-lab") || !strings.Contains(got, "10.0.0.1") {
		t.Errorf("String() = %q, want both the context and the server", got)
	}

	serverOnly := k8s.Target{Server: "https://10.0.0.1:6443"}
	if got := serverOnly.String(); got != "https://10.0.0.1:6443" {
		t.Errorf("String() = %q, want the bare server when there is no context", got)
	}
}
