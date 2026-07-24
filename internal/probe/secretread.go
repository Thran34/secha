package probe

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"secha/internal/experiment"
	"secha/internal/k8s"
)

func init() {
	Register("k8s-secret-read", newSecretReadProbe)
}

type secretReadConfig struct {
	As struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"as"`
	Target struct {
		Namespace string `json:"namespace"`
		Secret    string `json:"secret"`
	} `json:"target"`
}

type secretReadProbe struct {
	client   *k8s.Client
	user     string
	targetNS string
	secret   string
}

func newSecretReadProbe(spec experiment.Spec) (Probe, error) {
	var cfg secretReadConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode k8s-secret-read probe: %w", err)
	}
	if cfg.As.Namespace == "" || cfg.As.Name == "" {
		return nil, fmt.Errorf("k8s-secret-read: as.namespace and as.name are required")
	}
	if cfg.Target.Namespace == "" || cfg.Target.Secret == "" {
		return nil, fmt.Errorf("k8s-secret-read: target.namespace and target.secret are required")
	}
	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	return &secretReadProbe{
		client:   client,
		user:     fmt.Sprintf("system:serviceaccount:%s:%s", cfg.As.Namespace, cfg.As.Name),
		targetNS: cfg.Target.Namespace,
		secret:   cfg.Target.Secret,
	}, nil
}

func (p *secretReadProbe) Run(ctx context.Context) (Result, error) {
	cs, err := p.client.ClientsetAs(p.user)
	if err != nil {
		return Result{}, err
	}
	sec, err := cs.CoreV1().Secrets(p.targetNS).Get(ctx, p.secret, metav1.GetOptions{})
	res := Result{At: time.Now()}
	switch {
	case apierrors.IsForbidden(err):
		res.Reachable = true
		res.DataExposed = false
		res.Detail = fmt.Sprintf("%s forbidden from reading secret %s/%s", p.user, p.targetNS, p.secret)
	case err != nil:
		return Result{}, fmt.Errorf("read secret %s/%s as %s: %w", p.targetNS, p.secret, p.user, err)
	default:
		res.Reachable = true
		res.DataExposed = len(sec.Data) > 0
		res.Detail = fmt.Sprintf("%s read secret %s/%s (%d keys)", p.user, p.targetNS, p.secret, len(sec.Data))
	}
	return res, nil
}

func (p *secretReadProbe) Describe() string {
	return fmt.Sprintf("read secret %s/%s as %s", p.targetNS, p.secret, p.user)
}
