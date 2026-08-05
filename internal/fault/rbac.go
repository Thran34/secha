package fault

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/k8s"
)

func init() {
	Register("k8s-rbac", newRBACFault)
}

type rbacConfig struct {
	Action  string `json:"action"`
	Subject struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"subject"`
	Grant struct {
		Namespace string   `json:"namespace"`
		Resources []string `json:"resources"`
		Verbs     []string `json:"verbs"`
	} `json:"grant"`
}

type rbacFault struct {
	client      *k8s.Client
	subjectNS   string
	subjectName string
	grantNS     string
	resources   []string
	verbs       []string
	objName     string
}

func newRBACFault(spec experiment.Spec) (Fault, error) {
	var cfg rbacConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode k8s-rbac fault: %w", err)
	}
	if cfg.Action != "grant" {
		return nil, fmt.Errorf("k8s-rbac: unsupported action %q, want grant", cfg.Action)
	}
	if cfg.Subject.Namespace == "" || cfg.Subject.Name == "" {
		return nil, fmt.Errorf("k8s-rbac: subject.namespace and subject.name are required")
	}
	if cfg.Grant.Namespace == "" || len(cfg.Grant.Resources) == 0 || len(cfg.Grant.Verbs) == 0 {
		return nil, fmt.Errorf("k8s-rbac: grant.namespace, grant.resources and grant.verbs are required")
	}
	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	return &rbacFault{
		client:      client,
		subjectNS:   cfg.Subject.Namespace,
		subjectName: cfg.Subject.Name,
		grantNS:     cfg.Grant.Namespace,
		resources:   cfg.Grant.Resources,
		verbs:       cfg.Grant.Verbs,
		objName:     "secha-escalation-" + cfg.Subject.Name,
	}, nil
}

func (f *rbacFault) Inject(ctx context.Context) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: f.objName, Namespace: f.grantNS},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: f.resources,
			Verbs:     f.verbs,
		}},
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: f.objName, Namespace: f.grantNS},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      f.subjectName,
			Namespace: f.subjectNS,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     f.objName,
		},
	}

	roles := f.client.Clientset.RbacV1().Roles(f.grantNS)
	if _, err := roles.Create(ctx, role, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		if k8s.IsAdmissionDenied(err) {
			return Rejected(err.Error())
		}
		return fmt.Errorf("create role %s/%s: %w", f.grantNS, f.objName, err)
	}
	bindings := f.client.Clientset.RbacV1().RoleBindings(f.grantNS)
	if _, err := bindings.Create(ctx, rb, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		if k8s.IsAdmissionDenied(err) {
			return Rejected(err.Error())
		}
		return fmt.Errorf("create rolebinding %s/%s: %w", f.grantNS, f.objName, err)
	}
	return nil
}

func (f *rbacFault) Restore(ctx context.Context) error {
	bindings := f.client.Clientset.RbacV1().RoleBindings(f.grantNS)
	if err := bindings.Delete(ctx, f.objName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete rolebinding %s/%s: %w", f.grantNS, f.objName, err)
	}
	roles := f.client.Clientset.RbacV1().Roles(f.grantNS)
	if err := roles.Delete(ctx, f.objName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete role %s/%s: %w", f.grantNS, f.objName, err)
	}
	return nil
}

func (f *rbacFault) Describe() string {
	return fmt.Sprintf("grant %v on %v to serviceaccount %s/%s in %s",
		f.verbs, f.resources, f.subjectNS, f.subjectName, f.grantNS)
}
