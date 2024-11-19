//go:build ekstest

package clusterautoscaler

import (
	"context"
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

	vpcID := os.Getenv("VPC_ID")
	if vpcID == "" {
		t.Skip("VPC_ID is not set")
	}

	appName := "sample-for-scale"
	capacityType := "SPOT"
	// capacityType := "ON_DEMAND"

	h := testkit.New(t,
		testkit.Providers(
			&testkit.TerraformProvider{
				WorkspacePath: "testdata/terraform",
				Vars: map[string]string{
					"prefix":                        "kibertas-ca",
					"region":                        "ap-northeast-1",
					"vpc_id":                        vpcID,
					"capacity_type":                 capacityType,
					"node_template_app_label_value": appName,
				},
			},
			&testkit.KubectlProvider{},
		),
		testkit.RetainResourcesOnFailure(),
	)

	kc := h.KubernetesCluster(t)

	k := testkit.NewKubernetes(kc.KubeconfigPath)
	testkit.PollUntil(t, func() bool {
		return len(k.ListReadyNodeNames(t)) > 1
	}, 20*time.Second)

	helm := testkit.NewHelm(kc.KubeconfigPath)
	// See https://github.com/kubernetes/autoscaler/tree/master/charts/cluster-autoscaler#tldr
	helm.AddRepo(t, "autoscaler", "https://kubernetes.github.io/autoscaler")

	clusterautoscalerNs := "default"
	helm.UpgradeOrInstall(t, "cluster-autoscaler", "autoscaler/cluster-autoscaler", func(hc *testkit.HelmConfig) {
		hc.Values = map[string]interface{}{
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

	os.Setenv("RESOURCE_NAME", appName)
	os.Setenv("KUBECONFIG", kc.KubeconfigPath)
	// os.Setenv("NODE_LABEL_VALUE", "ON_DEMAND")
	os.Setenv("NODE_LABEL_VALUE", capacityType)

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

	if clusterautoscaler == nil {
		t.Error("Expected clusterautoscaler instance, got nil")
	}

	initialNodes := len(k.ListReadyNodeNames(t))

	// Scale from 1 to 2
	require.NoError(t, clusterautoscaler.Check())
	require.NoError(t, wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		nodes := k.ListReadyNodeNames(t)
		return len(nodes) == initialNodes+1, nil
	}))

	// Scale to 0
	require.NoError(t, wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 10*time.Minute, false, func(ctx context.Context) (bool, error) {
		nodes := k.ListReadyNodeNames(t)
		return len(nodes) == 2, nil
	}))

	// Scale from 0 to 1
	require.NoError(t, clusterautoscaler.Check())
	require.NoError(t, wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 8*time.Minute, false, func(ctx context.Context) (bool, error) {
		nodes := k.ListReadyNodeNames(t)
		return len(nodes) == 3, nil
	}))
}
