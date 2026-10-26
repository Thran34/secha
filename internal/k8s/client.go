package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

type Client struct {
	Clientset *kubernetes.Clientset
	config    *rest.Config
}

type Target struct {
	Context string
	Server  string
}

func (t Target) String() string {
	if t.Context == "" {
		return t.Server
	}
	return fmt.Sprintf("%s (%s)", t.Context, t.Server)
}

type Selection struct {
	Kubeconfig string
	Context    string
}

var selected Selection

func Select(s Selection) {
	selected = s
}

func loader() clientcmd.ClientConfig {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = selected.Kubeconfig
	overrides := &clientcmd.ConfigOverrides{CurrentContext: selected.Context}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
}

func CurrentTarget() (Target, error) {
	loader := loader()
	cfg, err := loader.ClientConfig()
	if err != nil {
		return Target{}, fmt.Errorf("load kubeconfig: %w", err)
	}
	raw, err := loader.RawConfig()
	if err != nil {
		return Target{Server: cfg.Host}, nil
	}
	context := raw.CurrentContext
	if selected.Context != "" {
		context = selected.Context
	}
	return Target{Context: context, Server: cfg.Host}, nil
}

func NewClient() (*Client, error) {
	cfg, err := loader().ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	return &Client{Clientset: cs, config: cfg}, nil
}

func (c *Client) ClientsetForServiceAccount(ctx context.Context, namespace, name string) (*kubernetes.Clientset, error) {
	expiration := int64(600)
	req := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: &expiration},
	}
	token, err := c.Clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, name, req, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("request token for serviceaccount %s/%s: %w", namespace, name, err)
	}

	cfg := rest.AnonymousClientConfig(c.config)
	cfg.BearerToken = token.Status.Token
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build clientset for serviceaccount %s/%s: %w", namespace, name, err)
	}
	return cs, nil
}

func (c *Client) Exec(ctx context.Context, namespace, pod, container string, command []string) (string, string, error) {
	if container == "" {
		p, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, pod, metav1.GetOptions{})
		if err != nil {
			return "", "", fmt.Errorf("get pod %s/%s: %w", namespace, pod, err)
		}
		if len(p.Spec.Containers) == 0 {
			return "", "", fmt.Errorf("pod %s/%s has no containers", namespace, pod)
		}
		container = p.Spec.Containers[0].Name
	}

	req := c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("build executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	return stdout.String(), stderr.String(), err
}

func (c *Client) PodLogsSince(ctx context.Context, namespace, pod, container string, since time.Time) (string, error) {
	opts := &corev1.PodLogOptions{Container: container}
	if !since.IsZero() {
		t := metav1.NewTime(since)
		opts.SinceTime = &t
	}
	stream, err := c.Clientset.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("get logs %s/%s: %w", namespace, pod, err)
	}
	defer stream.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, stream); err != nil {
		return "", fmt.Errorf("read logs %s/%s: %w", namespace, pod, err)
	}
	return buf.String(), nil
}

func (c *Client) FirstRunningPod(ctx context.Context, namespace, selector string) (string, error) {
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return "", fmt.Errorf("list pods in %s (%s): %w", namespace, selector, err)
	}
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			return p.Name, nil
		}
	}
	return "", fmt.Errorf("no running pod in %s matching %q", namespace, selector)
}
