package clusterautoscaler

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/stretchr/testify/require"

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

	h := testkit.New(t,
		testkit.Providers(
			&testkit.TerraformProvider{
				WorkspacePath: "testdata/terraform",
				Vars: map[string]string{
					"prefix": "kibertas-ca",
					"region": "ap-northeast-1",
					"vpc_id": vpcID,
				},
			},
			&testkit.KubectlProvider{},
		),
		testkit.RetainResourcesOnFailure(),
	)

	kc := h.KubernetesCluster(t)

	k := testkit.NewKubernetes(kc.KubeconfigPath)
	testkit.PollUntil(t, func() bool {
		return len(k.ListReadyNodeNames(t)) == 1
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
				"scale-down-delay-after-add": "1m",
			},
		}

		hc.Namespace = clusterautoscalerNs
	})

	os.Setenv("KUBECONFIG", kc.KubeconfigPath)
	os.Setenv("NODE_LABEL_VALUE", "ON_DEMAND")

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

	require.NoError(t, clusterautoscaler.Check())
}
