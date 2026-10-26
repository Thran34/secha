package scaffold

import (
	"strings"
	"testing"
)

func TestRelaxableAcceptsAnEnforcedNonPrivilegedLevel(t *testing.T) {
	for _, level := range []string{"baseline", "restricted"} {
		if err := relaxable("prod", level, true); err != nil {
			t.Errorf("relaxable(%q) = %v, want nil", level, err)
		}
	}
}

func TestRelaxableRefusesANamespaceWithoutTheLabel(t *testing.T) {
	err := relaxable("prod", "", false)

	if err == nil || !strings.Contains(err.Error(), "pod-security.kubernetes.io/enforce") {
		t.Errorf("err = %v, want a refusal that names the missing label", err)
	}
}

func TestRelaxableRefusesAnAlreadyPrivilegedNamespace(t *testing.T) {
	err := relaxable("prod", "privileged", true)

	if err == nil || !strings.Contains(err.Error(), "nothing to relax") {
		t.Errorf("err = %v, want a refusal because privileged allows everything already", err)
	}
}

func TestRelaxableRefusesAnUnknownLevel(t *testing.T) {
	if err := relaxable("prod", "strict", true); err == nil {
		t.Error("relaxable(strict) = nil, want an error for a level the cluster will not apply")
	}
}

func TestRenderPodSecurityNamesTheRealNamespaceAndLevel(t *testing.T) {
	out := renderPodSecurity("prod", "restricted")

	for _, want := range []string{
		"kind: Experiment",
		"type: k8s-podsecurity",
		"action: relax",
		"type: k8s-pod-create",
		"type: k8s-namespace-label",
		"namespace: prod",
		"want: restricted",
		"from restricted to privileged",
		"dataExposed: false",
		"T1610",
		"T1611",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated experiment is missing %q", want)
		}
	}
}

func TestRenderPodSecurityNeedsNoServiceAccountFromTheLab(t *testing.T) {
	out := renderPodSecurity("prod", "baseline")

	if strings.Contains(out, "as:") || strings.Contains(out, "attacker") {
		t.Error("scaffolding must not assume a lab service account, the operator's own identity is enough to test admission")
	}
}
