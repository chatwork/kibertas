package fluent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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

	s3Endpoint := "http://localstack.default.svc.cluster.local:4566"
	s3Region := "us-east-1"
	fluentdNs := "default"
	logsPath := "logs"
	s3Bucket := "kubernetes-logs"

	// Put KubectlProvider before KindProvider so namespaces/configmaps
	// are cleaned up before the Kind cluster is deleted.
	h := testkit.New(t,
		testkit.Providers(
			&testkit.KubectlProvider{},
			&testkit.KindProvider{
				Image: os.Getenv("KIND_IMAGE"),
			},
		),
		testkit.RetainResourcesOnFailure(),
	)

	kc := h.KubernetesCluster(t)
	ns := h.KubernetesNamespace(t, testkit.KubeconfigPath(kc.KubeconfigPath))
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("KUBECONFIG=%s", kc.KubeconfigPath)
		}
	})

	k := testkit.NewKubernetes(kc.KubeconfigPath)
	testkit.PollUntil(t, func() bool {
		return len(k.ListReadyNodeNames(t)) == 1
	}, 5*time.Minute)

	kctl := testkit.NewKubectl(kc.KubeconfigPath)

	helm := testkit.NewHelm(kc.KubeconfigPath)
	helm.AddRepo(t, "chatwork", "https://chatwork.github.io/charts")

	// We need to create a pod to alter the /var/log/fluentd-s3 directory
	// because the fluentd pod cannot create the directory.
	// Note that the fluentd pod uses:
	//   uid=999(fluent) gid=999(fluent) groups=999(fluent)
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
	}, 2*time.Minute)

	localhostS3Endpoint := deployLocalStack(t, kc.KubeconfigPath, kctl, h)
	prepareFluentdLogDestinationBucket(t, localhostS3Endpoint, s3Bucket)

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
  s3_endpoint %s
  s3_bucket %s
  s3_region %s
  path %s/
  flush_interval 10s
  force_path_style true
  use_ssl false
  aws_key_id test
  aws_sec_key test
  <buffer>
    @type file
    path /var/log/fluentd-s3
    timekey 60 # 1 min
    timekey_wait 30s
    chunk_limit_size 256m
  </buffer>
</match>
`, s3Endpoint, s3Bucket, s3Region, logsPath),
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
	os.Setenv("LOG_BUCKET_NAME", s3Bucket)
	os.Setenv("USE_PATH_STYLE", "true")
	os.Setenv("RESOURCE_NAMESPACE", ns.Name)
	os.Setenv("LOG_PATH", logsPath)

	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{
		Logger: logger,
	}

	// Set AWS environment variables for LocalStack
	// kibertas internally uses AWS SDK, so these environment variables
	// configure the SDK to connect to LocalStack instead of real AWS
	os.Setenv("AWS_ENDPOINT_URL", localhostS3Endpoint)
	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	os.Setenv("AWS_DEFAULT_REGION", s3Region)

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

func deployLocalStack(t *testing.T, kubeconfigPath string, kctl *testkit.Kubectl, h *testkit.TestKit) string {
	localstackYamlFile, err := filepath.Abs(filepath.Join("testdata", "localstack.yaml"))
	require.NoError(t, err)
	require.FileExists(t, localstackYamlFile)
	kctl.Capture(t, "apply", "-f", localstackYamlFile)
	t.Cleanup(func() {
		if h.CleanupNeeded(t.Failed()) {
			kctl.Capture(t, "delete", "-f", localstackYamlFile)
		}
	})

	testkit.PollUntil(t, func() bool {
		output := kctl.Capture(t, "get", "pod", "-l", "app=localstack", "-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
		return strings.Contains(output, "True")
	}, 3*time.Minute)

	localstackPort := "14566"
	localhostS3Endpoint := fmt.Sprintf("http://127.0.0.1:%s", localstackPort)

	// Start kubectl port-forward as a background process and manage its lifecycle
	command := exec.Command(
		"kubectl",
		"--kubeconfig", kubeconfigPath,
		"-n", "default",
		"port-forward",
		"service/localstack",
		localstackPort+":4566",
		"--address", "127.0.0.1",
	)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		t.Fatalf("failed to start port-forward: %v", err)
	}
	t.Cleanup(func() {
		// Ensure the port-forward process is terminated without failing the test
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	})

	testkit.PollUntil(t, func() bool {
		resp, err := http.Get(localhostS3Endpoint + "/_localstack/health")
		if err != nil {
			t.Logf("Port-forward health check failed: %v", err)
			return false
		}
		defer resp.Body.Close()
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		output := string(body[:n])
		return strings.Contains(output, "s3") && (strings.Contains(output, "running") || strings.Contains(output, "available"))
	}, 3*time.Minute)
	return localhostS3Endpoint
}

func prepareFluentdLogDestinationBucket(t *testing.T, s3Endpoint string, s3Bucket string) {
	req, err := http.NewRequest("PUT", s3Endpoint+"/"+s3Bucket, nil)
	require.NoError(t, err)
	client := &http.Client{}
	bucketResp, err := client.Do(req)
	if err == nil {
		bucketResp.Body.Close()
	}
}
