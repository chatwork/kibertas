package clusterautoscaler

import (
	"context"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cw-sakamoto/kibertas/cmd"
	"github.com/cw-sakamoto/kibertas/config"
	"github.com/cw-sakamoto/kibertas/util"
	"github.com/cw-sakamoto/kibertas/util/k8s"
	"github.com/cw-sakamoto/kibertas/util/notify"
	"github.com/sirupsen/logrus"
)

type ClusterAutoscaler struct {
	*cmd.Checker
	DeploymentName string
}

func NewClusterAutoscaler(debug bool, logger func() *logrus.Entry, chatwork *notify.Chatwork) *ClusterAutoscaler {
	t := time.Now()

	namespace := fmt.Sprintf("cluster-autoscaler-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))

	logger().Infof("cluster-autoscaler check application namespace: %s\n", namespace)
	chatwork.AddMessage(fmt.Sprintf("cluster-autoscaler check application namespace: %s\n", namespace))

	deploymentName := "sample-for-scale"

	if v := os.Getenv("DEPLOYMENT_NAME"); v != "" {
		deploymentName = v
	}

	return &ClusterAutoscaler{
		Checker:        cmd.NewChecker(namespace, config.NewK8sClientset(), debug, logger, chatwork),
		DeploymentName: deploymentName,
	}
}

func (c *ClusterAutoscaler) Check() error {
	c.Chatwork.AddMessage("cluster-autoscaler check start\n")
	defer c.Chatwork.Send()
	k := k8s.NewK8s(c.Namespace, c.Clientset, c.Debug, c.Logger)

	if err := k.CreateNamespace(&apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.Namespace,
		},
	}); err != nil {
		return err
	}
	defer k.DeleteNamespace()

	nodeListOption := metav1.ListOptions{
		LabelSelector: "eks.amazonaws.com/capacityType=SPOT",
	}

	nodes, err := c.Clientset.CoreV1().Nodes().List(context.TODO(), nodeListOption)
	if err != nil {
		return err
	}

	c.Logger().Infof("spot nodes: %d", len(nodes.Items))
	c.Chatwork.AddMessage(fmt.Sprintf("spot nodes: %d\n", len(nodes.Items)))
	desireReplicacount := len(nodes.Items) + 1
	c.Chatwork.AddMessage(fmt.Sprintf("create deployment with desire replicas %d\n", desireReplicacount))

	err = k.CreateDeployment(createDeploymentObject(c.DeploymentName, desireReplicacount))
	if err != nil {
		return err
	}
	defer k.DeleteDeployment(c.DeploymentName)
	c.Chatwork.AddMessage("cluster-autoscaler check finished\n")
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
