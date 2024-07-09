package ingress

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/mumoshu/testkit"
	"github.com/stretchr/testify/require"

	"github.com/sirupsen/logrus"
)

func TestIngressCheckE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

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
	time.Sleep(180 * time.Second)

	// Start cloud-provider-kind to manage service type=LoadBalancer
	//
	// This requiers cloud-provider-kind to be installed in the PATH.
	// Follow https://github.com/kubernetes-sigs/cloud-provider-kind?tab=readme-ov-file#install to install it.
	bin, err := exec.LookPath("cloud-provider-kind")
	if bin == "" {
		t.Fatalf("cloud-provider-kind not found in PATH: %s", os.Getenv("PATH"))
	}
	require.NoError(t, err)

	handle := SudoStartProcess(t, bin, kc.KubeconfigPath)
	defer handle.Stop(t)

	helm := testkit.NewHelm(kc.KubeconfigPath)
	// See https://github.com/kubernetes/autoscaler/tree/master/charts/cluster-autoscaler#tldr
	helm.AddRepo(t, "ingress-nginx", "https://kubernetes.github.io/ingress-nginx")

	ingressNginxNs := "default"
	helm.UpgradeOrInstall(t, "my-ingress-nginx", "ingress-nginx/ingress-nginx", func(hc *testkit.HelmConfig) {
		hc.Values = map[string]interface{}{
			"rbac": map[string]interface{}{
				"create": true,
			},
		}

		hc.Namespace = ingressNginxNs
	})

	kctl := testkit.NewKubectl(kc.KubeconfigPath)

	// Get the external IP of the ingress-nginx service
	ingressNginxSvcLBIP := kctl.Capture(t, "get", "svc", "-n", ingressNginxNs, "my-ingress-nginx-controller", "-o", "jsonpath={.status.loadBalancer.ingress[0].ip}")
	t.Logf("ingress-nginx service LB IP: %s", ingressNginxSvcLBIP)

	// We intentionally make the test namespace deterministic to avoid ingress path
	// conflicts among test namespaces across test runs
	namespace := fmt.Sprintf("ingress-test-%d%02d%02d", now.Year(), now.Month(), now.Day())

	os.Setenv("KUBECONFIG", kc.KubeconfigPath)
	os.Setenv("RESOURCE_NAME", "sample")
	os.Setenv("EXTERNAL_HOSTNAME", "sample.example.com")
	os.Setenv("INGRESS_CLASS_NAME", "nginx")
	ingress, err := NewIngress(
		cmd.NewChecker(context.Background(), true, logger, chatwork, "test", 1*time.Minute),
		true,
	)
	require.NoError(t, err)
	ingress.Namespace = namespace
	ingress.HTTPCheckEndpoint = "http://" + ingressNginxSvcLBIP + "/"

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

func SudoStartProcess(t *testing.T, name, kubeconfig string) *ProcessHandle {
	t.Helper()

	handle := &ProcessHandle{}

	env := os.Environ()
	env = append(env, "KUECONFIG="+kubeconfig)
	sudoPath, err := exec.LookPath("sudo")
	if sudoPath == "" {
		t.Fatalf("sudo not found in PATH: %s", os.Getenv("PATH"))
	}
	require.NoError(t, err)
	proc, err := os.StartProcess(sudoPath, []string{sudoPath, name}, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Env:   env,
	})

	if err != nil {
		t.Fatalf("Failed to start process: %s", err)
	}

	handle.proc = proc

	return handle
}

func TestIngressCheckDNSRecord(t *testing.T) {
	if os.Getenv("INGRESS_CHECK_DNS_RECORD") == "" {
		t.Skip("Skipping test as INGRESS_CHECK_DNS_RECORD is not set")
	}

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
