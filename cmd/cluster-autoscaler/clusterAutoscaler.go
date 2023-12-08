package clusterautoscaler

import (
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
	"github.com/chatwork/kibertas/util"
	"github.com/chatwork/kibertas/util/k8s"
	"github.com/hashicorp/go-multierror"
)

type ClusterAutoscaler struct {
	*cmd.Checker
	Clientset      *kubernetes.Clientset
	Namespace      string
	ResourceName   string
	ReplicaCount   int
	NodeLabelKey   string
	NodeLabelValue string
}

func NewClusterAutoscaler(checker *cmd.Checker) (*ClusterAutoscaler, error) {
	t := time.Now()

	namespace := fmt.Sprintf("cluster-autoscaler-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))

	checker.Logger().Infof("cluster-autoscaler check application Namespace: %s", namespace)
	checker.Chatwork.AddMessage(fmt.Sprintf("cluster-autoscaler check application Namespace: %s\n", namespace))

	resourceName := "sample-for-scale"
	nodeLabelKey := "eks.amazonaws.com/capacityType"
	nodeLabelValue := "SPOT"

	if v := os.Getenv("RESOURCE_NAME"); v != "" {
		resourceName = v
	}

	if v := os.Getenv("NODE_LABEL_KEY"); v != "" {
		nodeLabelKey = v
	}

	if v := os.Getenv("NODE_LABEL_VALUE"); v != "" {
		nodeLabelValue = v
	}

	k8sclientset, err := config.NewK8sClientset()
	if err != nil {
		checker.Logger().Error("Error NewK8sClientset: ", err)
	}

	return &ClusterAutoscaler{
		Checker:        checker,
		Clientset:      k8sclientset,
		Namespace:      namespace,
		ResourceName:   resourceName,
		NodeLabelKey:   nodeLabelKey,
		NodeLabelValue: nodeLabelValue,
	}, nil
}

// Check is check cluster-autoscaler
// replicaをノード数+1でdeploymentを作成する
func (c *ClusterAutoscaler) Check() error {
	c.Chatwork.AddMessage("cluster-autoscaler check start\n")
	defer c.Chatwork.Send()

	nodeListOption := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", c.NodeLabelKey, c.NodeLabelValue),
	}

	nodes, err := c.Clientset.CoreV1().Nodes().List(c.Ctx, nodeListOption)
	if err != nil {
		c.Logger().Errorf("Error List Nodes: %s", err)
		c.Chatwork.AddMessage(fmt.Sprintf("Error List Nodes: %s\n", err))
		return err
	}

	c.ReplicaCount = len(nodes.Items) + 1
	c.Logger().Infof("Nodes(have label: %s=%s): %d", c.NodeLabelKey, c.NodeLabelValue, len(nodes.Items))
	c.Chatwork.AddMessage(fmt.Sprintf("Nodes(have label: %s=%s): %d\n", c.NodeLabelKey, c.NodeLabelValue, len(nodes.Items)))

	defer func() {
		if err := c.cleanUpResources(); err != nil {
			c.Chatwork.AddMessage(fmt.Sprintf("Error Delete Resources: %s\n", err))
		}
	}()

	if err := c.createResources(); err != nil {
		return err
	}
	return nil
}

func (c *ClusterAutoscaler) createResources() error {
	k := k8s.NewK8s(c.Namespace, c.Clientset, c.Logger)

	if err := k.CreateNamespace(
		c.Ctx,
		&apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: c.Namespace,
			}}); err != nil {
		c.Chatwork.AddMessage(fmt.Sprintf("Error Create Namespace: %s\n", err))
		return err
	}

	c.Chatwork.AddMessage(fmt.Sprintf("Create Deployment with desire replicas %d\n", c.ReplicaCount))
	if err := k.CreateDeployment(c.Ctx, c.createDeploymentObject(), c.Timeout); err != nil {
		c.Chatwork.AddMessage(fmt.Sprintf("Error Create Deployment: %s\n", err))
		return err
	}

	c.Chatwork.AddMessage("cluster-autoscaler check finished\n")

	return nil
}

func (c *ClusterAutoscaler) cleanUpResources() error {
	if c.Debug {
		c.Logger().Info("Skip Delete Resources")
		c.Chatwork.AddMessage("Skip Delete Resources\n")
		return nil
	}
	k := k8s.NewK8s(c.Namespace, c.Clientset, c.Logger)
	var result *multierror.Error
	var err error
	if err = k.DeleteDeployment(c.ResourceName); err != nil {
		c.Chatwork.AddMessage(fmt.Sprintf("Error Delete Deployment: %s", err))
		result = multierror.Append(result, err)
	}

	if err = k.DeleteNamespace(); err != nil {
		c.Chatwork.AddMessage(fmt.Sprintf("Error Delete Namespace: %s", err))
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func (c *ClusterAutoscaler) createDeploymentObject() *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.ResourceName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Ptr(int32(c.ReplicaCount)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": c.ResourceName,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": c.ResourceName,
					},
				},
				Spec: apiv1.PodSpec{
					Affinity: &apiv1.Affinity{
						NodeAffinity: &apiv1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &apiv1.NodeSelector{
								NodeSelectorTerms: []apiv1.NodeSelectorTerm{
									{
										MatchExpressions: []apiv1.NodeSelectorRequirement{
											{
												Key:      c.NodeLabelKey,
												Operator: apiv1.NodeSelectorOpIn,
												Values:   []string{c.NodeLabelValue},
											},
										},
									},
								},
							},
						},
						PodAntiAffinity: &apiv1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []apiv1.PodAffinityTerm{
								{
									TopologyKey: "kubernetes.io/hostname",
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "app",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{c.ResourceName},
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
