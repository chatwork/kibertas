package datadogagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/sirupsen/logrus"
)

func TestNewDatadogAgent(t *testing.T) {
	t.Parallel()
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{}
	checker := cmd.NewChecker(context.TODO(), false, logger, chatwork, 3*time.Minute)
	datadogAgent, err := NewDatadogAgent(checker)
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

	chatwork := &notify.Chatwork{ApiToken: "token", RoomId: "test", Logger: logger}
	datadogAgent := &DatadogAgent{
		Checker:      cmd.NewChecker(context.TODO(), false, logger, chatwork, 1*time.Minute),
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
		Checker:      cmd.NewChecker(context.TODO(), false, logger, chatwork, 3*time.Minute),
		ApiKey:       "test",
		AppKey:       "test",
		QueryMetrics: "avg:kubernetes.cpu.user.total",
		WaitTime:     1 * time.Second,
	}

	err = datadogAgent.Check()
	expectedError = errors.New("error waiting for query metrics results: 403 Forbidden")
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected '%s', got '%s'", expectedError.Error(), err.Error())
	}
}
