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
		Run: func(cmd *cobra.Command, args []string) {
			clusterautoscaler.NewClusterAutoscaler().Check()
		},
	}

	var cmdIngress = &cobra.Command{
		Use:   "ingress",
		Short: "test ingress(aws-load-balancer-controller, external-dns)",
		Long:  "test ingress(aws-load-balancer-controller, external-dns))",
		Run: func(cmd *cobra.Command, args []string) {
			ingress.NewIngress().Check()
		},
	}

	var cmdFluent = &cobra.Command{
		Use:   "fluent",
		Short: "test fluent(fluent-bit, fluentd)",
		Long:  "test fluent(fluent-bit, fluentd)",
		Run: func(cmd *cobra.Command, args []string) {
			fluent.NewFluent().Check()
		},
	}

	rootCmd.AddCommand(cmdTest)
	cmdTest.AddCommand(cmdFluent)
	cmdTest.AddCommand(cmdClusterAutoscaler)
	cmdTest.AddCommand(cmdIngress)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal("error: ", err)
		panic(err)
	}
}
