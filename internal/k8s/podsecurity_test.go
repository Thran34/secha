package k8s

import (
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var pods = schema.GroupResource{Group: "", Resource: "pods"}

func TestPodSecurityDenialIsAForbiddenThatNamesPodSecurity(t *testing.T) {
	err := apierrors.NewForbidden(pods, "psa-check",
		errors.New(`violates PodSecurity "baseline:latest": privileged (container "c" must not set securityContext.privileged=true)`))

	if !IsPodSecurityDenied(err) {
		t.Error("IsPodSecurityDenied = false for the admission controller's own message, want true")
	}
}

func TestRBACForbiddenIsNotAPodSecurityDenial(t *testing.T) {
	err := apierrors.NewForbidden(pods, "psa-check",
		errors.New(`User "system:serviceaccount:attacker:default" cannot create resource "pods" in API group "" in the namespace "target"`))

	if IsPodSecurityDenied(err) {
		t.Error("IsPodSecurityDenied = true for an RBAC denial, want false so the probe reports it as an error")
	}
}

func TestNonForbiddenErrorsAreNotPodSecurityDenials(t *testing.T) {
	cases := []error{
		nil,
		errors.New("violates PodSecurity but is not an api error"),
		apierrors.NewInvalid(schema.GroupKind{Kind: "Pod"}, "x", nil),
	}
	for _, err := range cases {
		if IsPodSecurityDenied(err) {
			t.Errorf("IsPodSecurityDenied(%v) = true, want false", err)
		}
	}
}

func TestPodSecurityLevelsAreTheThreeTheClusterKnows(t *testing.T) {
	for _, level := range []string{"privileged", "baseline", "restricted"} {
		if !IsPodSecurityLevel(level) {
			t.Errorf("IsPodSecurityLevel(%q) = false, want true", level)
		}
	}
	for _, level := range []string{"", "Baseline", "strict", "latest"} {
		if IsPodSecurityLevel(level) {
			t.Errorf("IsPodSecurityLevel(%q) = true, want false", level)
		}
	}
}
