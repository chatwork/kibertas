/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Note: the example only works with the code within the same release/branch.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chatwork/kibertas/cmd"
	certmanager "github.com/chatwork/kibertas/cmd/cert-manager"
	clusterautoscaler "github.com/chatwork/kibertas/cmd/cluster-autoscaler"
	datadogagent "github.com/chatwork/kibertas/cmd/datadog-agent"
	"github.com/chatwork/kibertas/cmd/fluent"
	"github.com/chatwork/kibertas/cmd/ingress"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func main() {
	var logLevel string

	var checker *cmd.Checker
	var debug bool
	var timeout int
	var logger func() *logrus.Entry
	var chatwork *notify.Chatwork

	var ctx context.Context

	var noDnsCheck bool

	clusterName := os.Getenv("CLUSTER_NAME")

	var rootCmd = &cobra.Command{
		Use:           "kibertas",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	var cmdTest = &cobra.Command{
		Use:   "test",
		Short: "test",
		Long:  "test",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	var cmdClusterAutoscaler = &cobra.Command{
		Use:   "cluster-autoscaler",
		Short: "test cluster-autoscaler",
		Long:  "test cluster-autoscaler",
		RunE: func(cobra_cmd *cobra.Command, args []string) error {
			checker = cmd.NewChecker(ctx, debug, logger, chatwork, clusterName, time.Duration(timeout)*time.Minute)
			ca, err := clusterautoscaler.NewClusterAutoscaler(checker)
			if err != nil {
				return err
			}
			return ca.Check()
		},
	}

	var cmdIngress = &cobra.Command{
		Use:   "ingress",
		Short: "test ingress",
		Long:  "test ingress(ingress-controller, external-dns)",
		RunE: func(cobra_cmd *cobra.Command, args []string) error {
			checker = cmd.NewChecker(ctx, debug, logger, chatwork, clusterName, time.Duration(timeout)*time.Minute)
			i, err := ingress.NewIngress(checker, noDnsCheck)
			if err != nil {
				return err
			}
			return i.Check()
		},
	}

	var cmdFluent = &cobra.Command{
		Use:   "fluent",
		Short: "test fluent(fluent-bit, fluentd)",
		Long:  "test fluent(fluent-bit, fluentd)",
		RunE: func(cobra_cmd *cobra.Command, args []string) error {
			checker = cmd.NewChecker(ctx, debug, logger, chatwork, clusterName, time.Duration(timeout)*time.Minute)
			f, err := fluent.NewFluent(checker)
			if err != nil {
				return err
			}
			return f.Check()
		},
	}

	var cmdDatadogAgent = &cobra.Command{
		Use:   "datadog-agent",
		Short: "test datadog-agent",
		Long:  "test datadog-agent",
		RunE: func(cobra_cmd *cobra.Command, args []string) error {
			checker = cmd.NewChecker(ctx, debug, logger, chatwork, clusterName, time.Duration(timeout)*time.Minute)
			da, err := datadogagent.NewDatadogAgent(checker)
			if err != nil {
				return err
			}
			return da.Check()
		},
	}

	var cmdCertManager = &cobra.Command{
		Use:   "cert-manager",
		Short: "test cert-manager",
		Long:  "test cert-manager",
		RunE: func(cobra_cmd *cobra.Command, args []string) error {
			checker = cmd.NewChecker(ctx, debug, logger, chatwork, clusterName, time.Duration(timeout)*time.Minute)
			cm, err := certmanager.NewCertManager(checker)
			if err != nil {
				return err
			}
			return cm.Check()
		},
	}

	var cmdAll = &cobra.Command{
		Use:   "all",
		Short: "test all application",
		Long:  "test all application",
		RunE: func(cobra_cmd *cobra.Command, args []string) error {
			logger().Info("test all application")
			checker = cmd.NewChecker(ctx, debug, logger, chatwork, clusterName, time.Duration(timeout)*time.Minute)
			ca, err := clusterautoscaler.NewClusterAutoscaler(checker)
			if err != nil {
				return err
			}
			if err := ca.Check(); err != nil {
				return err
			}

			//checker.Chatwork = initChatwork(logger)
			i, err := ingress.NewIngress(checker, false)
			if err != nil {
				return err
			}
			if err := i.Check(); err != nil {
				return err
			}

			//checker.Chatwork = initChatwork(logger)
			f, err := fluent.NewFluent(checker)
			if err != nil {
				return err
			}
			if err := f.Check(); err != nil {
				return err
			}

			//checker.Chatwork = initChatwork(logger)
			da, err := datadogagent.NewDatadogAgent(checker)
			if err != nil {
				return err
			}
			if err := da.Check(); err != nil {
				return err
			}

			//checker.Chatwork = initChatwork(logger)
			cm, err := certmanager.NewCertManager(checker)
			if err != nil {
				return err
			}
			if err := cm.Check(); err != nil {
				return err
			}
			return nil
		},
	}

	rootCmd.AddCommand(cmdTest)
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 15, "Check timeout. If you want to change the timeout, please specify the number of minutes.")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug mode")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "The log level to use. Valid values are \"debug\", \"info\", \"warn\", \"error\", and \"fatal\".")
	logger, err := initLogger(logLevel, debug)
	if err != nil {
		panic(err)
	}
	if debug {
		logger().Debug("debug mode enabled")
	}
	logger().Debug("log level: ", logLevel)

	chatwork = initChatwork(logger)
	ctx = newSignalContext(logger, chatwork)

	cmdIngress.Flags().BoolVar(&noDnsCheck, "no-dns-check", false, "This is a flag for the dns check. If you want to skip the dns check, please specify false.(default: false)")

	cmdTest.AddCommand(cmdAll)
	cmdTest.AddCommand(cmdFluent)
	cmdTest.AddCommand(cmdClusterAutoscaler)
	cmdTest.AddCommand(cmdIngress)
	cmdTest.AddCommand(cmdCertManager)
	cmdTest.AddCommand(cmdDatadogAgent)

	if err := rootCmd.Execute(); err != nil {
		chatwork.AddMessage("Error: " + err.Error() + "\n")
		logger().Fatal("Error: ", err)
	}
}

