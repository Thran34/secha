package detector

import (
	"strings"
	"testing"
)

const enforce = "pod-security.kubernetes.io/enforce"

func TestLabelDriftIsSilentWhenTheLabelMatches(t *testing.T) {
	_, drifted := labelDrift(map[string]string{enforce: "baseline", "team": "db"}, enforce, "baseline")

	if drifted {
		t.Error("labelDrift = true for the expected value, want false")
	}
}

func TestLabelDriftFiresWhenTheValueChanged(t *testing.T) {
	detail, drifted := labelDrift(map[string]string{enforce: "privileged"}, enforce, "baseline")

	if !drifted {
		t.Fatal("labelDrift = false for a relaxed level, want true")
	}
	if !strings.Contains(detail, "privileged") || !strings.Contains(detail, "baseline") {
		t.Errorf("detail = %q, want it to name both the actual and the wanted value", detail)
	}
}

func TestLabelDriftFiresWhenTheLabelIsGone(t *testing.T) {
	detail, drifted := labelDrift(map[string]string{}, enforce, "baseline")

	if !drifted {
		t.Fatal("labelDrift = false for a missing label, want true: no label means no enforcement")
	}
	if !strings.Contains(detail, "missing") {
		t.Errorf("detail = %q, want it to say the label is missing", detail)
	}
}

func TestLabelDriftIgnoresOtherLabels(t *testing.T) {
	_, drifted := labelDrift(map[string]string{enforce: "baseline", "pod-security.kubernetes.io/warn": "privileged"}, enforce, "baseline")

	if drifted {
		t.Error("labelDrift = true because of an unrelated label, want false")
	}
}
