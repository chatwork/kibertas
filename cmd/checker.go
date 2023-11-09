package cmd

import (
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

type Checker struct {
	Namespace string
	Clientset *kubernetes.Clientset
	Debug     bool
	Logger    func() *logrus.Entry
}

func NewChecker(namespace string, clientset *kubernetes.Clientset, debug bool, logger func() *logrus.Entry) *Checker {
	return &Checker{
		Namespace: namespace,
		Clientset: clientset,
		Debug:     debug,
		Logger:    logger,
	}
}
