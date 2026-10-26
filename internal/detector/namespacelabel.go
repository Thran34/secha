package detector

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/k8s"
)

func init() {
	Register("k8s-namespace-label", newNamespaceLabelDetector)
}

const namespaceLabelPollInterval = 250 * time.Millisecond

type namespaceLabelConfig struct {
	Namespace string `json:"namespace"`
	Label     string `json:"label"`
	Want      string `json:"want"`
}

type namespaceLabelDetector struct {
	client    *k8s.Client
	namespace string
	label     string
	want      string
}

func newNamespaceLabelDetector(spec experiment.Spec) (Detector, error) {
	var cfg namespaceLabelConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode k8s-namespace-label detector: %w", err)
	}
	if cfg.Namespace == "" || cfg.Label == "" || cfg.Want == "" {
		return nil, fmt.Errorf("k8s-namespace-label: namespace, label and want are required")
	}
	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	return &namespaceLabelDetector{client: client, namespace: cfg.Namespace, label: cfg.Label, want: cfg.Want}, nil
}

func labelDrift(labels map[string]string, label, want string) (string, bool) {
	got, ok := labels[label]
	if !ok {
		return fmt.Sprintf("label %s is missing, want %s", label, want), true
	}
	if got != want {
		return fmt.Sprintf("label %s is %s, want %s", label, got, want), true
	}
	return "", false
}

func (d *namespaceLabelDetector) Observe(ctx context.Context, window time.Duration) (Signal, error) {
	deadline := time.Now().Add(window)
	for {
		ns, err := d.client.Clientset.CoreV1().Namespaces().Get(ctx, d.namespace, metav1.GetOptions{})
		if err != nil {
			return Signal{}, fmt.Errorf("get namespace %s: %w", d.namespace, err)
		}
		if detail, drifted := labelDrift(ns.Labels, d.label, d.want); drifted {
			return Signal{
				Detected: true,
				At:       time.Now(),
				Source:   "k8s-namespace-label",
				Detail:   fmt.Sprintf("namespace %s: %s", d.namespace, detail),
			}, nil
		}
		if time.Now().After(deadline) {
			return Signal{Detected: false, Source: "k8s-namespace-label"}, nil
		}
		select {
		case <-ctx.Done():
			return Signal{}, ctx.Err()
		case <-time.After(namespaceLabelPollInterval):
		}
	}
}

func (d *namespaceLabelDetector) Describe() string {
	return fmt.Sprintf("namespace label: %s=%s on %s", d.label, d.want, d.namespace)
}
