package probe

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/k8s"
)

func init() {
	Register("k8s-pod-create", newPodCreateProbe)
}

type podCreateConfig struct {
	As struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"as"`
	Target struct {
		Namespace string `json:"namespace"`
	} `json:"target"`
	Privileged *bool  `json:"privileged"`
	Image      string `json:"image"`
}

type podCreateProbe struct {
	client     *k8s.Client
	saNS       string
	saName     string
	user       string
	namespace  string
	privileged bool
	image      string
}

func newPodCreateProbe(spec experiment.Spec) (Probe, error) {
	var cfg podCreateConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode k8s-pod-create probe: %w", err)
	}
	if cfg.Target.Namespace == "" {
		return nil, fmt.Errorf("k8s-pod-create: target.namespace is required")
	}
	if (cfg.As.Namespace == "") != (cfg.As.Name == "") {
		return nil, fmt.Errorf("k8s-pod-create: as.namespace and as.name go together")
	}
	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	user := "the operator"
	if cfg.As.Name != "" {
		user = fmt.Sprintf("system:serviceaccount:%s:%s", cfg.As.Namespace, cfg.As.Name)
	}
	privileged := true
	if cfg.Privileged != nil {
		privileged = *cfg.Privileged
	}
	image := cfg.Image
	if image == "" {
		image = defaultProbeImage
	}
	return &podCreateProbe{
		client:     client,
		saNS:       cfg.As.Namespace,
		saName:     cfg.As.Name,
		user:       user,
		namespace:  cfg.Target.Namespace,
		privileged: privileged,
		image:      image,
	}, nil
}

func (p *podCreateProbe) attacker(ctx context.Context) (kubernetes.Interface, error) {
	if p.saName == "" {
		return p.client.Clientset, nil
	}
	return p.client.ClientsetForServiceAccount(ctx, p.saNS, p.saName)
}

func (p *podCreateProbe) Run(ctx context.Context) (Result, error) {
	cs, err := p.attacker(ctx)
	if err != nil {
		return Result{}, err
	}

	created, err := cs.CoreV1().Pods(p.namespace).Create(ctx, p.podSpec(), metav1.CreateOptions{})
	res := Result{At: time.Now(), Reachable: true}
	switch {
	case k8s.IsPodSecurityDenied(err):
		res.DataExposed = false
		res.Detail = fmt.Sprintf("pod security admission refused %s a %s pod in %s", p.user, p.kind(), p.namespace)
		return res, nil
	case apierrors.IsForbidden(err):
		return Result{}, fmt.Errorf("%s may not create pods in %s at all, so this probe cannot test admission: %w", p.user, p.namespace, err)
	case err != nil:
		return Result{}, fmt.Errorf("create %s pod in %s as %s: %w", p.kind(), p.namespace, p.user, err)
	}

	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), probeCleanupTimeout)
	defer cancel()
	grace := int64(0)
	_ = p.client.Clientset.CoreV1().Pods(p.namespace).Delete(cleanup, created.Name, metav1.DeleteOptions{GracePeriodSeconds: &grace})

	res.DataExposed = true
	res.Detail = fmt.Sprintf("%s was admitted a %s pod %s/%s", p.user, p.kind(), p.namespace, created.Name)
	return res, nil
}

func (p *podCreateProbe) kind() string {
	if p.privileged {
		return "privileged"
	}
	return "plain"
}

func (p *podCreateProbe) podSpec() *corev1.Pod {
	privileged := p.privileged
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "secha-attack-",
			Namespace:    p.namespace,
			Annotations: map[string]string{
				"secha.dev/probe": "k8s-pod-create",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:            "attack",
				Image:           p.image,
				Command:         []string{"true"},
				SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("16Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("32Mi"),
					},
				},
			}},
		},
	}
}

func (p *podCreateProbe) Describe() string {
	return fmt.Sprintf("create a %s pod in %s as %s", p.kind(), p.namespace, p.user)
}
