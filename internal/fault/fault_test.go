package fault

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRejectedIsRecognisedByErrorsIs(t *testing.T) {
	err := Rejected("policy X denied the request")

	if !errors.Is(err, ErrRejected) {
		t.Error("errors.Is(err, ErrRejected) = false, want true")
	}
}

func TestRejectedSurvivesWrapping(t *testing.T) {
	err := fmt.Errorf("inject fault: %w", Rejected("policy X denied the request"))

	if !errors.Is(err, ErrRejected) {
		t.Error("errors.Is() = false through a wrapping layer, want true")
	}
}

func TestRejectedKeepsTheDetail(t *testing.T) {
	err := Rejected("policy X denied the request")

	if !strings.Contains(err.Error(), "policy X denied the request") {
		t.Errorf("error = %q, want it to carry the rejection detail", err)
	}
}

func TestOrdinaryErrorsAreNotRejections(t *testing.T) {
	cases := []error{
		errors.New("connection refused"),
		fmt.Errorf("create role: %w", errors.New("timeout")),
	}

	for _, err := range cases {
		if errors.Is(err, ErrRejected) {
			t.Errorf("errors.Is(%v, ErrRejected) = true, want false", err)
		}
	}
}
