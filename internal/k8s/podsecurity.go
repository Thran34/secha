package k8s

import (
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	PodSecurityEnforceLabel = "pod-security.kubernetes.io/enforce"
	PodSecurityPrivileged   = "privileged"
	PodSecurityBaseline     = "baseline"
	PodSecurityRestricted   = "restricted"
)

func IsPodSecurityLevel(level string) bool {
	switch level {
	case PodSecurityPrivileged, PodSecurityBaseline, PodSecurityRestricted:
		return true
	}
	return false
}

func IsPodSecurityDenied(err error) bool {
	if err == nil || !apierrors.IsForbidden(err) {
		return false
	}
	return strings.Contains(err.Error(), "violates PodSecurity")
}
