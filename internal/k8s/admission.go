package k8s

import (
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var admissionMarkers = []string{
	"ValidatingAdmissionPolicy",
	"admission webhook",
	"denied the request",
	"denied request",
}

func IsAdmissionDenied(err error) bool {
	if err == nil {
		return false
	}
	if !apierrors.IsInvalid(err) && !apierrors.IsForbidden(err) {
		return false
	}
	msg := err.Error()
	for _, marker := range admissionMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
