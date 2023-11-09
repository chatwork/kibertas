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

func NewAwsConfig() aws.Config {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}
	return cfg
}

func NewK8sClientset() *kubernetes.Clientset {
	config := ctrl.GetConfigOrDie()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal("error kubernetes NewForConfig: ", err)
		panic(err)
	}
	return clientset
}

func NewK8sClient(options client.Options) client.Client {
	config := ctrl.GetConfigOrDie()
	c, err := client.New(config, options)

	if err != nil {
		log.Fatal("error kubernetes client: ", err)
		panic(err)
	}
	return c
}
