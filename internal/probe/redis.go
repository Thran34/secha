package probe

import (
	"context"
	"fmt"
	"strings"
	"time"

	"secha/internal/experiment"
	"secha/internal/k8s"
)

func init() {
	Register("redis-exfil", newRedisExfilProbe)
}

const probeSeparator = "===SECHA-SEP==="

type redisExfilConfig struct {
	From struct {
		Namespace string `json:"namespace"`
		Pod       string `json:"pod"`
	} `json:"from"`
	Target string `json:"target"`
}

type redisExfilProbe struct {
	client    *k8s.Client
	namespace string
	pod       string
	host      string
	port      string
}

func newRedisExfilProbe(spec experiment.Spec) (Probe, error) {
	var cfg redisExfilConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode redis-exfil probe: %w", err)
	}
	if cfg.From.Namespace == "" || cfg.From.Pod == "" {
		return nil, fmt.Errorf("redis-exfil: from.namespace and from.pod are required")
	}
	host, port, ok := strings.Cut(cfg.Target, ":")
	if !ok {
		return nil, fmt.Errorf("redis-exfil: target must be host:port, got %q", cfg.Target)
	}
	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	return &redisExfilProbe{
		client:    client,
		namespace: cfg.From.Namespace,
		pod:       cfg.From.Pod,
		host:      host,
		port:      port,
	}, nil
}

func (p *redisExfilProbe) Run(ctx context.Context) (Result, error) {
	script := fmt.Sprintf(
		`redis-cli -h %s -p %s -t 3 AUTH wrong-pass-guess 2>&1; echo "%s"; redis-cli -h %s -p %s -t 3 GET secret:flag 2>&1`,
		p.host, p.port, probeSeparator, p.host, p.port,
	)
	stdout, stderr, err := p.client.Exec(ctx, p.namespace, p.pod, "", []string{"sh", "-c", script})
	combined := stdout + stderr
	if err != nil && strings.TrimSpace(combined) == "" {
		return Result{}, fmt.Errorf("exec probe in %s/%s: %w", p.namespace, p.pod, err)
	}

	authPart, getPart, _ := strings.Cut(combined, probeSeparator)
	authPart = strings.TrimSpace(authPart)
	getPart = strings.TrimSpace(getPart)

	res := Result{At: time.Now(), Detail: strings.TrimSpace(combined)}
	res.Reachable = !unreachable(authPart)
	res.DataExposed = res.Reachable && looksLikeData(getPart)
	return res, nil
}

func unreachable(s string) bool {
	for _, needle := range []string{"Could not connect", "Connection refused", "timed out", "No route to host"} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func looksLikeData(s string) bool {
	if s == "" {
		return false
	}
	for _, denied := range []string{"NOAUTH", "WRONGPASS", "NOPERM", "Could not connect", "Connection refused", "timed out"} {
		if strings.Contains(s, denied) {
			return false
		}
	}
	return true
}

func (p *redisExfilProbe) Describe() string {
	return fmt.Sprintf("redis exfil from %s/%s to %s:%s", p.namespace, p.pod, p.host, p.port)
}
