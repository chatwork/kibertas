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

	os.Setenv("RESOURCE_NAME", appName)
	os.Setenv("KUBECONFIG", kc.KubeconfigPath)
	os.Setenv("NODE_LABEL_KEY", nodeLabelKey)
	os.Setenv("NODE_LABEL_VALUE", nodeLabelValue)

	kctl := testkit.NewKubectl(kc.KubeconfigPath)
	k := testkit.NewKubernetes(kc.KubeconfigPath)

	const controlPlaneNodes = 1
	testkit.PollUntil(t, func() bool {
		return len(k.ListReadyNodeNames(t)) == controlPlaneNodes
	}, 5*time.Minute)
	t.Logf("Kind cluster is ready with %d control-plane nodes", controlPlaneNodes)

	clusterName := kctl.Capture(t, "config", "current-context")
	clusterName = strings.TrimPrefix(strings.TrimSpace(clusterName), "kind-")
	// ko が利用する Kind のクラスタ名が妥当か検証し、必要なら自動補正
	clusterName = resolveKindClusterName(t, clusterName)

	helm := testkit.NewHelm(kc.KubeconfigPath)

	helmInstallKwok(t, helm)
	helmInstallKarpenter(t, clusterName, helm, kctl)

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
	require.NoError(t, wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
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
			// provider 明示（環境差による不一致を避ける）
			"KIND_EXPERIMENTAL_PROVIDER=docker",
		)
		t.Logf("Using KIND_CLUSTER_NAME=%s", clusterName)
		t.Logf("Running: (cd %s && %s)", cmd.Dir, "ko build -B sigs.k8s.io/karpenter/kwok")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("ko build failed: %v\n%s", err, string(out))
		} else {
			t.Logf("ko build succeeded:\n%s", string(out))
		}
	} else {
		t.Skipf("ko not found in PATH; skipping Karpenter test: %v", err)
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

// resolveKindClusterName は、与えられたクラスタ名で kind ノードが見つからない場合に
// `kind get clusters` の結果から自動補正する。見つからない場合は詳細ログを出して fail。
func resolveKindClusterName(t *testing.T, name string) string {
	// まず与えられた名前で確認
	if hasKindNodes(t, name) {
		return name
	}
	// 一覧から補正
	out, err := exec.Command("kind", "get", "clusters").CombinedOutput()
	if err != nil {
		t.Fatalf("failed to list kind clusters: %v\n%s", err, string(out))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	t.Logf("kind clusters detected: %v", lines)
	if len(lines) == 1 && lines[0] != "" {
		candidate := strings.TrimSpace(lines[0])
		if hasKindNodes(t, candidate) {
			t.Logf("Auto-selected KIND_CLUSTER_NAME=%s (was %s)", candidate, name)
			return candidate
		}
	}
	t.Fatalf("kind nodes not found for cluster %q; candidates=%v", name, lines)
	return name
}

func hasKindNodes(t *testing.T, name string) bool {
	if _, err := exec.LookPath("kind"); err != nil {
		t.Fatalf("kind CLI not found in PATH: %v", err)
	}
	out, err := exec.Command("kind", "get", "nodes", "--name", name).CombinedOutput()
	if err != nil {
		t.Logf("kind get nodes failed for %s: %v\n%s", name, err, string(out))
		return false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return len(lines) > 0 && strings.TrimSpace(lines[0]) != ""
}
