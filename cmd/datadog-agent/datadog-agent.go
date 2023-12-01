package datadogagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/config"
	"github.com/chatwork/kibertas/util"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/sirupsen/logrus"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

type DatadogAgent struct {
	*cmd.Checker
	ApiKey       string
	AppKey       string
	QueryMetrics string
	WaitTime     time.Duration
}

func NewDatadogAgent(debug bool, logger func() *logrus.Entry, chatwork *notify.Chatwork) (*DatadogAgent, error) {
	t := time.Now()

	// dummy namespace
	namespace := fmt.Sprintf("datadog-agent-test-%d%02d%02d-%s", t.Year(), t.Month(), t.Day(), util.GenerateRandomString(5))

	apiKey := ""
	appKey := ""
	queryMetrics := ""
	timeout := 10

	if v := os.Getenv("DD_API_KEY"); v != "" {
		apiKey = v
	}
	if v := os.Getenv("DD_APP_KEY"); v != "" {
		appKey = v
	}

	var err error
	if v := os.Getenv("CHECK_TIMEOUT"); v != "" {
		timeout, err = strconv.Atoi(v)
		if err != nil {
			logger().Errorf("strconv.Atoi: %s", err)
			return nil, err
		}
	}

	queryMetrics = "avg:kubernetes.cpu.user.total"
	if v := os.Getenv("QUERY_METRICS"); v != "" {
		queryMetrics = v
	}

	k8sclient, err := config.NewK8sClientset()
	if err != nil {
		logger().Errorf("NewK8sClientset: %s", err)
		return nil, err
	}

	return &DatadogAgent{
		Checker:      cmd.NewChecker(namespace, k8sclient, debug, logger, chatwork, time.Duration(timeout)*time.Minute),
		ApiKey:       apiKey,
		AppKey:       appKey,
		QueryMetrics: queryMetrics,
		WaitTime:     3 * 60 * time.Second,
	}, nil
}

func (d *DatadogAgent) Check() error {
	defer d.Chatwork.Send()

	if d.ApiKey == "" || d.AppKey == "" {
		d.Logger().Error("DD_API_KEY or DD_APP_KEY is empty")
		d.Chatwork.AddMessage("DD_API_KEY or DD_APP_KEY is empty\n")
		return errors.New("DD_API_KEY or DD_APP_KEY is empty")
	}
	d.Chatwork.AddMessage("datadog-agent check start\n")

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

	d.Logger().Info("Waiting metrics...")
	time.Sleep(d.WaitTime)

	d.Logger().Infof("Querying metrics with query: %s", d.QueryMetrics)
	d.Chatwork.AddMessage(fmt.Sprintf("Querying metrics with query: %s", d.QueryMetrics))
	now := time.Now().Unix()
	from := now - 60*2
	err := wait.PollUntilContextTimeout(context.Background(), 30*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		resp, r, err := api.QueryMetrics(ddctx, from, now, d.QueryMetrics)

		if err != nil {
			if r != nil && r.StatusCode == 403 {
				return true, errors.New("403 Forbidden")
			} else if r != nil && r.StatusCode == 401 {
				return true, errors.New("401 Unauthorized")
			}
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