func newSignalContext(logger func() *logrus.Entry, chatwork *notify.Chatwork) context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		logger().Info("Received Ctrl+C or SIGTERM. Exiting...")
		chatwork.AddMessage("Received Ctrl+C or SIGERM. Exiting...\n")
		cancel()
	}()

	return ctx
}

func initChatwork(logger func() *logrus.Entry) *notify.Chatwork {
	apiToken := os.Getenv("CHATWORK_API_TOKEN")
	roomId := os.Getenv("CHATWORK_ROOM_ID")
	chatwork := notify.NewChatwork(apiToken, roomId, logger)

	return chatwork
}

func initLogger(logLevel string, debug bool) (func() *logrus.Entry, error) {
	logr := logrus.New()
	logr.SetFormatter(&logrus.JSONFormatter{})

	level, err := logrus.ParseLevel(logLevel)

	if err != nil {
		logr.Infof("invalid log level: %v", err)
		return nil, err
	}

	if debug {
		logr.SetLevel(logrus.DebugLevel)
	} else {
		logr.SetLevel(level)
	}

	return func() *logrus.Entry {
		_, file, line, ok := runtime.Caller(1)
		if !ok {
			logr.Warn("Could not get context info for logger!")
			return logr.WithField("file", "unknown")
		}

		filename := file[strings.LastIndex(file, "/")+1:] + ":" + strconv.Itoa(line)
		//funcname := runtime.FuncForPC(pc).Name()
		//lastSlashIndex := strings.LastIndex(funcname, "/")
		//fn := funcname[lastSlashIndex+1:]
		//return logr.WithField("file", filename).WithField("function", fn)
		return logr.WithField("file", filename)
	}, nil
}
