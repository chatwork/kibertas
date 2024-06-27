package datadogagent

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/mumoshu/testkit"
	"github.com/sirupsen/logrus"
)

func TestDatadogAgentNew(t *testing.T) {
	t.Parallel()
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{}
	checker := cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 3*time.Minute)
	datadogAgent, err := NewDatadogAgent(checker)
	if err != nil {
		t.Fatalf("NewDatadogAgent: %s", err)
	}

	if datadogAgent == nil {
		t.Error("Expected datadogAgent instance, got nil")
	}
}

func TestDatadogAgentCheck(t *testing.T) {
	t.Parallel()

	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}

	chatwork := &notify.Chatwork{ApiToken: "token", RoomId: "test", Logger: logger}
	datadogAgent := &DatadogAgent{
		Checker:      cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 1*time.Minute),
		ApiKey:       "",
		AppKey:       "",
		QueryMetrics: "",
		WaitTime:     1 * time.Second,
	}

	err := datadogAgent.Check()
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	expectedError := errors.New("DD_API_KEY or DD_APP_KEY is empty")
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected '%s', got '%s'", expectedError.Error(), err.Error())
	}

	datadogAgent = &DatadogAgent{
		Checker:      cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 3*time.Minute),
		ApiKey:       "test",
		AppKey:       "test",
		QueryMetrics: "avg:kubernetes.cpu.user.total{*}",
		WaitTime:     1 * time.Second,
	}

	err = datadogAgent.Check()
	expectedError = errors.New("error waiting for query metrics results: 403 Forbidden")
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected '%s', got '%s'", expectedError.Error(), err.Error())
	}
}

func TestDatadogAgentCheckE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}

	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}

	datadogAPIKey := os.Getenv("DD_API_KEY")
	if datadogAPIKey == "" {
		t.Skip("DD_API_KEY is not set")
	}
	datadogAPPKey := os.Getenv("DD_APP_KEY")
	if datadogAPPKey == "" {
		t.Skip("DD_APP_KEY is not set")
	}

	h := testkit.New(t,
		testkit.Providers(
			&testkit.KindProvider{},
			&testkit.KubectlProvider{},
		),
		testkit.RetainResourcesOnFailure(),
	)

	kc := h.KubernetesCluster(t)

	helm := testkit.NewHelm(kc.KubeconfigPath)
	// See https://github.com/kubernetes/autoscaler/tree/master/charts/cluster-autoscaler#tldr
	helm.AddRepo(t, "datadog", "https://helm.datadoghq.com")

	datadogAgentNs := "default"
	helm.UpgradeOrInstall(t, "my-datadog-operator", "datadog/datadog-operator", func(hc *testkit.HelmConfig) {
		hc.Values = map[string]interface{}{}

		hc.Namespace = datadogAgentNs
	})

	kubectl := testkit.NewKubectl(kc.KubeconfigPath)
	t.Cleanup(func() {
		kubectl.Capture(t, "delete", "secret", "datadog-secret")
	})
	kubectl.Capture(t, "create", "secret", "generic", "datadog-secret",
		"--from-literal", "api-key="+datadogAPIKey, "--from-literal", "app-key="+datadogAPPKey)

	t.Cleanup(func() {
		if !t.Failed() {
			kubectl.Capture(t, "delete", "-f", "testdata/datadog-agent.yaml")
		}
	})
	kubectl.Capture(t, "apply", "-f", "testdata/datadog-agent.yaml")

	chatwork := &notify.Chatwork{Logger: logger}
	datadogAgent, err := NewDatadogAgent(cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 3*time.Minute))
	if err != nil {
		t.Fatalf("NewDatadogAgent: %s", err)
	}
	datadogAgent.WaitTime = 1 * time.Minute
	datadogAgent.QueryMetrics = "avg:kubernetes.cpu.user.total{*}"

	err = datadogAgent.Check()
	if err != nil {
		t.Fatalf("Unexpected error on Check: %s", err)
	}
}
