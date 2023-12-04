package certmanager

import (
	"context"
	"fmt"
	"testing"
	"time"

	cmapiv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/chatwork/kibertas/util"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
)

func TestNewCertManager(t *testing.T) {
	t.Parallel()
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{}
	ingress, err := NewCertManager(true, logger, chatwork)
	if err != nil {
		t.Fatalf("NewCertManager: %s", err)
	}

	if ingress == nil {
		t.Error("Expected certManager instance, got nil")
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}

	k8sclientset, err := config.NewK8sClientset()
	if err != nil {
		t.Fatalf("NewK8sClientset: %s", err)
	}

	scheme := runtime.NewScheme()
	_ = cmapiv1.AddToScheme(scheme)

	k8sclient, err := config.NewK8sClient(client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("NewK8sClient: %s", err)
	}

	chatwork := &notify.Chatwork{ApiToken: "token", RoomId: "test", Logger: logger}

	now := time.Now()
	namespace := fmt.Sprintf("cert-manager-test-%d%02d%02d-%s", now.Year(), now.Month(), now.Day(), util.GenerateRandomString(5))
	cm := &CertManager{
		Checker:  cmd.NewChecker(namespace, k8sclientset, true, logger, chatwork, 3*time.Minute),
		CertName: "sample",
		Client:   k8sclient,
	}

	err = cm.Check(context.TODO())
	if err != nil {
		t.Fatalf("Expected No Error, but got error: %s", err)
	}
}
