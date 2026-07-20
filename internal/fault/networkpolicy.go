package fault

import (
	"context"
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"secha/internal/experiment"
	"secha/internal/k8s"
)

func init() {
	Register("k8s-networkpolicy", newNetworkPolicyFault)
}

type networkPolicyConfig struct {
	Action string `json:"action"`
	Target struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"target"`
}

type networkPolicyFault struct {
	client    *k8s.Client
	namespace string
	name      string
	saved     *networkingv1.NetworkPolicy
}

func newNetworkPolicyFault(spec experiment.Spec) (Fault, error) {
	var cfg networkPolicyConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode k8s-networkpolicy fault: %w", err)
	}
	if cfg.Action != "delete" {
		return nil, fmt.Errorf("k8s-networkpolicy: unsupported action %q, want delete", cfg.Action)
	}
	if cfg.Target.Namespace == "" || cfg.Target.Name == "" {
		return nil, fmt.Errorf("k8s-networkpolicy: target.namespace and target.name are required")
	}
	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	return &networkPolicyFault{
		client:    client,
		namespace: cfg.Target.Namespace,
		name:      cfg.Target.Name,
	}, nil
}

func (f *networkPolicyFault) Inject(ctx context.Context) error {
	api := f.client.Clientset.NetworkingV1().NetworkPolicies(f.namespace)
	np, err := api.Get(ctx, f.name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get networkpolicy %s/%s: %w", f.namespace, f.name, err)
	}

	saved := np.DeepCopy()
	saved.ResourceVersion = ""
	saved.UID = ""
	saved.Generation = 0
	saved.CreationTimestamp = metav1.Time{}
	saved.ManagedFields = nil
	f.saved = saved

	if err := api.Delete(ctx, f.name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("delete networkpolicy %s/%s: %w", f.namespace, f.name, err)
	}
	return nil
}

func (f *networkPolicyFault) Restore(ctx context.Context) error {
	if f.saved == nil {
		return nil
	}
	api := f.client.Clientset.NetworkingV1().NetworkPolicies(f.namespace)
	_, err := api.Create(ctx, f.saved, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("restore networkpolicy %s/%s: %w", f.namespace, f.name, err)
	}
	f.saved = nil
	return nil
}

func (f *networkPolicyFault) Describe() string {
	return fmt.Sprintf("delete networkpolicy %s/%s", f.namespace, f.name)
}
