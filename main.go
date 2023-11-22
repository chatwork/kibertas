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
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

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
	var debug bool
	var logLevel string
	var ingressClassName string
	var noDnsCheck bool
	var logger func() *logrus.Entry
	var chatwork *notify.Chatwork
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
		RunE: func(cmd *cobra.Command, args []string) error {
			ca, err := clusterautoscaler.NewClusterAutoscaler(debug, logger, chatwork)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			i, err := ingress.NewIngress(debug, logger, chatwork, noDnsCheck, ingressClassName)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := fluent.NewFluent(debug, logger, chatwork)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			da, err := datadogagent.NewDatadogAgent(debug, logger, chatwork)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			cm, err := certmanager.NewCertManager(debug, logger, chatwork)
			if err != nil {
				return err
			}
			return cm.Check()
		},
	}

	/*
		var cmdAll = &cobra.Command{
			Use:   "all",
			Short: "test all application",
			Long:  "test all application",
			RunE: func(cmd *cobra.Command, args []string) error {
				logger().Info("test all application")
				i, err := ingress.NewIngress(debug, logger, chatwork, noDnsCheck, ingressClassName)
				if err != nil {
					return err
				}

				err = i.Check()
				if err != nil {
					return err
				}

				ca, err := clusterautoscaler.NewClusterAutoscaler(debug, logger, chatwork)
				if err != nil {
					return err
				}

				err = ca.Check()
				if err != nil {
					return err
				}

				cm, err := certmanager.NewCertManager(debug, logger, chatwork)
				if err != nil {
					return err
				}

				err = cm.Check()
				if err != nil {
					return err
				}

				f, err := fluent.NewFluent(debug, logger, chatwork)
				if err != nil {
					return err
				}

				err = f.Check()
				if err != nil {
					return err
				}
				return nil
			},
		}
	*/

	rootCmd.AddCommand(cmdTest)
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug mode")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "The log level to use. Valid values are \"debug\", \"info\", \"warn\", \"error\", and \"fatal\".")
	logger = initLogger(logLevel)
	if debug {
		logger().Debug("debug mode enabled")
	}
	logger().Debug("log level: ", logLevel)

	chatwork = initChatwork(logger)

	//cmdTest.AddCommand(cmdAll)
	cmdTest.AddCommand(cmdFluent)
	cmdTest.AddCommand(cmdClusterAutoscaler)
	cmdTest.AddCommand(cmdIngress)
	cmdTest.AddCommand(cmdCertManager)
	cmdTest.AddCommand(cmdDatadogAgent)

	cmdIngress.Flags().BoolVar(&noDnsCheck, "no-dns-check", false, "This is a flag for the dns check. If you want to skip the dns check, please specify false.(default: false)")
	cmdIngress.Flags().StringVar(&ingressClassName, "ingress-class-name", "alb", "This is a flag for the ingress class name. If you want to change the ingress class name, please specify the name.(default: alb)")

	if err := rootCmd.Execute(); err != nil {
		logger().Fatal("error: ", err)
	}
}

func initChatwork(logger func() *logrus.Entry) *notify.Chatwork {
	apiToken := os.Getenv("CHATWORK_API_TOKEN")
	roomId := os.Getenv("CHATWORK_ROOM_ID")
	clusterName := os.Getenv("CLUSTER_NAME")
	chatwork := notify.NewChatwork(apiToken, roomId, logger)
	chatwork.AddMessage(fmt.Sprintf("kibertas start in %s\n", clusterName))
	return chatwork
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
