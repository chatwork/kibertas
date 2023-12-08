package datadogagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/util"

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

func NewDatadogAgent(checker *cmd.Checker) (*DatadogAgent, error) {
	apiKey := ""
	appKey := ""
	queryMetrics := ""

	if v := os.Getenv("DD_API_KEY"); v != "" {
		apiKey = v
	}
	if v := os.Getenv("DD_APP_KEY"); v != "" {
		appKey = v
	}

	queryMetrics = "avg:kubernetes.cpu.user.total"
	if v := os.Getenv("QUERY_METRICS"); v != "" {
		queryMetrics = v
	}

	return &DatadogAgent{
		Checker:      checker,
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
		d.Ctx,
		datadog.ContextAPIKeys,
		keys,
	))

	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)
	api := datadogV1.NewMetricsApi(apiClient)

	d.Logger().Infof("Querying metrics with query: %s", d.QueryMetrics)
	d.Chatwork.AddMessage(fmt.Sprintf("Querying metrics with query: %s\n", d.QueryMetrics))

	d.Logger().Info("Waiting metrics...")

	if err := util.SleepContext(d.Ctx, d.WaitTime); err != nil {
		return err
	}

	now := time.Now().Unix()
	from := now - 60*2
	err := wait.PollUntilContextTimeout(d.Ctx, 30*time.Second, d.Timeout, true, func(ctx context.Context) (bool, error) {
		resp, r, err := api.QueryMetrics(ddctx, from, now, d.QueryMetrics)

		if err != nil {
			if r != nil && r.StatusCode == 403 {
				return true, errors.New("403 Forbidden")
			} else if r != nil && r.StatusCode == 401 {
				return true, errors.New("401 Unauthorized")
			}
			d.Logger().Warnf("Error when querying metrics: %v", err)
			return false, err
		}

		if len(resp.GetSeries()) == 0 {
			d.Logger().Info("No results found")
			return false, nil
		} else if len(resp.GetSeries()) > 0 {
			d.Logger().Info("Response from `MetricsApi.QueryMetrics`")
			d.Chatwork.AddMessage("Response from `MetricsApi.QueryMetrics`\n")
			responseContent, _ := json.MarshalIndent(resp, "", "  ")
			d.Logger().Debugf("Response: %s", responseContent)
			return true, nil
		}

		return false, nil
	})
	if err != nil {
		return fmt.Errorf("error waiting for query metrics results: %w", err)
	}

	return nil
}
