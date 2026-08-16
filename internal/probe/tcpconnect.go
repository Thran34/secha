package probe

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/k8s"
)

func init() {
	Register("k8s-tcp-connect", newTCPConnectProbe)
}

const (
	defaultProbeImage   = "busybox:1.36"
	defaultDialTimeout  = 5 * time.Second
	podSettleTimeout    = 90 * time.Second
	podPollInterval     = 500 * time.Millisecond
	probeCleanupTimeout = 30 * time.Second
)

type tcpConnectConfig struct {
	From struct {
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels"`
	} `json:"from"`
	Target  string `json:"target"`
	Timeout string `json:"timeout"`
	Image   string `json:"image"`
}

type tcpConnectProbe struct {
	client    *k8s.Client
	namespace string
	labels    map[string]string
	host      string
	port      string
	dial      time.Duration
	image     string
}

func newTCPConnectProbe(spec experiment.Spec) (Probe, error) {
	var cfg tcpConnectConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode k8s-tcp-connect probe: %w", err)
	}
	if cfg.From.Namespace == "" {
		return nil, fmt.Errorf("k8s-tcp-connect: from.namespace is required, it decides which policies apply")
	}
	if cfg.Target == "" {
		return nil, fmt.Errorf("k8s-tcp-connect: target is required, in host:port form")
	}
	host, port, err := net.SplitHostPort(cfg.Target)
	if err != nil {
		return nil, fmt.Errorf("k8s-tcp-connect: target %q must be host:port: %w", cfg.Target, err)
	}
	if _, err := strconv.Atoi(port); err != nil {
		return nil, fmt.Errorf("k8s-tcp-connect: target port %q is not a number", port)
	}

	dial := defaultDialTimeout
	if cfg.Timeout != "" {
		dial, err = time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("k8s-tcp-connect: timeout: %w", err)
		}
	}

	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	image := cfg.Image
	if image == "" {
		image = defaultProbeImage
	}
	return &tcpConnectProbe{
		client:    client,
		namespace: cfg.From.Namespace,
		labels:    cfg.From.Labels,
		host:      host,
		port:      port,
		dial:      dial,
		image:     image,
	}, nil
}

func (p *tcpConnectProbe) Run(ctx context.Context) (Result, error) {
	api := p.client.Clientset.CoreV1().Pods(p.namespace)

	created, err := api.Create(ctx, p.podSpec(), metav1.CreateOptions{})
	if err != nil {
		return Result{}, fmt.Errorf("create probe pod in %s: %w", p.namespace, err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), probeCleanupTimeout)
		defer cancel()
		grace := int64(0)
		_ = api.Delete(cleanup, created.Name, metav1.DeleteOptions{GracePeriodSeconds: &grace})
	}()

	phase, err := p.waitForExit(ctx, created.Name)
	if err != nil {
		return Result{}, err
	}

	res := Result{At: time.Now(), Reachable: phase == corev1.PodSucceeded}
	if res.Reachable {
		res.Detail = fmt.Sprintf("opened a tcp connection to %s:%s from %s", p.host, p.port, p.namespace)
	} else {
		res.Detail = fmt.Sprintf("could not reach %s:%s from %s within %s", p.host, p.port, p.namespace, p.dial)
	}
	return res, nil
}

func (p *tcpConnectProbe) podSpec() *corev1.Pod {
	seconds := int(p.dial.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "secha-probe-",
			Namespace:    p.namespace,
			Labels:       p.labels,
			Annotations: map[string]string{
				"secha.dev/probe": "k8s-tcp-connect",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    "probe",
				Image:   p.image,
				Command: []string{"nc", "-z", "-w", strconv.Itoa(seconds), p.host, p.port},
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

func (p *tcpConnectProbe) waitForExit(ctx context.Context, name string) (corev1.PodPhase, error) {
	api := p.client.Clientset.CoreV1().Pods(p.namespace)
	deadline := time.Now().Add(podSettleTimeout)
	for {
		pod, err := api.Get(ctx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return "", fmt.Errorf("get probe pod %s/%s: %w", p.namespace, name, err)
		}
		if err == nil {
			switch pod.Status.Phase {
			case corev1.PodSucceeded, corev1.PodFailed:
				return pod.Status.Phase, nil
			}
			if reason, stuck := imagePullFailure(pod); stuck {
				return "", fmt.Errorf("probe pod %s/%s cannot start: %s", p.namespace, name, reason)
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("probe pod %s/%s did not finish within %s", p.namespace, name, podSettleTimeout)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(podPollInterval):
		}
	}
}

func imagePullFailure(pod *corev1.Pod) (string, bool) {
	for _, cs := range pod.Status.ContainerStatuses {
		w := cs.State.Waiting
		if w == nil {
			continue
		}
		if w.Reason == "ImagePullBackOff" || w.Reason == "ErrImagePull" || w.Reason == "InvalidImageName" {
			return w.Reason + ": " + w.Message, true
		}
	}
	return "", false
}

func (p *tcpConnectProbe) Describe() string {
	return fmt.Sprintf("tcp connect to %s:%s from a throwaway pod in %s", p.host, p.port, p.namespace)
}
