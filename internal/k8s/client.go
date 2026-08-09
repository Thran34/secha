package k8s

import (
	"bytes"
	"context"
	"fmt"

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

func CurrentTarget() (Target, error) {
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})

	cfg, err := loader.ClientConfig()
	if err != nil {
		return Target{}, fmt.Errorf("load kubeconfig: %w", err)
	}
	raw, err := loader.RawConfig()
	if err != nil {
		return Target{Server: cfg.Host}, nil
	}
	return Target{Context: raw.CurrentContext, Server: cfg.Host}, nil
}

func NewClient() (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	return &Client{Clientset: cs, config: cfg}, nil
}

func (c *Client) ClientsetAs(user string) (*kubernetes.Clientset, error) {
	cfg := rest.CopyConfig(c.config)
	cfg.Impersonate = rest.ImpersonationConfig{UserName: user}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build impersonating clientset for %q: %w", user, err)
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
