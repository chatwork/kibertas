//go:build ekstest

package clusterautoscaler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"text/template"
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

	vpcID := os.Getenv("VPC_ID")
	if vpcID == "" {
		t.Skip("VPC_ID is not set")
	}

	amdAMIID := os.Getenv("AMD_AMI_ID")
	if amdAMIID == "" {
		t.Skip("AMD_AMI_ID is not set")
	}

	appName := "sample-for-scale"

	h := testkit.New(t,
		testkit.Providers(
			&testkit.TerraformProvider{
				WorkspacePath: "testdata/terraform",
				Vars: map[string]string{
					"autoscaler_name":               "karpenter",
					"prefix":                        "kibertas-ca",
					"region":                        "ap-northeast-1",
					"vpc_id":                        vpcID,
					"capacity_type":                 "SPOT",
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

	clusterautoscalerNs := "default"
	helm.UpgradeOrInstall(t, "karpenter", "oci://public.ecr.aws/karpenter/karpenter", func(hc *testkit.HelmConfig) {
		hc.Values = map[string]interface{}{
			"settings": map[string]interface{}{
				// This is so because we specify prefix=kibertas-ca in the testkit constructor above
				// and the terraform main.tf uses prefix + "-cluster" as the cluster name.
				"clusterName":       "kibertas-ca-cluster",
				"interruptionQueue": "kibertas-ca-cluster",
			},
			"controller": map[string]interface{}{
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
			// Otherwise, karpenter can be deployed onto the spot nodes, and it will prevent the scale-down
			"tolerations": []map[string]interface{}{
				{
					"key":    "node-role.kubernetes.io/control-plane",
					"effect": "NoSchedule",
				},
			},
		}

		hc.Namespace = clusterautoscalerNs
	})

	kubectl := testkit.NewKubectl(kc.KubeconfigPath)

	tmpKarpenterYamlPath := filepath.Join(t.TempDir(), "karpenter.yaml")
	tmpl, err := template.New("karpenter.yaml").ParseFiles("testdata/karpenter.yaml")
	require.NoError(t, err)
	{
		f, err := os.Create(tmpKarpenterYamlPath)
		require.NoError(t, err)
		defer f.Close()

		require.NoError(t, tmpl.ExecuteTemplate(f, "karpenter.yaml", map[string]string{
			"ClusterName": "kibertas-ca-cluster",
			"AmdAmiId":    amdAMIID,
			"RoleName":    "kibertas-ca-node",
		}))
	}

	t.Cleanup(func() {
		if !t.Failed() {
			kubectl.Capture(t, "delete", "-f", tmpKarpenterYamlPath)
		}
	})
	kubectl.Capture(t, "apply", "-f", tmpKarpenterYamlPath)

	os.Setenv("RESOURCE_NAME", appName)
	os.Setenv("KUBECONFIG", kc.KubeconfigPath)
	// os.Setenv("NODE_LABEL_VALUE", "ON_DEMAND")
	os.Setenv("NODE_LABEL_KEY", "karpenter.sh/capacity-type")
	os.Setenv("NODE_LABEL_VALUE", "spot")

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

	// Scale up by 1
	require.NoError(t, clusterautoscaler.Check())
	require.NoError(t, wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		nodes := k.ListReadyNodeNames(t)
		return len(nodes) == initialNodes+1, nil
	}))

	// Scale to zero (or the original number of nodes)
	require.NoError(t, wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 10*time.Minute, false, func(ctx context.Context) (bool, error) {
		nodes := k.ListReadyNodeNames(t)
		return len(nodes) == initialNodes, nil
	}))

	// Scale up by 1 (again)
	require.NoError(t, clusterautoscaler.Check())
	require.NoError(t, wait.PollUntilContextTimeout(context.Background(), 5*time.Second, 8*time.Minute, false, func(ctx context.Context) (bool, error) {
		nodes := k.ListReadyNodeNames(t)
		return len(nodes) == initialNodes+1, nil
	}))
}
