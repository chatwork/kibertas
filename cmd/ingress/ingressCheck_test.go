package ingress

import (
	"fmt"
	"testing"
	"time"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
	"github.com/chatwork/kibertas/util"
	"github.com/chatwork/kibertas/util/notify"

	"github.com/sirupsen/logrus"
)

func TestNewIngress(t *testing.T) {
	t.Parallel()
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{}
	ingress, err := NewIngress(true, logger, chatwork, false, "alb")
	if err != nil {
		t.Fatalf("NewIngress: %s", err)
	}

	if ingress == nil {
		t.Error("Expected ingress instance, got nil")
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

	now := time.Now()

	namespace := fmt.Sprintf("ingress-test-%d%02d%02d-%s", now.Year(), now.Month(), now.Day(), util.GenerateRandomString(5))

	// kindとingress-nginxがある前提
	// レコードは作れないのでNoDnsCheckをtrueにする
	ingress := &Ingress{
		Checker:          cmd.NewChecker(namespace, k8sclient, true, logger, chatwork),
		NoDnsCheck:       true,
		IngressClassName: "nginx",
		ResourceName:     "sample",
		ExternalHostname: "sample.example.com",
	}

	err = ingress.Check()
	if err != nil {
		t.Fatalf("Expected No Error, but got error: %s", err)
	}
}

func TestCheckDNSRecord(t *testing.T) {
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}

	k8sclient, err := config.NewK8sClientset()
	if err != nil {
		t.Fatalf("NewK8sClientset: %s", err)
	}

	chatwork := &notify.Chatwork{ApiToken: "token", RoomId: "test", Logger: logger}

	// kindとingress-nginxがある前提
	// レコードは作れないのでNoDnsCheckをtrueにする
	ingress := &Ingress{
		Checker:          cmd.NewChecker("test", k8sclient, true, logger, chatwork),
		NoDnsCheck:       true,
		IngressClassName: "nginx",
		ResourceName:     "sample",
		ExternalHostname: "go.chatwork.com",
	}

	err = ingress.checkDNSRecord()
	if err != nil {
		t.Fatalf("Expected No Error, but got error: %s", err)
	}
}
