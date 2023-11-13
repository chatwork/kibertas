package datadogagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/cw-sakamoto/kibertas/cmd"
	"github.com/cw-sakamoto/kibertas/config"
	"github.com/cw-sakamoto/kibertas/util"
	"github.com/cw-sakamoto/kibertas/util/notify"
	"github.com/sirupsen/logrus"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

type DatadogAgent struct {
	*cmd.Checker
	ApiKey      string
	AppKey      string
	ClusterName string
}

func NewDatadogAgent(debug bool, logger func() *logrus.Entry, chatwork *notify.Chatwork) *DatadogAgent {
	t := time.Now()

	namespace := fmt.Sprintf("datadog-agent-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))

	logger().Infof("datadog-agent check application namespace: %s", namespace)
	chatwork.AddMessage(fmt.Sprintf("datadog-agent check application namespace: %s\n", namespace))

	apiKey := ""
	appKey := ""
	clusterName := ""

	if v := os.Getenv("DD_API_KEY"); v != "" {
		apiKey = v
	}
	if v := os.Getenv("DD_APP_KEY"); v != "" {
		appKey = v
	}
	if v := os.Getenv("CLUSTER_NAME"); v != "" {
		clusterName = v
	}
	return &DatadogAgent{
		Checker:     cmd.NewChecker(namespace, config.NewK8sClientset(), debug, logger, chatwork),
		ApiKey:      apiKey,
		AppKey:      appKey,
		ClusterName: clusterName,
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

	if err := d.checkMetrics(); err != nil {
		d.Chatwork.AddMessage(fmt.Sprintf("checkMetrics error: %s\n", err.Error()))
		return err
	}
	d.Chatwork.AddMessage("datadog-agent check finished\n")
	return nil
}

func (d *DatadogAgent) checkMetrics() error {
	keys := make(map[string]datadog.APIKey)
	keys["apiKeyAuth"] = datadog.APIKey{Key: d.ApiKey}
	keys["appKeyAuth"] = datadog.APIKey{Key: d.AppKey}

	ddctx := datadog.NewDefaultContext(context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		keys,
	))

	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)
	api := datadogV1.NewMetricsApi(apiClient)
	query := fmt.Sprintf("avg:kubernetes.cpu.user.total{env:%s}", d.ClusterName)

	d.Logger().Info("Waiting metrics...")
	time.Sleep((60 * 3) * time.Second)

	d.Logger().Infof("Querying metrics with query: %s", query)
	d.Chatwork.AddMessage(fmt.Sprintf("Querying metrics with query: %s", query))
	now := time.Now().Unix()
	from := now - 60*2
	err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		resp, _, err := api.QueryMetrics(ddctx, from, now, query)

		if err != nil {
			d.Logger().Errorf("Error when querying metrics: %v", err)
			return false, err
		}

		if len(resp.GetSeries()) == 0 {
			d.Logger().Infof("No results found\n")
			return false, nil
		} else if len(resp.GetSeries()) > 0 {
			d.Logger().Infof("Response from `MetricsApi.QueryMetrics`")
			d.Chatwork.AddMessage("Response from `MetricsApi.QueryMetrics`\n")
			responseContent, _ := json.MarshalIndent(resp, "", "  ")
			d.Logger().Debugf("Response:%s", responseContent)
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
