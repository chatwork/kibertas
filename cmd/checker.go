package cmd

import (
	"time"

	"github.com/chatwork/kibertas/util/notify"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

type Checker struct {
	Namespace string
	Clientset *kubernetes.Clientset
	Debug     bool
	Logger    func() *logrus.Entry
	Chatwork  *notify.Chatwork
	Timeout   time.Duration
}

func NewChecker(namespace string, clientset *kubernetes.Clientset, debug bool, logger func() *logrus.Entry, chatwork *notify.Chatwork, timeout time.Duration) *Checker {
	return &Checker{
		Namespace: namespace,
		Clientset: clientset,
		Debug:     debug,
		Logger:    logger,
		Chatwork:  chatwork,
		Timeout:   timeout,
	}
}
