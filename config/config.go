package config

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAwsConfig(ctx context.Context) aws.Config {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Printf("unable to load SDK config: %v", err)
	}
	return cfg
}

func NewK8sClientset() (*kubernetes.Clientset, error) {
	config, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("error kubernetes NewForConfig: %v", err)
		return nil, err
	}
	return clientset, nil
}

func NewK8sClient(options client.Options) (client.Client, error) {
	config := ctrl.GetConfigOrDie()
	c, err := client.New(config, options)

	if err != nil {
		log.Printf("error kubernetes client: %v", err)
		return nil, err
	}
	return c, nil
}
