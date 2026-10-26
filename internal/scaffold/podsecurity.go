package scaffold

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Thran34/secha/internal/k8s"
)

type PodSecurityRequest struct {
	Namespace string
}

func PodSecurity(ctx context.Context, req PodSecurityRequest) (string, error) {
	client, err := k8s.NewClient()
	if err != nil {
		return "", err
	}

	ns, err := client.Clientset.CoreV1().Namespaces().Get(ctx, req.Namespace, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("read namespace %s: %w", req.Namespace, err)
	}

	level, present := ns.Labels[k8s.PodSecurityEnforceLabel]
	if err := relaxable(req.Namespace, level, present); err != nil {
		return "", err
	}
	return renderPodSecurity(req.Namespace, level), nil
}

func relaxable(namespace, level string, present bool) error {
	if !present {
		return fmt.Errorf("namespace %s has no %s label, so there is no Pod Security level to relax",
			namespace, k8s.PodSecurityEnforceLabel)
	}
	if level == k8s.PodSecurityPrivileged {
		return fmt.Errorf("namespace %s already enforces %s, which allows everything, so there is nothing to relax",
			namespace, level)
	}
	if !k8s.IsPodSecurityLevel(level) {
		return fmt.Errorf("namespace %s enforces %q, which is not a Pod Security level the cluster will apply",
			namespace, level)
	}
	return nil
}

func renderPodSecurity(namespace, level string) string {
	return fmt.Sprintf(`apiVersion: secha/v1
kind: Experiment
metadata:
  name: %s-podsecurity
  hypothesis: >
    Relaxing Pod Security on namespace %s from %s to privileged lets anyone who
    may create pods there start a privileged container. Admission is the only
    thing in the way, and something should notice the label change or the pod.

safety:
  contexts: []

steadyState:
  checks:
    - type: k8s-pod-create
      target:
        namespace: %s
      expect:
        dataExposed: false

fault:
  type: k8s-podsecurity
  action: relax
  target:
    namespace: %s
  level: privileged

probe:
  type: k8s-pod-create
  attack: [T1610, T1611]
  target:
    namespace: %s

observe:
  window: 10s
  detectors:
    - type: k8s-namespace-label
      namespace: %s
      label: %s
      want: %s

indicators: [exposure, detection_coverage, mttd, recovery_latency]
`, namespace, namespace, level, namespace, namespace, namespace, namespace, k8s.PodSecurityEnforceLabel, level)
}
