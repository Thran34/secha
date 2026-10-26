package fault

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/k8s"
)

func init() {
	Register("k8s-podsecurity", newPodSecurityFault)
}

type podSecurityConfig struct {
	Action string `json:"action"`
	Target struct {
		Namespace string `json:"namespace"`
	} `json:"target"`
	Level string `json:"level"`
}

type podSecurityFault struct {
	client    *k8s.Client
	namespace string
	level     string
	injected  bool
	previous  *string
}

func newPodSecurityFault(spec experiment.Spec) (Fault, error) {
	var cfg podSecurityConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode k8s-podsecurity fault: %w", err)
	}
	if cfg.Action != "relax" {
		return nil, fmt.Errorf("k8s-podsecurity: unsupported action %q, want relax", cfg.Action)
	}
	if cfg.Target.Namespace == "" {
		return nil, fmt.Errorf("k8s-podsecurity: target.namespace is required")
	}
	level := cfg.Level
	if level == "" {
		level = k8s.PodSecurityPrivileged
	}
	if !k8s.IsPodSecurityLevel(level) {
		return nil, fmt.Errorf("k8s-podsecurity: level %q is not one of privileged, baseline, restricted", level)
	}
	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	return &podSecurityFault{client: client, namespace: cfg.Target.Namespace, level: level}, nil
}

func labelPatch(label string, value *string) []byte {
	patch, _ := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"labels": map[string]any{label: value},
		},
	})
	return patch
}

func (f *podSecurityFault) Inject(ctx context.Context) error {
	api := f.client.Clientset.CoreV1().Namespaces()
	ns, err := api.Get(ctx, f.namespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get namespace %s: %w", f.namespace, err)
	}
	current, ok := ns.Labels[k8s.PodSecurityEnforceLabel]
	if !ok {
		return fmt.Errorf("namespace %s enforces no Pod Security level, there is nothing to relax", f.namespace)
	}
	if current == f.level {
		return fmt.Errorf("namespace %s already enforces %s, relaxing to it changes nothing", f.namespace, f.level)
	}

	previous := current
	f.previous = &previous
	level := f.level
	if _, err := api.Patch(ctx, f.namespace, types.MergePatchType, labelPatch(k8s.PodSecurityEnforceLabel, &level), metav1.PatchOptions{}); err != nil {
		if k8s.IsAdmissionDenied(err) {
			return Rejected(err.Error())
		}
		return fmt.Errorf("relax %s on namespace %s: %w", k8s.PodSecurityEnforceLabel, f.namespace, err)
	}
	f.injected = true
	return nil
}

func (f *podSecurityFault) Restore(ctx context.Context) error {
	if !f.injected {
		return nil
	}
	api := f.client.Clientset.CoreV1().Namespaces()
	if _, err := api.Patch(ctx, f.namespace, types.MergePatchType, labelPatch(k8s.PodSecurityEnforceLabel, f.previous), metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("restore %s on namespace %s: %w", k8s.PodSecurityEnforceLabel, f.namespace, err)
	}
	f.injected = false
	return nil
}

func (f *podSecurityFault) Describe() string {
	return fmt.Sprintf("relax pod security on namespace %s to %s", f.namespace, f.level)
}
