package probe

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/k8s"
)

func init() {
	Register("redis-key-present", newRedisKeyPresentProbe)
}

type redisKeyPresentConfig struct {
	Target struct {
		Namespace string `json:"namespace"`
		Selector  string `json:"selector"`
	} `json:"target"`
	Key string `json:"key"`
}

type redisKeyPresentProbe struct {
	client    *k8s.Client
	namespace string
	selector  string
	key       string
}

func newRedisKeyPresentProbe(spec experiment.Spec) (Probe, error) {
	var cfg redisKeyPresentConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode redis-key-present probe: %w", err)
	}
	if cfg.Target.Namespace == "" || cfg.Target.Selector == "" {
		return nil, fmt.Errorf("redis-key-present: target.namespace and target.selector are required")
	}
	if cfg.Key == "" {
		return nil, fmt.Errorf("redis-key-present: key is required")
	}
	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	return &redisKeyPresentProbe{
		client:    client,
		namespace: cfg.Target.Namespace,
		selector:  cfg.Target.Selector,
		key:       cfg.Key,
	}, nil
}

func (p *redisKeyPresentProbe) Run(ctx context.Context) (Result, error) {
	pod, err := p.client.FirstRunningPod(ctx, p.namespace, p.selector)
	if err != nil {
		return Result{}, err
	}
	script := fmt.Sprintf(
		`AUTH=""; [ -n "$REDIS_PASSWORD" ] && AUTH="-a $REDIS_PASSWORD --no-auth-warning"; redis-cli $AUTH GET '%s'`,
		p.key,
	)
	stdout, stderr, err := p.client.Exec(ctx, p.namespace, pod, "", []string{"sh", "-c", script})
	if err != nil {
		return Result{}, fmt.Errorf("exec redis-cli in %s/%s: %w (%s)", p.namespace, pod, err, strings.TrimSpace(stderr))
	}

	value := strings.TrimSpace(stdout)
	present := value != "" && !strings.Contains(value, "NOAUTH") && !strings.Contains(value, "WRONGPASS")

	res := Result{At: time.Now(), Reachable: true, DataExposed: present}
	if present {
		res.Detail = fmt.Sprintf("key %s present (%d bytes)", p.key, len(value))
	} else {
		res.Detail = fmt.Sprintf("key %s missing or unreadable", p.key)
	}
	return res, nil
}

func (p *redisKeyPresentProbe) Describe() string {
	return fmt.Sprintf("redis key %s in %s (%s)", p.key, p.namespace, p.selector)
}
