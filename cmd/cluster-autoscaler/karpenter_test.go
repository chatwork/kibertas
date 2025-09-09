package clusterautoscaler

import (
	"context"
	"fmt"
	apiv1 "k8s.io/api/core/v1"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/mumoshu/testkit"
	"github.com/sirupsen/logrus"
)

func TestKarpenterScaleUpFromNonZero(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	appName := "sample-for-scale"
	nodeLabelKey := "kwok-nodegroup"
	nodeLabelValue := "kwok-worker"

	h := testkit.New(t,
		testkit.Providers(
			&testkit.KindProvider{
				Image: os.Getenv("KIND_IMAGE"),
			},
			&testkit.KubectlProvider{},
		),
		testkit.RetainResourcesOnFailure(),
	)

	kc := h.KubernetesCluster(t)

	kctl := testkit.NewKubectl(kc.KubeconfigPath)
	k := testkit.NewKubernetes(kc.KubeconfigPath)

	const controlPlaneNodes = 1
	testkit.PollUntil(t, func() bool {
		return len(k.ListReadyNodeNames(t)) == controlPlaneNodes
	}, 5*time.Minute)
	t.Logf("Kind cluster is ready with %d control-plane nodes", controlPlaneNodes)

	clusterName := kctl.Capture(t, "config", "current-context")
	clusterName = strings.TrimPrefix(clusterName, "kind-")

	helm := testkit.NewHelm(kc.KubeconfigPath)

	helmInstallKarpenter(t, clusterName, helm, kctl)

	helmInstallKwok(t, helm)

	os.Setenv("RESOURCE_NAME", appName)
	os.Setenv("KUBECONFIG", kc.KubeconfigPath)
	os.Setenv("NODE_LABEL_KEY", nodeLabelKey)
	os.Setenv("NODE_LABEL_VALUE", nodeLabelValue)

	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{Logger: logger}
	checker := cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 7*time.Minute)
	clusterautoscaler, err := NewClusterAutoscaler(checker)
	require.NoError(t, err)
	require.NotNil(t, clusterautoscaler)

	// Allow pods to schedule on kwok-simulated nodes with kwok-provider taint
	clusterautoscaler.DeploymentOption.Tolerations = []apiv1.Toleration{
		{
			Key:      "kwok-provider",
			Operator: apiv1.TolerationOpEqual,
			Value:    "true",
			Effect:   apiv1.TaintEffectNoSchedule,
		},
	}

	// Scale up by 1 data-plane node
	require.NoError(t, clusterautoscaler.Check())
	require.NoError(t, wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		nodes := k.ListReadyNodeNames(t)
		return len(nodes) == controlPlaneNodes+1, nil
	}))

	// Scale to zero (or the original number of nodes)
	require.NoError(t, wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 10*time.Minute, false, func(ctx context.Context) (bool, error) {
		nodes := k.ListReadyNodeNames(t)
		return len(nodes) == controlPlaneNodes, nil
	}))
}

func helmInstallKarpenter(t *testing.T, clusterName string, helm *testkit.Helm, kctl *testkit.Kubectl) {
	if _, err := exec.LookPath("ko"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		t.Cleanup(cancel)
		cmd := exec.CommandContext(ctx, "ko", "build", "-B", "sigs.k8s.io/karpenter/kwok")
		cmd.Dir = filepath.Join("..", "..", "submodules", "karpenter")
		cmd.Env = append(os.Environ(),
			"KO_DOCKER_REPO=kind.local",
			fmt.Sprintf("KIND_CLUSTER_NAME=%s", clusterName),
		)
		t.Logf("Running: (cd %s && %s)", cmd.Dir, "ko build -B sigs.k8s.io/karpenter/kwok")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("ko build failed: %v\n%s", err, string(out))
		} else {
			t.Logf("ko build succeeded:\n%s", string(out))
		}
	} else {
		t.Fatalf("ko not found in PATH.")
	}

	clusterautoscalerNs := "default"
	helm.UpgradeOrInstall(t, "karpenter", "../../submodules/karpenter/kwok/charts", func(hc *testkit.HelmConfig) {
		hc.Values = map[string]interface{}{
			"controller": map[string]interface{}{
				"image": map[string]interface{}{
					"repository": "kind.local/kwok",
					"tag":        "latest",
				},
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu":    "1",
						"memory": "1Gi",
					},
					"limits": map[string]interface{}{
						"cpu":    "1",
						"memory": "1Gi",
					},
				},
			},
			"tolerations": []map[string]interface{}{
				{
					"key":    "node-role.kubernetes.io/control-plane",
					"effect": "NoSchedule",
				},
			},
		}
		hc.Namespace = clusterautoscalerNs
	})

	kctl.Capture(t, "apply", "-f", "testdata/karpenter.yaml")
	// Apply Karpenter NodePool/NodeClass for KWOK (no templating required)
	t.Cleanup(func() {
		if !t.Failed() {
			kctl.Capture(t, "delete", "-f", "testdata/karpenter.yaml")
		}
	})

	testkit.PollUntil(t, func() bool {
		output := kctl.Capture(t, "get", "deployment", "karpenter", "-n", clusterautoscalerNs, "-o", "jsonpath={.status.readyReplicas}")
		return output == "1"
	}, 1*time.Minute)

	t.Logf("Karpenter NodePool and NodeClass applied successfully")
}
