package fluent

import (
	"testing"

	"github.com/chatwork/kibertas/util/notify"

	"github.com/sirupsen/logrus"
)

func TestNewFluent(t *testing.T) {
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{}
	fluent, err := NewFluent(true, logger, chatwork)
	if err != nil {
		t.Fatalf("NewFluent: %s", err)
	}

	if fluent == nil {
		t.Error("Expected fluent instance, got nil")
	}
}
