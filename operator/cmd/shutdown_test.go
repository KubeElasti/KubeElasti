package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// These tests run the real operator binary against an envtest API server:
// how the process ends is the contract under test, and only a real process
// shows whether it exits.

var (
	buildOnce   sync.Once
	operatorBin string
	buildErr    error
)

func operatorBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "elasti-operator-bin")
		if err != nil {
			buildErr = err
			return
		}
		operatorBin = filepath.Join(dir, "operator")
		out, err := exec.Command("go", "build", "-o", operatorBin, ".").CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return operatorBin
}

// apiServer starts envtest with the ElastiService CRD and returns the path
// of an admin kubeconfig for it.
func apiServer(t *testing.T) string {
	t.Helper()
	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: filepath.Join("..", "bin", "k8s",
			fmt.Sprintf("1.29.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}
	if _, err := env.Start(); err != nil {
		t.Fatalf("envtest: %v", err)
	}
	t.Cleanup(func() { _ = env.Stop() })
	kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(kubeconfig, env.KubeConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	return kubeconfig
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// syncBuffer is a bytes.Buffer safe to read while the process writes to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type operatorProcess struct {
	cmd    *exec.Cmd
	output *syncBuffer
	exited chan error
}

func startOperator(t *testing.T, kubeconfig string, args ...string) *operatorProcess {
	t.Helper()
	cmd := exec.Command(operatorBinary(t), args...)
	cmd.Env = append(os.Environ(),
		"KUBECONFIG="+kubeconfig,
		"KUBERNETES_CLUSTER_DOMAIN=cluster.local",
		"ELASTI_OPERATOR_NAMESPACE=elasti",
		"ELASTI_OPERATOR_DEPLOYMENT_NAME=elasti-operator",
		"ELASTI_OPERATOR_SERVICE_NAME=elasti-operator",
		fmt.Sprintf("ELASTI_OPERATOR_PORT=%d", freePort(t)),
		"ELASTI_RESOLVER_NAMESPACE=elasti",
		"ELASTI_RESOLVER_DEPLOYMENT_NAME=elasti-resolver",
		"ELASTI_RESOLVER_SERVICE_NAME=elasti-resolver",
		"ELASTI_RESOLVER_PORT=8012",
		"ELASTI_RESOLVER_PROXY_PORT=8013",
	)
	p := &operatorProcess{cmd: cmd, output: &syncBuffer{}, exited: make(chan error, 1)}
	cmd.Stdout, cmd.Stderr = p.output, p.output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { p.exited <- cmd.Wait() }()
	t.Cleanup(func() {
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("kill operator: %v", err)
		}
		if t.Failed() {
			t.Logf("operator output:\n%s", p.output.String())
		}
	})
	return p
}

// waitExit returns the process's exit code, or fails if it is still running
// after d.
func (p *operatorProcess) waitExit(t *testing.T, d time.Duration) int {
	t.Helper()
	select {
	case err := <-p.exited:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
		return 0
	case <-time.After(d):
		t.Fatalf("operator still running %s later", d)
		return -1
	}
}

// Kubernetes stops a pod with SIGTERM and SIGKILLs it after the grace
// period; an operator that doesn't exit on SIGTERM holds up every rollout
// and drain for the whole period, and ends as if it crashed.
func TestOperatorExitsCleanlyOnSIGTERM(t *testing.T) {
	p := startOperator(t, apiServer(t), "--metrics-bind-address=0", "--health-probe-bind-address=0")

	deadline := time.Now().Add(2 * time.Minute)
	for !strings.Contains(p.output.String(), "initialized controller") {
		select {
		case err := <-p.exited:
			t.Fatalf("operator exited before it was up: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("operator never logged \"initialized controller\"")
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := p.waitExit(t, 10*time.Second); code != 0 {
		t.Fatalf("exit code after SIGTERM = %d, want 0", code)
	}
}

// When the manager can't start, the operator must exit with an error
// rather than wait forever for a cache that will never start.
func TestOperatorExitsWhenTheManagerCannotStart(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()

	p := startOperator(t, apiServer(t),
		"--metrics-bind-address="+taken.Addr().String(), "--health-probe-bind-address=0")
	if code := p.waitExit(t, 30*time.Second); code == 0 {
		t.Fatal("exit code = 0 after the manager failed to start, want non-zero")
	}
}
