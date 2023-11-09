package cmd

import (
	"github.com/cw-sakamoto/kibertas/util/notify"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

type Checker struct {
	Namespace string
	Clientset *kubernetes.Clientset
	Debug     bool
	Logger    func() *logrus.Entry
	Chatwork  *notify.Chatwork
}

func NewChecker(namespace string, clientset *kubernetes.Clientset, debug bool, logger func() *logrus.Entry, chatwork *notify.Chatwork) *Checker {
	return &Checker{
		Namespace: namespace,
		Clientset: clientset,
		Debug:     debug,
		Logger:    logger,
		Chatwork:  chatwork,
	}
}
