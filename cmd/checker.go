package cmd

import (
	"context"
	"time"

	"github.com/chatwork/kibertas/util/notify"
	"github.com/sirupsen/logrus"
)

type Checker struct {
	Ctx      context.Context
	Debug    bool
	Logger   func() *logrus.Entry
	Chatwork *notify.Chatwork
	Timeout  time.Duration
}

func NewChecker(ctx context.Context, debug bool, logger func() *logrus.Entry, chatwork *notify.Chatwork, timeout time.Duration) *Checker {
	logger().Info("Checker timeout: ", timeout)

	return &Checker{
		Ctx:      ctx,
		Debug:    debug,
		Logger:   logger,
		Chatwork: chatwork,
		Timeout:  timeout,
	}
}
