package clusterautoscaler

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/cw-sakamoto/kibertas/config"
	"github.com/cw-sakamoto/kibertas/util"
	"github.com/cw-sakamoto/kibertas/util/k8s"
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

type ClusterAutoscaler struct {
	namespace      string
	deploymentName string
	clientset      *kubernetes.Clientset
}

func NewClusterAutoscaler() *ClusterAutoscaler {
	t := time.Now()

	namespace := fmt.Sprintf("cluster-autoscaler-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))
	log.Printf("cluster-autoscaler check application namespace: %s\n", namespace)

	deploymentName := "sample-for-scale"

	if v := os.Getenv("DEPLOYMENT_NAME"); v != "" {
		deploymentName = v
	}

	return &ClusterAutoscaler{
		namespace:      namespace,
		deploymentName: deploymentName,
		clientset:      config.InitK8sConfig(),
	}
}

func (c *ClusterAutoscaler) Check() error {
	k8s.CreateNamespace(c.namespace, c.clientset)
	defer k8s.DeleteNamespace(c.namespace, c.clientset)

	nodeListOption := metav1.ListOptions{
		LabelSelector: "eks.amazonaws.com/capacityType=SPOT",
	}

	nodes, err := c.clientset.CoreV1().Nodes().List(context.TODO(), nodeListOption)

	if err != nil {
		return err
	}

	log.Printf("spot nodes: %d\n", len(nodes.Items))
	desireReplicacount := len(nodes.Items) + 1

	err = k8s.CreateDeployment(createDeploymentObject(c.deploymentName, desireReplicacount), c.namespace, c.clientset)
	if err != nil {
		return err
	}
	defer k8s.DeleteDeployment(c.deploymentName, c.namespace, c.clientset)
	return nil
}

func createDeploymentObject(deploymentName string, desireReplicaCount int) *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: deploymentName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Ptr(int32(desireReplicaCount)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": deploymentName,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": deploymentName,
					},
				},
				Spec: apiv1.PodSpec{
					Affinity: &apiv1.Affinity{
						PodAntiAffinity: &apiv1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []apiv1.PodAffinityTerm{
								{
									TopologyKey: "kubernetes.io/hostname",
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "app",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{deploymentName},
											},
										},
									},
								},
							},
						},
					},
					Containers: []apiv1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
							Ports: []apiv1.ContainerPort{
								{
									Name:          "http",
									Protocol:      apiv1.ProtocolTCP,
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
		},
	}

	return deployment
}
