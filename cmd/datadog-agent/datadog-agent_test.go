package datadogagent

import (
	"errors"
	"os"
	"testing"

	"github.com/cw-sakamoto/kibertas/cmd"
	"github.com/cw-sakamoto/kibertas/config"
	"github.com/cw-sakamoto/kibertas/util/notify"
	"github.com/sirupsen/logrus"
)

func TestNewDatadogAgent(t *testing.T) {
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{}
	datadogAgent, err := NewDatadogAgent(true, logger, chatwork)
	if err != nil {
		t.Fatalf("NewDatadogAgent: %s", err)
	}

	if datadogAgent == nil {
		t.Error("Expected datadogAgent instance, got nil")
	}
}

func TestCheck(t *testing.T) {
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}

	k8sclient, err := config.NewK8sClientset()
	if err != nil {
		t.Fatalf("NewK8sClientset: %s", err)
	}

	chatwork := &notify.Chatwork{ApiToken: "token", RoomId: "test", Logger: logger}
	datadogAgent := &DatadogAgent{
		Checker:     cmd.NewChecker("test", k8sclient, true, logger, chatwork),
		ApiKey:      "",
		AppKey:      "",
		ClusterName: "",
	}

	err = datadogAgent.Check()
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	expectedError := errors.New("DD_API_KEY or DD_APP_KEY is empty")
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected '%s', got '%s'", expectedError.Error(), err.Error())
	}

	os.Setenv("DD_API_KEY", "test")
	os.Setenv("DD_APP_KEY", "test")
	datadogAgent, err = NewDatadogAgent(true, logger, chatwork)
	if err != nil {
		t.Fatalf("NewDatadogAgent: %s", err)
	}
	err = datadogAgent.Check()
	expectedError = errors.New("CLUSTER_NAME is empty")
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected '%s', got '%s'", expectedError.Error(), err.Error())
	}

	os.Setenv("CLUSTER_NAME", "test")
	datadogAgent, err = NewDatadogAgent(true, logger, chatwork)
	if err != nil {
		t.Fatalf("NewDatadogAgent: %s", err)
	}

	err = datadogAgent.Check()
	expectedError = errors.New("403 Forbidden")
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected '%s', got '%s'", expectedError.Error(), err.Error())
	}
}
