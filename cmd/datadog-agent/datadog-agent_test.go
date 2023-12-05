package datadogagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/sirupsen/logrus"
)

func TestNewDatadogAgent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}

	k8sclient, err := config.NewK8sClientset()
	if err != nil {
		t.Fatalf("NewK8sClientset: %s", err)
	}

	chatwork := &notify.Chatwork{ApiToken: "token", RoomId: "test", Logger: logger}
	datadogAgent := &DatadogAgent{
		Checker:      cmd.NewChecker("test", k8sclient, true, logger, chatwork, 1*time.Minute),
		ApiKey:       "",
		AppKey:       "",
		QueryMetrics: "",
		WaitTime:     1 * time.Second,
	}

	err = datadogAgent.Check(context.TODO())
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	expectedError := errors.New("DD_API_KEY or DD_APP_KEY is empty")
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected '%s', got '%s'", expectedError.Error(), err.Error())
	}

	datadogAgent = &DatadogAgent{
		Checker:      cmd.NewChecker("test", k8sclient, true, logger, chatwork, 3*time.Minute),
		ApiKey:       "test",
		AppKey:       "test",
		QueryMetrics: "avg:kubernetes.cpu.user.total",
		WaitTime:     1 * time.Second,
	}

	err = datadogAgent.Check(context.TODO())
	expectedError = errors.New("403 Forbidden")
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected '%s', got '%s'", expectedError.Error(), err.Error())
	}
}
