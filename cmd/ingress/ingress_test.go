package ingress

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/mumoshu/testkit"

	"github.com/sirupsen/logrus"
)

func TestNewIngress(t *testing.T) {
	t.Parallel()
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{}
	checker := cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 3*time.Minute)
	ingress, err := NewIngress(checker, false)
	if err != nil {
		t.Fatalf("NewIngress: %s", err)
	}

	if ingress == nil {
		t.Error("Expected ingress instance, got nil")
	}
}

func TestCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	t.Parallel()
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}

	chatwork := &notify.Chatwork{ApiToken: "token", RoomId: "test", Logger: logger}

	now := time.Now()

	h := testkit.New(t,
		testkit.Providers(
			&testkit.KindProvider{},
			&testkit.KubectlProvider{},
		),
		testkit.RetainResourcesOnFailure(),
	)

	kc := h.KubernetesCluster(t)
	kctl := testkit.NewKubectl(kc.KubeconfigPath)

	kctl.Capture(t, "apply", "--wait=true", "-f", "https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml")

	// We intentionally make the test namespace deterministic to avoid ingress path
	// conflicts among test namespaces across test runs
	namespace := fmt.Sprintf("ingress-test-%d%02d%02d", now.Year(), now.Month(), now.Day())

	k8sclient, err := config.NewK8sClientset()
	if err != nil {
		t.Fatalf("NewK8sClientset: %s", err)
	}

	// kindとingress-nginxがある前提
	// レコードは作れないのでNoDnsCheckをtrueにする
	ingress := &Ingress{
		Checker:           cmd.NewChecker(context.Background(), true, logger, chatwork, "test", 1*time.Minute),
		Namespace:         namespace,
		Clientset:         k8sclient,
		NoDnsCheck:        true,
		IngressClassName:  "nginx",
		ResourceName:      "sample",
		ExternalHostname:  "sample.example.com",
		HTTPCheckEndpoint: "http://localhost/",
	}

	err = ingress.Check()
	if err != nil {
		t.Fatalf("Expected No Error, but got error: %s", err)
	}
}

type ProcessHandle struct {
	proc *os.Process
}

// Sends a SIGTERM to the process
func (h *ProcessHandle) Stop(t *testing.T) {
	t.Helper()

	if err := h.proc.Signal(syscall.SIGTERM); err != nil {
		t.Errorf("Failed to send SIGTERM to the process: %s", err)
	}

	if _, err := h.proc.Wait(); err != nil {
		t.Errorf("Failed to wait for the process to exit: %s", err)
	}
}

func StartProcess(t *testing.T, name string) *ProcessHandle {
	t.Helper()

	handle := &ProcessHandle{}

	proc, err := os.StartProcess(name, []string{}, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})

	if err != nil {
		t.Fatalf("Failed to start process: %s", err)
	}

	handle.proc = proc

	return handle
}

func TestCheckDNSRecord(t *testing.T) {
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}

	k8sclient, err := config.NewK8sClientset()
	if err != nil {
		t.Fatalf("NewK8sClientset: %s", err)
	}

	chatwork := &notify.Chatwork{ApiToken: "token", RoomId: "test", Logger: logger}

	// kindとingress-nginxがある前提
	// レコードは作れないのでNoDnsCheckをtrueにする
	ingress := &Ingress{
		Checker:          cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 1*time.Minute),
		Namespace:        "ingress-test",
		Clientset:        k8sclient,
		NoDnsCheck:       true,
		IngressClassName: "nginx",
		ResourceName:     "sample",
		ExternalHostname: "go.chatwork.com",
	}

	err = ingress.checkDNSRecord()
	if err != nil {
		t.Fatalf("Expected No Error, but got error: %s", err)
	}
}
