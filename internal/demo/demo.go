package demo

import (
	"context"
	"embed"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/Thran34/secha/internal/experiment"
	"github.com/Thran34/secha/internal/k8s"
)

//go:embed assets/*.yaml
var assets embed.FS

const (
	Namespace     = "secha-demo"
	readyTimeout  = 3 * time.Minute
	deleteTimeout = 2 * time.Minute
	pollInterval  = time.Second
)

type Runner struct {
	client *k8s.Client
	out    io.Writer
}

func New(out io.Writer) (*Runner, error) {
	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	return &Runner{client: client, out: out}, nil
}

func Experiments() (baseline, treatment *experiment.Experiment, err error) {
	treatment, err = parseAsset("assets/exp-auth.yaml")
	if err != nil {
		return nil, nil, err
	}
	baseline, err = parseAsset("assets/exp-noauth.yaml")
	if err != nil {
		return nil, nil, err
	}
	return baseline, treatment, nil
}

func parseAsset(name string) (*experiment.Experiment, error) {
	b, err := assets.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read embedded %s: %w", name, err)
	}
	return experiment.Parse(b)
}

func (r *Runner) Setup(ctx context.Context) error {
	if err := r.removeStaleNamespace(ctx); err != nil {
		return err
	}

	names, err := manifestNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		b, err := assets.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", name, err)
		}
		if err := r.applyDocuments(ctx, b); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return r.waitReady(ctx)
}

func (r *Runner) Teardown(ctx context.Context) error {
	api := r.client.Clientset.CoreV1().Namespaces()
	if err := api.Delete(ctx, Namespace, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete namespace %s: %w", Namespace, err)
	}
	return r.waitNamespaceGone(ctx)
}

func (r *Runner) removeStaleNamespace(ctx context.Context) error {
	api := r.client.Clientset.CoreV1().Namespaces()
	if _, err := api.Get(ctx, Namespace, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("look for namespace %s: %w", Namespace, err)
	}
	fmt.Fprintf(r.out, "  found a leftover %s namespace, removing it first\n", Namespace)
	return r.Teardown(ctx)
}

func (r *Runner) waitNamespaceGone(ctx context.Context) error {
	api := r.client.Clientset.CoreV1().Namespaces()
	deadline := time.Now().Add(deleteTimeout)
	for {
		_, err := api.Get(ctx, Namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("namespace %s still present after %s", Namespace, deleteTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func manifestNames() ([]string, error) {
	entries, err := assets.ReadDir("assets")
	if err != nil {
		return nil, fmt.Errorf("read embedded assets: %w", err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "exp-") {
			continue
		}
		names = append(names, "assets/"+e.Name())
	}
	sort.Strings(names)
	return names, nil
}

func (r *Runner) applyDocuments(ctx context.Context, raw []byte) error {
	for _, doc := range strings.Split(string(raw), "\n---\n") {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		obj, _, err := scheme.Codecs.UniversalDeserializer().Decode([]byte(doc), nil, nil)
		if err != nil {
			return fmt.Errorf("decode manifest: %w", err)
		}
		if err := r.create(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) create(ctx context.Context, obj any) error {
	cs := r.client.Clientset
	var err error
	switch o := obj.(type) {
	case *corev1.Namespace:
		_, err = cs.CoreV1().Namespaces().Create(ctx, o, metav1.CreateOptions{})
	case *corev1.Secret:
		_, err = cs.CoreV1().Secrets(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
	case *corev1.Service:
		_, err = cs.CoreV1().Services(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
	case *corev1.Pod:
		_, err = cs.CoreV1().Pods(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
	case *appsv1.Deployment:
		_, err = cs.AppsV1().Deployments(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
	case *networkingv1.NetworkPolicy:
		_, err = cs.NetworkingV1().NetworkPolicies(o.Namespace).Create(ctx, o, metav1.CreateOptions{})
	default:
		return fmt.Errorf("demo manifests contain an unsupported kind %T", obj)
	}
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (r *Runner) waitReady(ctx context.Context) error {
	fmt.Fprintf(r.out, "  waiting for the demo workloads to become ready\n")
	deadline := time.Now().Add(readyTimeout)
	for {
		ready, detail, err := r.allReady(ctx)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("demo workloads not ready after %s: %s", readyTimeout, detail)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (r *Runner) allReady(ctx context.Context) (bool, string, error) {
	cs := r.client.Clientset
	for _, name := range []string{"redis", "redis-open"} {
		d, err := cs.AppsV1().Deployments(Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, "", fmt.Errorf("get deployment %s: %w", name, err)
		}
		if d.Status.ReadyReplicas < 1 {
			return false, "deployment " + name + " has no ready replica", nil
		}
	}
	pod, err := cs.CoreV1().Pods(Namespace).Get(ctx, "attacker", metav1.GetOptions{})
	if err != nil {
		return false, "", fmt.Errorf("get pod attacker: %w", err)
	}
	if pod.Status.Phase != corev1.PodRunning {
		return false, "attacker pod is " + string(pod.Status.Phase), nil
	}
	return true, "", nil
}
