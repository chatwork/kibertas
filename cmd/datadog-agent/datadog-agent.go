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

	queryMetrics = "avg:kubernetes.cpu.user.total{*}"
	if v := os.Getenv("QUERY_METRICS"); v != "" {
		queryMetrics = v
	}

	location, _ := time.LoadLocation("Asia/Tokyo")
	checker.Chatwork.AddMessage(fmt.Sprintf("Start in %s at %s\n", checker.ClusterName, time.Now().In(location).Format("2006-01-02 15:04:05")))

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
	// NewDefaultContext loads DD_API_KEY and DD_APP_KEY
	// into apiKeyAuth and appKeyAuth respectively,
	// and doing:
	//
	// datadog.NewDefaultContext(context.WithValue(
	// 	ctx,
	// 	datadog.ContextAPIKeys,
	// 	keys,
	// ))
	//
	// does not clear the apiKeyAuth and appKeyAuth values
	// set by the NewDefaultContext function.
	//
	// So the choice is to use NewDefaultContext for loading
	// environment variables or use context.WithValue for
	// setting the API keys from DatadogAgent struct,
	// but not both.
	//
	// As we already check for empty API keys before calling checkMetrics,
	// let's use NewDefaultContext to load the environment variables.
	ddctx := datadog.NewDefaultContext(d.Ctx)

	// In case we had non-empty d.ApiKey and d.AppKey set along with
	// the environment variables, the user intent is to use the
	// API keys from the DatadogAgent struct.
	keys := map[string]datadog.APIKey{}
	if d.ApiKey != "" {
		keys["apiKeyAuth"] = datadog.APIKey{Key: d.ApiKey}
	}
	if d.AppKey != "" {
		keys["appKeyAuth"] = datadog.APIKey{Key: d.AppKey}
	}
	if len(keys) > 0 {
		ddctx = context.WithValue(
			ddctx,
			datadog.ContextAPIKeys,
			keys,
		)
	}

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

		if resp.Error != nil {
			if r.StatusCode == 200 {
				return true, fmt.Errorf("HTTP status was 200 OK but got Datadog API error: %s", *resp.Error)
			}
			d.Logger().Warnf("Datadog API error: %s", *resp.Error)
			return false, nil
		}

		if len(resp.GetSeries()) == 0 {
			d.Logger().Infof("No results found: from=%d to=%d", from, now)
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
