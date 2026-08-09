package detector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/k8s"
)

func init() {
	Register("falco-alert", newFalcoAlertDetector)
}

const (
	defaultFalcoNamespace = "falco"
	defaultFalcoSelector  = "app.kubernetes.io/name=falco"
	defaultFalcoContainer = "falco"
	defaultFalcoInterval  = 200 * time.Millisecond
)

type falcoAlertConfig struct {
	Namespace string            `json:"namespace"`
	Selector  string            `json:"selector"`
	Container string            `json:"container"`
	Rules     []string          `json:"rules"`
	Match     map[string]string `json:"match"`
	Interval  string            `json:"interval"`
}

type falcoAlert struct {
	Rule         string          `json:"rule"`
	Time         time.Time       `json:"time"`
	Output       string          `json:"output"`
	Priority     string          `json:"priority"`
	Source       string          `json:"source"`
	OutputFields json.RawMessage `json:"output_fields"`
}

type falcoAlertDetector struct {
	client    *k8s.Client
	namespace string
	selector  string
	container string
	rules     []string
	match     map[string]string
	interval  time.Duration
	since     time.Time
}

func newFalcoAlertDetector(spec experiment.Spec) (Detector, error) {
	var cfg falcoAlertConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode falco-alert detector: %w", err)
	}
	if len(cfg.Rules) == 0 {
		return nil, fmt.Errorf("falco-alert: rules are required, an unfiltered detector would fire on unrelated activity")
	}

	interval := defaultFalcoInterval
	if cfg.Interval != "" {
		parsed, err := time.ParseDuration(cfg.Interval)
		if err != nil {
			return nil, fmt.Errorf("falco-alert: interval: %w", err)
		}
		interval = parsed
	}

	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	return &falcoAlertDetector{
		client:    client,
		namespace: orDefault(cfg.Namespace, defaultFalcoNamespace),
		selector:  orDefault(cfg.Selector, defaultFalcoSelector),
		container: orDefault(cfg.Container, defaultFalcoContainer),
		rules:     cfg.Rules,
		match:     cfg.Match,
		interval:  interval,
	}, nil
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func (d *falcoAlertDetector) Reset(ctx context.Context) error {
	d.since = time.Now()
	return nil
}

func (d *falcoAlertDetector) Observe(ctx context.Context, window time.Duration) (Signal, error) {
	pod, err := d.client.FirstRunningPod(ctx, d.namespace, d.selector)
	if err != nil {
		return Signal{}, fmt.Errorf("falco-alert: %w", err)
	}

	deadline := time.Now().Add(window)
	for {
		logs, err := d.client.PodLogsSince(ctx, d.namespace, pod, d.container, d.since)
		if err != nil {
			return Signal{}, err
		}
		if alert, ok := d.firstMatch(logs); ok {
			return Signal{
				Detected: true,
				At:       time.Now(),
				Source:   d.Describe(),
				Detail:   fmt.Sprintf("%s at %s: %s", alert.Rule, alert.Time.Format(time.RFC3339Nano), alert.Output),
			}, nil
		}
		if time.Now().After(deadline) {
			return Signal{}, nil
		}
		select {
		case <-ctx.Done():
			return Signal{}, ctx.Err()
		case <-time.After(d.interval):
		}
	}
}

func (d *falcoAlertDetector) firstMatch(logs string) (falcoAlert, bool) {
	for _, line := range strings.Split(logs, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var alert falcoAlert
		if err := json.Unmarshal([]byte(line), &alert); err != nil {
			continue
		}
		if alert.Rule == "" || !alert.Time.After(d.since) {
			continue
		}
		if !contains(d.rules, alert.Rule) {
			continue
		}
		if d.fieldsMatch(alert) {
			return alert, true
		}
	}
	return falcoAlert{}, false
}

func (d *falcoAlertDetector) fieldsMatch(alert falcoAlert) bool {
	if len(d.match) == 0 {
		return true
	}
	var fields map[string]any
	if err := json.Unmarshal(alert.OutputFields, &fields); err != nil {
		return false
	}
	for key, want := range d.match {
		got, ok := fields[key]
		if !ok || fmt.Sprint(got) != want {
			return false
		}
	}
	return true
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

func (d *falcoAlertDetector) Describe() string {
	return fmt.Sprintf("falco alert %s in %s", strings.Join(d.rules, "/"), d.namespace)
}
