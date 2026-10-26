package fault

import (
	"encoding/json"
	"testing"
)

func decodeLabelPatch(t *testing.T, patch []byte) map[string]any {
	t.Helper()
	var body struct {
		Metadata struct {
			Labels map[string]any `json:"labels"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(patch, &body); err != nil {
		t.Fatalf("patch is not json: %v", err)
	}
	return body.Metadata.Labels
}

func TestLabelPatchSetsTheGivenValue(t *testing.T) {
	value := "privileged"
	labels := decodeLabelPatch(t, labelPatch("pod-security.kubernetes.io/enforce", &value))

	if got := labels["pod-security.kubernetes.io/enforce"]; got != "privileged" {
		t.Errorf("patch sets label to %v, want privileged", got)
	}
}

func TestLabelPatchWithNilRemovesTheLabel(t *testing.T) {
	labels := decodeLabelPatch(t, labelPatch("pod-security.kubernetes.io/enforce", nil))

	got, present := labels["pod-security.kubernetes.io/enforce"]
	if !present || got != nil {
		t.Errorf("patch = %v, want an explicit null so a merge patch drops the label", labels)
	}
}

func TestLabelPatchTouchesOnlyTheOneLabel(t *testing.T) {
	value := "baseline"
	labels := decodeLabelPatch(t, labelPatch("pod-security.kubernetes.io/enforce", &value))

	if len(labels) != 1 {
		t.Errorf("patch touches %d labels, want exactly 1 so other labels survive", len(labels))
	}
}
