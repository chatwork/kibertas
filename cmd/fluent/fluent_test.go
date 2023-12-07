package fluent

import (
	"context"
	"testing"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/util/notify"

	"github.com/sirupsen/logrus"
)

func TestNewFluent(t *testing.T) {
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{}
	checker := cmd.NewChecker(context.Background(), false, logger, chatwork, 3*time.Minute)
	fluent, err := NewFluent(checker)
	if err != nil {
		t.Fatalf("NewFluent: %s", err)
	}

	if fluent == nil {
		t.Error("Expected fluent instance, got nil")
	}
}
