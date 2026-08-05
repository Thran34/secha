package detector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/k8s"
)

func init() {
	Register("redis-acl-log", newACLLogDetector)
}

const aclLogPollInterval = 250 * time.Millisecond

type aclLogConfig struct {
	Target struct {
		Namespace string `json:"namespace"`
		Selector  string `json:"selector"`
	} `json:"target"`
}

type aclLogDetector struct {
	client    *k8s.Client
	namespace string
	selector  string
}

func newACLLogDetector(spec experiment.Spec) (Detector, error) {
	var cfg aclLogConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode redis-acl-log detector: %w", err)
	}
	if cfg.Target.Namespace == "" || cfg.Target.Selector == "" {
		return nil, fmt.Errorf("redis-acl-log: target.namespace and target.selector are required")
	}
	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	return &aclLogDetector{
		client:    client,
		namespace: cfg.Target.Namespace,
		selector:  cfg.Target.Selector,
	}, nil
}

func (d *aclLogDetector) redisCLI(ctx context.Context, args string) (string, error) {
	pod, err := d.client.FirstRunningPod(ctx, d.namespace, d.selector)
	if err != nil {
		return "", err
	}
	script := fmt.Sprintf(
		`AUTH=""; [ -n "$REDIS_PASSWORD" ] && AUTH="-a $REDIS_PASSWORD --no-auth-warning"; redis-cli $AUTH %s`,
		args,
	)
	stdout, stderr, err := d.client.Exec(ctx, d.namespace, pod, "", []string{"sh", "-c", script})
	if err != nil {
		return "", fmt.Errorf("exec redis-cli in %s/%s: %w (%s)", d.namespace, pod, err, strings.TrimSpace(stderr))
	}
	return stdout, nil
}

func (d *aclLogDetector) Reset(ctx context.Context) error {
	_, err := d.redisCLI(ctx, "ACL LOG RESET")
	return err
}

func (d *aclLogDetector) Observe(ctx context.Context, window time.Duration) (Signal, error) {
	deadline := time.Now().Add(window)
	for {
		out, err := d.redisCLI(ctx, "ACL LOG")
		if err != nil {
			return Signal{}, err
		}
		if isAuthDenial(out) {
			return Signal{
				Detected: true,
				At:       time.Now(),
				Source:   "redis-acl-log",
				Detail:   "auth denial recorded in redis ACL LOG",
			}, nil
		}
		if time.Now().After(deadline) {
			return Signal{Detected: false, Source: "redis-acl-log"}, nil
		}
		select {
		case <-ctx.Done():
			return Signal{}, ctx.Err()
		case <-time.After(aclLogPollInterval):
		}
	}
}

func isAuthDenial(aclLog string) bool {
	return strings.Contains(aclLog, "reason") && strings.Contains(aclLog, "auth")
}

func (d *aclLogDetector) Describe() string {
	return fmt.Sprintf("redis ACL LOG in %s (%s)", d.namespace, d.selector)
}
