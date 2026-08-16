package scaffold

import (
	"strings"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func policyWithPorts(ports ...int) *networkingv1.NetworkPolicy {
	var rules []networkingv1.NetworkPolicyIngressRule
	for _, p := range ports {
		port := intstr.FromInt32(int32(p))
		rules = append(rules, networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{{Port: &port}},
		})
	}
	return &networkingv1.NetworkPolicy{Spec: networkingv1.NetworkPolicySpec{Ingress: rules}}
}

func TestFirstPortTakesThePolicysOwnPort(t *testing.T) {
	if got := firstPort(policyWithPorts(6379, 8080)); got != 6379 {
		t.Errorf("firstPort = %d, want 6379", got)
	}
}

func TestFirstPortIsZeroWhenThePolicyNamesNone(t *testing.T) {
	if got := firstPort(policyWithPorts()); got != 0 {
		t.Errorf("firstPort = %d, want 0 so the caller can ask for one", got)
	}
}

func TestCoversMatchesAServiceThatSelectsTheProtectedPods(t *testing.T) {
	policy := map[string]string{"app": "redis", "tier": "db"}

	cases := map[string]struct {
		service map[string]string
		want    bool
	}{
		"service selects a subset":        {map[string]string{"app": "redis"}, true},
		"service selects the same labels": {map[string]string{"app": "redis", "tier": "db"}, true},
		"different app":                   {map[string]string{"app": "web"}, false},
		"one label right, one wrong":      {map[string]string{"app": "redis", "tier": "cache"}, false},
		"service selects something extra": {map[string]string{"app": "redis", "zone": "a"}, false},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := covers(policy, c.service); got != c.want {
				t.Errorf("covers = %t, want %t", got, c.want)
			}
		})
	}
}

func TestDescribeSelectorIsStableAndReadable(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{}
	policy.Spec.PodSelector.MatchLabels = map[string]string{"tier": "db", "app": "redis"}

	got := describeSelector(policy)

	if got != "pods matching app=redis,tier=db" {
		t.Errorf("describeSelector = %q, want the labels sorted so the output does not shuffle", got)
	}
}

func TestDescribeSelectorSaysWhenThePolicyCoversEverything(t *testing.T) {
	if got := describeSelector(&networkingv1.NetworkPolicy{}); !strings.Contains(got, "every pod") {
		t.Errorf("describeSelector = %q, want it to say the policy selects every pod", got)
	}
}

func TestRenderProducesAnExperimentThatNamesTheRealTarget(t *testing.T) {
	out := render("prod", "redis-allow-web-only", "redis.prod.svc.cluster.local:6379", "pods matching app=redis")

	for _, want := range []string{
		"kind: Experiment",
		"type: k8s-tcp-connect",
		"type: k8s-networkpolicy",
		"namespace: prod",
		"name: redis-allow-web-only",
		"redis.prod.svc.cluster.local:6379",
		"reachable: false",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated experiment is missing %q", want)
		}
	}
}

func TestRenderUsesTheGenericProbeSoNothingNeedsDeploying(t *testing.T) {
	out := render("prod", "p", "svc.prod.svc.cluster.local:1234", "pods")

	if strings.Contains(out, "redis-exfil") || strings.Contains(out, "pod: attacker") {
		t.Error("scaffolding must not reference a lab the user does not have")
	}
}
