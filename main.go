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
	"os"
	"runtime"
	"strconv"
	"strings"

	certmanager "github.com/cw-sakamoto/kibertas/cmd/cert-manager"
	clusterautoscaler "github.com/cw-sakamoto/kibertas/cmd/cluster-autoscaler"
	"github.com/cw-sakamoto/kibertas/cmd/fluent"
	"github.com/cw-sakamoto/kibertas/cmd/ingress"
	"github.com/cw-sakamoto/kibertas/util/notify"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func main() {
	var debug bool
	var logLevel string
	var logger func() *logrus.Entry
	var chatwork *notify.Chatwork
	var rootCmd = &cobra.Command{Use: "kibertas"}

	var cmdTest = &cobra.Command{
		Use:   "test",
		Short: "test",
		Long:  "test",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	var cmdClusterAutoscaler = &cobra.Command{
		Use:   "cluster-autoscaler",
		Short: "test cluster-autoscaler",
		Long:  "test cluster-autoscaler",
		RunE: func(cmd *cobra.Command, args []string) error {
			return clusterautoscaler.NewClusterAutoscaler(debug, logger, chatwork).Check()
		},
	}

	var cmdIngress = &cobra.Command{
		Use:   "ingress",
		Short: "test ingress(aws-load-balancer-controller, external-dns)",
		Long:  "test ingress(aws-load-balancer-controller, external-dns))",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ingress.NewIngress(debug, logger, chatwork).Check()
		},
	}

	var cmdFluent = &cobra.Command{
		Use:   "fluent",
		Short: "test fluent(fluent-bit, fluentd)",
		Long:  "test fluent(fluent-bit, fluentd)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fluent.NewFluent(debug, logger, chatwork).Check()
		},
	}

	var cmdCertManager = &cobra.Command{
		Use:   "cert-manager",
		Short: "test cert-manager",
		Long:  "test cert-manager",
		RunE: func(cmd *cobra.Command, args []string) error {
			return certmanager.NewCertManager(debug, logger, chatwork).Check()
		},
	}

	var cmdAll = &cobra.Command{
		Use:   "all",
		Short: "test all application",
		Long:  "test all application",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger().Info("test all application")
			err := ingress.NewIngress(debug, logger, chatwork).Check()
			if err != nil {
				return err
			}
			err = clusterautoscaler.NewClusterAutoscaler(debug, logger, chatwork).Check()
			if err != nil {
				return err
			}

			err = certmanager.NewCertManager(debug, logger, chatwork).Check()
			if err != nil {
				return err
			}

			err = fluent.NewFluent(debug, logger, chatwork).Check()
			if err != nil {
				return err
			}
			return nil
		},
	}

	rootCmd.AddCommand(cmdTest)
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug mode")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "The log level to use. Valid values are \"debug\", \"info\", \"warn\", \"error\", and \"fatal\".")
	logger = initLogger(logLevel)
	if debug {
		logger().Debug("debug mode enabled")
	}
	logger().Debug("log level: ", logLevel)

	chatwork = initChatwork(logger)

	cmdTest.AddCommand(cmdAll)
	cmdTest.AddCommand(cmdFluent)
	cmdTest.AddCommand(cmdClusterAutoscaler)
	cmdTest.AddCommand(cmdIngress)
	cmdTest.AddCommand(cmdCertManager)

	if err := rootCmd.Execute(); err != nil {
		logger().Fatal("error: ", err)
	}
}

func initChatwork(logger func() *logrus.Entry) *notify.Chatwork {
	apiToken := os.Getenv("CHATWORK_API_TOKEN")
	roomId := os.Getenv("CHATWORK_ROOM_ID")
	return notify.NewChatwork(apiToken, roomId, logger)
}

func initLogger(logLevel string) func() *logrus.Entry {
	logr := logrus.New()
	logr.SetFormatter(&logrus.JSONFormatter{})
	level, err := logrus.ParseLevel(logLevel)

	if err != nil {
		logr.Fatal("invalid log level: ", err)
	}

	logr.SetLevel(level)

	return func() *logrus.Entry {
		_, file, line, ok := runtime.Caller(1)
		if !ok {
			panic("Could not get context info for logger!")
		}

		filename := file[strings.LastIndex(file, "/")+1:] + ":" + strconv.Itoa(line)
		//funcname := runtime.FuncForPC(pc).Name()
		//lastSlashIndex := strings.LastIndex(funcname, "/")
		//fn := funcname[lastSlashIndex+1:]
		//return logr.WithField("file", filename).WithField("function", fn)
		return logr.WithField("file", filename)
	}
}
