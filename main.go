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
	"log"

	certmanager "github.com/cw-sakamoto/kibertas/cmd/cert-manager"
	clusterautoscaler "github.com/cw-sakamoto/kibertas/cmd/cluster-autoscaler"
	"github.com/cw-sakamoto/kibertas/cmd/fluent"
	"github.com/cw-sakamoto/kibertas/cmd/ingress"
	"github.com/spf13/cobra"
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

func main() {
	var debug bool
	var logLevel string
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
			return clusterautoscaler.NewClusterAutoscaler().Check()
		},
	}

	var cmdIngress = &cobra.Command{
		Use:   "ingress",
		Short: "test ingress(aws-load-balancer-controller, external-dns)",
		Long:  "test ingress(aws-load-balancer-controller, external-dns))",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ingress.NewIngress().Check()
		},
	}

	var cmdFluent = &cobra.Command{
		Use:   "fluent",
		Short: "test fluent(fluent-bit, fluentd)",
		Long:  "test fluent(fluent-bit, fluentd)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fluent.NewFluent().Check()
		},
	}

	var cmdCertManager = &cobra.Command{
		Use:   "cert-manager",
		Short: "test cert-manager",
		Long:  "test cert-manager",
		RunE: func(cmd *cobra.Command, args []string) error {
			return certmanager.NewCertManager(debug, logLevel).Check()
		},
	}

	var cmdAll = &cobra.Command{
		Use:   "all",
		Short: "test all application",
		Long:  "test all application",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := ingress.NewIngress().Check()
			if err != nil {
				return err
			}
			err = fluent.NewFluent().Check()
			if err != nil {
				return err
			}

			err = clusterautoscaler.NewClusterAutoscaler().Check()
			if err != nil {
				return err
			}

			err = certmanager.NewCertManager(debug, logLevel).Check()
			if err != nil {
				return err
			}
			return nil
		},
	}

	rootCmd.AddCommand(cmdTest)
	cmdTest.AddCommand(cmdAll)
	cmdTest.AddCommand(cmdFluent)
	cmdTest.AddCommand(cmdClusterAutoscaler)
	cmdTest.AddCommand(cmdIngress)
	cmdTest.AddCommand(cmdCertManager)

	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug mode")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "The log level to use. Valid values are \"debug\", \"info\", \"warn\", \"error\", and \"fatal\".")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal("error: ", err)
		panic(err)
	}
}
