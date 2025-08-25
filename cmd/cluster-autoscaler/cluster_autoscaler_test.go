//go:build !ekstest

package clusterautoscaler

import (
	"context"
	apiv1 "k8s.io/api/core/v1"
	"os"
	"testing"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/mumoshu/testkit"
	"github.com/sirupsen/logrus"
)

func TestClusterAutoscalerScaleUpFromNonZero(t *testing.T) {
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
	testkit.PollUntil(t, func() bool {
		// Only the control-plane node should be counted as Ready
		return len(k.ListReadyNodeNames(t)) == 1
	}, 20*time.Second)

	helm := testkit.NewHelm(kc.KubeconfigPath)
	// See https://github.com/kubernetes/autoscaler/tree/master/charts/cluster-autoscaler#tldr

	helmInstallKwok(t, helm)
	helmInstallClusterAutoscaler(t, helm, kctl)

	os.Setenv("RESOURCE_NAME", appName)
	os.Setenv("KUBECONFIG", kc.KubeconfigPath)
	os.Setenv("NODE_LABEL_KEY", nodeLabelKey)
	os.Setenv("NODE_LABEL_VALUE", nodeLabelValue)

	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{
		Logger: logger,
	}
	checker := cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 7*time.Minute)
	clusterautoscaler, err := NewClusterAutoscaler(checker)
	if err != nil {
		t.Fatalf("NewClusterAutoscaler: %s", err)
	}

	// Configure tolerations for kwok provider nodes
	// This allows test pods to be scheduled on kwok-simulated nodes with taints
	clusterautoscaler.SetDeploymentOption(DeploymentOption{
		Tolerations: []apiv1.Toleration{
			{
				Key:      "kwok-provider",
				Operator: apiv1.TolerationOpEqual,
				Value:    "true",
				Effect:   apiv1.TaintEffectNoSchedule,
			},
		},
	})

	if clusterautoscaler == nil {
		t.Error("Expected clusterautoscaler instance, got nil")
	}

	require.NoError(t, wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		nodes := k.ListReadyNodeNames(t)
		return len(nodes) == 1, nil
	}))

	// Scale from 1 to 2
	require.NoError(t, clusterautoscaler.Check())
	require.NoError(t, wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		nodes := k.ListReadyNodeNames(t)
		return len(nodes) == 2, nil
	}))
}

func helmInstallClusterAutoscaler(t *testing.T, helm *testkit.Helm, kctl *testkit.Kubectl) {
	helm.AddRepo(t, "autoscaler", "https://kubernetes.github.io/autoscaler")

	clusterautoscalerNs := "default"
	helm.UpgradeOrInstall(t, "cluster-autoscaler", "autoscaler/cluster-autoscaler", func(hc *testkit.HelmConfig) {
		hc.Values = map[string]interface{}{
			"cloudProvider": "kwok",
			"autoDiscovery": map[string]interface{}{
				// This is so because we specify prefix=kibertas-ca in the testkit constructor above
				// and the terraform main.tf uses prefix + "-cluster" as the cluster name.
				"clusterName": "kibertas-ca-cluster",
			},
			"awsRegion": "ap-northeast-1",
			"extraArgs": map[string]interface{}{
				"scale-down-delay-after-add":     "10s",
				"scale-down-delay-after-failure": "20s",
				// Let the scale-down happen quickly for the test
				"scale-down-unneeded-time": "30s",
				// Make system pods deployed onto the spot nodes not prevent the scale-down
				"skip-nodes-with-system-pods": "false",
			},
			// Otherwise, cluster-autoscaler can be deployed onto the spot nodes, and it will prevent the scale-down
			"tolerations": []map[string]interface{}{
				{
					"key":    "node-role.kubernetes.io/control-plane",
					"effect": "NoSchedule",
				},
			},
		}

		hc.Namespace = clusterautoscalerNs
	})

	// Apply custom node templates for kwok provider to define node specifications
	kctl.Capture(t,
		"apply", "-n", clusterautoscalerNs, "-f", "testdata/kwok-provider-templates.yaml",
	)
	// Restart cluster-autoscaler to load the new node templates
	kctl.Capture(t,
		"rollout", "restart", "deployment", "cluster-autoscaler-kwok-cluster-autoscaler",
		"-n", clusterautoscalerNs,
	)
}

func helmInstallKwok(t *testing.T, helm *testkit.Helm) {
	helm.AddRepo(t, "kwok", "https://kwok.sigs.k8s.io/charts")

	helm.UpgradeOrInstall(t, "kwok", "kwok/kwok", func(hc *testkit.HelmConfig) {
		hc.Namespace = "kube-system"
	})
	// why: install stage rules to simulate Pod/Node lifecycle in a KWOK simulated cluster
	helm.UpgradeOrInstall(t, "kwok-stage-fast", "kwok/stage-fast", func(hc *testkit.HelmConfig) {
		hc.Namespace = "kube-system"
	})
}
