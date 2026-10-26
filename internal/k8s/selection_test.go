package k8s

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const twoClusters = `apiVersion: v1
kind: Config
current-context: lab
clusters:
  - name: lab
    cluster: {server: https://lab.example:6443}
  - name: prod
    cluster: {server: https://prod.example:6443}
contexts:
  - name: lab
    context: {cluster: lab, user: u}
  - name: prod
    context: {cluster: prod, user: u}
users:
  - name: u
    user: {token: not-a-real-token}
`

func writeKubeconfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(twoClusters), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSelectedKubeconfigReplacesTheEnvironment(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "does-not-exist"))
	t.Cleanup(func() { Select(Selection{}) })

	Select(Selection{Kubeconfig: writeKubeconfig(t)})
	target, err := CurrentTarget()
	if err != nil {
		t.Fatalf("CurrentTarget() error = %v", err)
	}
	if target.Context != "lab" || target.Server != "https://lab.example:6443" {
		t.Errorf("target = %+v, want the file's current-context lab", target)
	}
}

func TestSelectedContextOverridesCurrentContext(t *testing.T) {
	t.Cleanup(func() { Select(Selection{}) })

	Select(Selection{Kubeconfig: writeKubeconfig(t), Context: "prod"})
	target, err := CurrentTarget()
	if err != nil {
		t.Fatalf("CurrentTarget() error = %v", err)
	}
	if target.Context != "prod" || target.Server != "https://prod.example:6443" {
		t.Errorf("target = %+v, want the prod context, not the file's current-context", target)
	}
}

func TestUnknownContextIsRefusedNotSilentlyIgnored(t *testing.T) {
	t.Cleanup(func() { Select(Selection{}) })

	Select(Selection{Kubeconfig: writeKubeconfig(t), Context: "staging"})
	_, err := CurrentTarget()
	if err == nil || !strings.Contains(err.Error(), "staging") {
		t.Errorf("CurrentTarget() error = %v, want a refusal naming the missing context", err)
	}
}
