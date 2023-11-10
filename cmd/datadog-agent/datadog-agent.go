package datadogagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/cw-sakamoto/kibertas/cmd"
	"github.com/cw-sakamoto/kibertas/config"
	"github.com/cw-sakamoto/kibertas/util"
	"github.com/cw-sakamoto/kibertas/util/k8s"
	"github.com/cw-sakamoto/kibertas/util/notify"
	"github.com/sirupsen/logrus"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

type DatadogAgent struct {
	*cmd.Checker
	ApiKey         string
	AppKey         string
	ClusterName    string
	DeploymentName string
}

func NewDatadogAgent(debug bool, logger func() *logrus.Entry, chatwork *notify.Chatwork) *DatadogAgent {
	t := time.Now()

	namespace := fmt.Sprintf("datadog-agent-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))

	logger().Infof("datadog-agent check application namespace: %s\n", namespace)
	chatwork.AddMessage(fmt.Sprintf("datadog-agent check application namespace: %s\n", namespace))

	apiKey := ""
	appKey := ""
	clusterName := ""
	deploymentName := "sample-for-datadog"

	if v := os.Getenv("DD_API_KEY"); v != "" {
		apiKey = v
	}
	if v := os.Getenv("DD_APP_KEY"); v != "" {
		appKey = v
	}
	if v := os.Getenv("CLUSTER_NAME"); v != "" {
		clusterName = v
	}
	if v := os.Getenv("DEPLOYMENT_NAME"); v != "" {
		deploymentName = v
	}

	return &DatadogAgent{
		Checker:        cmd.NewChecker(namespace, config.NewK8sClientset(), debug, logger, chatwork),
		ApiKey:         apiKey,
		AppKey:         appKey,
		ClusterName:    clusterName,
		DeploymentName: deploymentName,
	}
}

func (d *DatadogAgent) Check() error {
	if d.ApiKey == "" || d.AppKey == "" {
		d.Logger().Error("DD_API_KEY or DD_APP_KEY is empty")
		d.Chatwork.AddMessage("DD_API_KEY or DD_APP_KEY is empty\n")
		d.Chatwork.Send()
		return errors.New("DD_API_KEY or DD_APP_KEY is empty")
	}
	if d.ClusterName == "" {
		d.Logger().Error("CLUSTER_NAME is empty")
		d.Chatwork.AddMessage("CLUSTER_NAME is empty\n")
		d.Chatwork.Send()
		return errors.New("CLUSTER_NAME is empty")
	}

	d.Chatwork.AddMessage("datadog-agent check start\n")
	defer d.Chatwork.Send()

	if err := d.prepareResources(); err != nil {
		d.Chatwork.AddMessage(fmt.Sprintf("prepareResources error: %s\n", err.Error()))
		return err
	}
	defer d.cleanupResources()

	if err := d.checkMetrics(); err != nil {
		d.Chatwork.AddMessage(fmt.Sprintf("checkMetrics error: %s\n", err.Error()))
		return err
	}
	return nil
}

func (d *DatadogAgent) cleanupResources() error {
	k := k8s.NewK8s(d.Namespace, d.Clientset, d.Debug, d.Logger)
	k.DeleteDeployment(d.DeploymentName)
	k.DeleteNamespace()
	return nil
}

func (d *DatadogAgent) prepareResources() error {
	k := k8s.NewK8s(d.Namespace, d.Clientset, d.Debug, d.Logger)
	if err := k.CreateNamespace(&apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: d.Namespace,
		},
	}); err != nil {
		return err
	}

	if err := k.CreateDeployment(d.createDeploymentObject()); err != nil {
		return err
	}
	return nil
}

func (d *DatadogAgent) checkMetrics() error {
	//d.Logger().Infof(d.ApiKey)
	//d.Logger().Infof(d.AppKey)

	ctx := context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: d.ApiKey,
			},
			"appKeyAuth": {
				Key: d.AppKey,
			},
		},
	)

	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)
	metricsApi := datadogV1.NewMetricsApi(apiClient)

	now := time.Now().Unix()
	from := now - 60*10

	query := fmt.Sprintf(
		"kubernetes.cpu.user.total{env:%s, kube_deployment:%s}", d.ClusterName, d.DeploymentName)

	query = "system.cpu.idle{*}"

	d.Logger().Infof("Querying metrics with query: %s\n", query)

	resp, _, err := metricsApi.QueryMetrics(ctx, from, now, query)
	err = wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 10*time.Minute, false, func(ctx context.Context) (bool, error) {
		resp, _, err = metricsApi.QueryMetrics(ctx, from, now, query)
		if err != nil {
			d.Logger().Errorf("Error when querying metrics: %v\n", err)
			return false, err
		}

		if len(resp.GetSeries()) == 0 {
			d.Logger().Infof("No results found\n")
			return false, nil
		} else if len(resp.GetSeries()) > 0 {
			d.Logger().Infof("Metrics Results found\n")
			return true, nil
		}

		return false, nil
	})
	if err != nil {
		d.Logger().Error("Timed out waiting for metrics to be ready:", err)
		return err
	}
	return nil
}

func (d *DatadogAgent) createDeploymentObject() *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: d.DeploymentName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: util.Int32Ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": d.DeploymentName,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": d.DeploymentName,
					},
				},
				Spec: apiv1.PodSpec{
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
