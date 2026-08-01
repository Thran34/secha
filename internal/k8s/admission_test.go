package k8s

import (
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func statusErr(code int32, reason metav1.StatusReason, message string) error {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    code,
		Reason:  reason,
		Message: message,
	}}
}

const realPolicyDenial = `roles.rbac.authorization.k8s.io "secha-escalation-default" is forbidden: ` +
	`ValidatingAdmissionPolicy 'secha-deny-secret-roles' with binding 'secha-deny-secret-roles' ` +
	`denied request: Role grants read access to secrets`

func TestIsAdmissionDeniedAcceptsValidatingAdmissionPolicy(t *testing.T) {
	err := statusErr(422, metav1.StatusReasonInvalid, realPolicyDenial)

	if apierrors.IsForbidden(err) {
		t.Error("fixture drifted: a ValidatingAdmissionPolicy denial is reported as Invalid, not Forbidden")
	}
	if !IsAdmissionDenied(err) {
		t.Error("IsAdmissionDenied() = false, want true for a ValidatingAdmissionPolicy denial")
	}
}

func TestIsAdmissionDeniedAcceptsWebhookDenial(t *testing.T) {
	err := statusErr(403, metav1.StatusReasonForbidden,
		`admission webhook "validate.kyverno.svc" denied the request: policy blocked this Role`)

	if !IsAdmissionDenied(err) {
		t.Error("IsAdmissionDenied() = false, want true for a webhook denial")
	}
}

func TestIsAdmissionDeniedRejectsOperationalFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"plain error", errors.New("connection refused")},
		{
			name: "operator lacks permission",
			err: apierrors.NewForbidden(
				schema.GroupResource{Group: "rbac.authorization.k8s.io", Resource: "roles"},
				"secha-escalation-default",
				errors.New("user cannot create roles"),
			),
		},
		{
			name: "object genuinely malformed",
			err:  statusErr(422, metav1.StatusReasonInvalid, `Role "x" is invalid: rules[0].verbs: Required value`),
		},
		{"not found", apierrors.NewNotFound(schema.GroupResource{Resource: "roles"}, "x")},
		{"already exists", apierrors.NewAlreadyExists(schema.GroupResource{Resource: "roles"}, "x")},
		{"timeout", apierrors.NewTimeoutError("request timed out", 30)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if IsAdmissionDenied(c.err) {
				t.Errorf("IsAdmissionDenied() = true, want false: %v", c.err)
			}
		})
	}
}

func TestIsAdmissionDeniedRequiresBothStatusAndMarker(t *testing.T) {
	notFoundMentioningPolicy := statusErr(404, metav1.StatusReasonNotFound,
		"ValidatingAdmissionPolicy 'x' not found")

	if IsAdmissionDenied(notFoundMentioningPolicy) {
		t.Error("a 404 that merely mentions a policy must not count as a denial")
	}

	invalidWithoutMarker := statusErr(422, metav1.StatusReasonInvalid, "Role is invalid: something else")

	if IsAdmissionDenied(invalidWithoutMarker) {
		t.Error("an Invalid status without an admission marker must not count as a denial")
	}
}
