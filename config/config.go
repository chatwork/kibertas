package config

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func NewAwsConfig(ctx context.Context) (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return aws.Config{}, err
	}

	if cfg.Region == "" {
		// Region is required by the S3 client.
		// If not set, methods like ListObjectsV2 will fail saying:
		//   level=warning msg="Got an error retrieving items: operation error S3: ListObjectsV2, resolve auth scheme: resolve endpoint: endpoint rule error, Invalid region: region was not a valid DNS name."
		return aws.Config{}, errors.New("region is empty: please set AWS_DEFAULT_REGION")
	}
	return cfg, nil
}

// NewK8sClientset returns a new kubernetes clientset
// using the KUBECONFIG environment variable if set.
// Otherwise, it uses the in-cluster config.
func NewK8sClientset() (*kubernetes.Clientset, error) {
	// GetConfig is expected to respect the KUBECONFIG
	// environment variable if set.
	config, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

func NewK8sClient(options client.Options) (client.Client, error) {
	// https://github.com/kubernetes-sigs/controller-runtime/blob/main/pkg/log/log.go#L58
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	config := ctrl.GetConfigOrDie()
	c, err := client.New(config, options)

	if err != nil {
		return nil, err
	}
	return c, nil
}
