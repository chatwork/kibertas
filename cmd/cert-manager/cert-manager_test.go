package certmanager

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	cmapiv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/chatwork/kibertas/util"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/mumoshu/testkit"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
)

func TestMain(m *testing.M) {
	h, err := testkit.Build(
		testkit.Providers(
			&testkit.KindProvider{},
			&testkit.KubectlProvider{},
		),
		testkit.RetainResourcesOnFailure(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build test harness: %s", err)
		os.Exit(1)
	}

	kcp, err := h.KubernetesClusterProvider()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get KubernetesClusterProvider: %s", err)
		os.Exit(1)
	}

	kc, err := kcp.GetKubernetesCluster()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get KubernetesCluster: %s", err)
		os.Exit(1)
	}

	os.Setenv("KUBECONFIG", kc.KubeconfigPath)

	code := m.Run()

	if h.CleanupNeeded(code != 0) {
		if err := h.DoCleanup(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to cleanup test harness: %s", err)
		}
	}

	os.Exit(code)
}

func TestCertManagerNew(t *testing.T) {
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{}
	checker := cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 3*time.Minute)
	ingress, err := NewCertManager(checker)
	if err != nil {
		t.Fatalf("NewCertManager: %s", err)
	}

	if ingress == nil {
		t.Error("Expected certManager instance, got nil")
	}
}

func TestCertManagerCheck(t *testing.T) {
	helm := testkit.NewHelm(os.Getenv("KUBECONFIG"))
	helm.AddRepo(t, "jetstack", "https://charts.jetstack.io")

	certManagerNs := "default"
	helm.UpgradeOrInstall(t, "cert-manager", "jetstack/cert-manager", func(hc *testkit.HelmConfig) {
		hc.Values = map[string]interface{}{
			"installCRDs": true,
			"prometheus": map[string]interface{}{
				"enabled": false,
			},
		}

		hc.Namespace = certManagerNs
	})

	kubectl := testkit.NewKubectl(os.Getenv("KUBECONFIG"))
	kubectl.Capture(t, "create", "-f", "testdata/clusterissuer.yaml")
	t.Cleanup(func() {
		if !t.Failed() {
			kubectl.Capture(t, "delete", "-f", "testdata/clusterissuer.yaml")
		}
	})

	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}

	k8sclientset, err := config.NewK8sClientset()
	if err != nil {
		t.Fatalf("NewK8sClientset: %s", err)
	}

	scheme := runtime.NewScheme()
	_ = cmapiv1.AddToScheme(scheme)

	k8sclient, err := config.NewK8sClient(client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("NewK8sClient: %s", err)
	}

	chatwork := &notify.Chatwork{ApiToken: "token", RoomId: "test", Logger: logger}

	now := time.Now()
	namespace := fmt.Sprintf("cert-manager-test-%d%02d%02d-%s", now.Year(), now.Month(), now.Day(), util.GenerateRandomString(5))
	cm := &CertManager{
		Checker:      cmd.NewChecker(context.Background(), true, logger, chatwork, "test", 3*time.Minute),
		Namespace:    namespace,
		ResourceName: "sample",
		Clientset:    k8sclientset,
		Client:       k8sclient,
	}

	err = cm.Check()
	if err != nil {
		t.Fatalf("Expected No Error, but got error: %s", err)
	}
}
