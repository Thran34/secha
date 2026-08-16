package scaffold

import (
	"context"
	"fmt"
	"sort"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Thran34/secha/internal/k8s"
)

type NetworkPolicyRequest struct {
	Namespace string
	Policy    string
	Port      int
	Service   string
}

func NetworkPolicy(ctx context.Context, req NetworkPolicyRequest) (string, error) {
	client, err := k8s.NewClient()
	if err != nil {
		return "", err
	}

	policy, err := client.Clientset.NetworkingV1().
		NetworkPolicies(req.Namespace).Get(ctx, req.Policy, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("read networkpolicy %s/%s: %w", req.Namespace, req.Policy, err)
	}

	port := req.Port
	if port == 0 {
		port = firstPort(policy)
	}
	if port == 0 {
		return "", fmt.Errorf("networkpolicy %s/%s names no port, pass --port to say which one to test",
			req.Namespace, req.Policy)
	}

	service := req.Service
	if service == "" {
		service, err = guessService(ctx, client, req.Namespace, policy)
		if err != nil {
			return "", err
		}
	}

	target := fmt.Sprintf("%s.%s.svc.cluster.local:%d", service, req.Namespace, port)
	return render(req.Namespace, req.Policy, target, describeSelector(policy)), nil
}

func firstPort(policy *networkingv1.NetworkPolicy) int {
	for _, rule := range policy.Spec.Ingress {
		for _, p := range rule.Ports {
			if p.Port != nil && p.Port.IntValue() > 0 {
				return p.Port.IntValue()
			}
		}
	}
	return 0
}

func guessService(ctx context.Context, client *k8s.Client, namespace string, policy *networkingv1.NetworkPolicy) (string, error) {
	selector := policy.Spec.PodSelector.MatchLabels
	if len(selector) == 0 {
		return "", fmt.Errorf("networkpolicy %s/%s selects every pod, pass --service to say what to reach",
			namespace, policy.Name)
	}

	services, err := client.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list services in %s: %w", namespace, err)
	}

	var matches []string
	for _, svc := range services.Items {
		if len(svc.Spec.Selector) == 0 {
			continue
		}
		if covers(selector, svc.Spec.Selector) {
			matches = append(matches, svc.Name)
		}
	}
	sort.Strings(matches)

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no service in %s selects the pods this policy protects, pass --service", namespace)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("several services select those pods (%s), pass --service to pick one",
			strings.Join(matches, ", "))
	}
}

func covers(policySelector, serviceSelector map[string]string) bool {
	for k, v := range serviceSelector {
		if policySelector[k] != v {
			return false
		}
	}
	return true
}

func describeSelector(policy *networkingv1.NetworkPolicy) string {
	labels := policy.Spec.PodSelector.MatchLabels
	if len(labels) == 0 {
		return "every pod in the namespace"
	}
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return "pods matching " + strings.Join(pairs, ",")
}

func render(namespace, policy, target, protects string) string {
	return fmt.Sprintf(`apiVersion: secha/v1
kind: Experiment
metadata:
  name: %s-networkpolicy
  hypothesis: >
    Deleting the %s NetworkPolicy opens the network path to %s.
    Whatever sits behind it is the only thing left between an attacker and the
    data, and something should notice the attempt.

safety:
  contexts: []

steadyState:
  checks:
    - type: k8s-tcp-connect
      from:
        namespace: %s
      target: %s
      expect:
        reachable: false

fault:
  type: k8s-networkpolicy
  d3fend: [D3-ITF]
  action: delete
  target:
    namespace: %s
    name: %s

probe:
  type: k8s-tcp-connect
  from:
    namespace: %s
  target: %s

observe:
  window: 10s
  detectors: []

indicators: [exposure, recovery_latency]
`, policy, policy, protects, namespace, target, namespace, policy, namespace, target)
}
