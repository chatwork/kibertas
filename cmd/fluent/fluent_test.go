//go:build ekstest

package fluent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/stretchr/testify/require"

	"github.com/mumoshu/testkit"
	"github.com/sirupsen/logrus"
)

func TestFluentE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	prefix := os.Getenv("PREFIX")
	if prefix == "" {
		t.Skip("PREFIX is not set")
	}

	vpcID := os.Getenv("VPC_ID")
	if vpcID == "" {
		t.Skip("VPC_ID is not set")
	}

	terraformStateBucket := os.Getenv("TERRAFORM_STATE_BUCKET")
	if terraformStateBucket == "" {
		t.Skip("TERRAFORM_STATE_BUCKET is not set")
	}

	terraformStateKey := os.Getenv("TERRAFORM_STATE_KEY")
	if terraformStateKey == "" {
		t.Skip("TERRAFORM_STATE_KEY is not set")
	}

	eksAccessPrincipalArn := os.Getenv("EKS_ACCESS_PRINCIPAL_ARN")
	if eksAccessPrincipalArn == "" {
		t.Skip("EKS_ACCESS_PRINCIPAL_ARN is not set")
	}

	h := testkit.New(t,
		testkit.Providers(
			&testkit.TerraformProvider{
				WorkspacePath: "testdata/terraform",
				Vars: map[string]string{
					"prefix":                   prefix,
					"region":                   "ap-northeast-1",
					"vpc_id":                   vpcID,
					"eks_access_principal_arn": eksAccessPrincipalArn,
				},
				BackendConfig: map[string]string{
					"bucket": terraformStateBucket,
					"key":    terraformStateKey,
					"region": "ap-northeast-1",
				},
			},
			&testkit.KubectlProvider{},
		),
		testkit.RetainResourcesOnFailure(),
	)

	kc := h.KubernetesCluster(t)
	s3Bucket := h.S3Bucket(t)
	ns := h.KubernetesNamespace(t, testkit.KubeconfigPath(kc.KubeconfigPath))
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("KUBECONFIG=%s", kc.KubeconfigPath)
		}
	})

	k := testkit.NewKubernetes(kc.KubeconfigPath)
	testkit.PollUntil(t, func() bool {
		return len(k.ListReadyNodeNames(t)) == 1
	}, 20*time.Second)

	helm := testkit.NewHelm(kc.KubeconfigPath)
	helm.AddRepo(t, "chatwork", "https://chatwork.github.io/charts")

	// We need to create a pod to alter the /var/log/fluentd-s3 directory
	// because the fluentd pod cannot create the directory.
	// Note that the fluentd pod uses:
	//   uid=999(fluent) gid=999(fluent) groups=999(fluent)
	kctl := testkit.NewKubectl(kc.KubeconfigPath)
	podYamlFile, err := filepath.Abs(filepath.Join("testdata", "fluentd-alter-log-dir.pod.yaml"))
	require.NoError(t, err)
	require.FileExists(t, podYamlFile)
	kctl.Capture(t,
		"create", "-f", podYamlFile,
	)
	t.Cleanup(func() {
		kctl.Capture(t, "delete", "-f", podYamlFile)
	})

	testkit.PollUntil(t, func() bool {
		return strings.Contains(kctl.Capture(t, "get", "pod", "fluentd-alter-log-dir"), "Completed")
	}, 30*time.Second)

	fluentdNs := "default"
	logsPath := "logs"
	helm.UpgradeOrInstall(t, "fluentd", "chatwork/fluentd", func(hc *testkit.HelmConfig) {
		hc.Values = map[string]interface{}{
			"dasemonset": map[string]interface{}{
				"enabled": true,
			},
			"serviceAccount": map[string]interface{}{
				"create": true,
			},
			"rbac": map[string]interface{}{
				"create": true,
			},
			"configmaps": map[string]interface{}{
				"daemonset.conf": `<source>
  @type tail
  path /var/log/containers/*.log
  pos_file /var/log/fluentd/containers.log.pos
  tag kube.*
  exclude_path ["/var/log/containers/fluent*"]
  read_from_head true
  # See https://github.com/fluent/fluentd-kubernetes-daemonset/issues/412#issuecomment-678353684
  <parse>
    @type regexp
	expression /^(?<time>.+) (?<stream>stdout|stderr) (?<logtag>[FP]) (?<log>.+)$/
	time_format %Y-%m-%dT%H:%M:%S.%N%:z
  </parse>
</source>
<filter kube.**>
  @type kubernetes_metadata
</filter>` + fmt.Sprintf(`
<match kube.**>
  @type s3
  s3_bucket %s
  s3_region ap-northeast-1
  path %s/
  flush_interval 10s
  <buffer>
    @type file
    path /var/log/fluentd-s3
    timekey 60 # 1 min
    timekey_wait 30s
    chunk_limit_size 256m
  </buffer>
</match>
`, s3Bucket.Name, logsPath),
			},
		}

		hc.Namespace = fluentdNs
	})

	fluentdClusterRoleBindingName := "fluentd-cluster-admin-binding"

	defer func() {
		if h.CleanupNeeded(t.Failed()) {
			kctl.Capture(t, "delete", "clusterrolebinding", fluentdClusterRoleBindingName)
		}
	}()

	if kctl.Failed(t, "get", "clusterrolebinding", fluentdClusterRoleBindingName) {
		kctl.Capture(t, "create", "clusterrolebinding", fluentdClusterRoleBindingName, "--clusterrole=cluster-admin", "--serviceaccount="+fluentdNs+":fluentd")
	}

	os.Setenv("KUBECONFIG", kc.KubeconfigPath)
	os.Setenv("LOG_BUCKET_NAME", s3Bucket.Name)
	os.Setenv("RESOURCE_NAMESPACE", ns.Name)
	os.Setenv("LOG_PATH", logsPath)

	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{
		Logger: logger,
	}
	checker := cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 3*time.Minute)
	fluent, err := NewFluent(checker)
	if err != nil {
		t.Fatalf("NewFluent: %s", err)
	}

	if fluent == nil {
		t.Error("Expected fluent instance, got nil")
	}

	require.NoError(t, fluent.Check())
}
