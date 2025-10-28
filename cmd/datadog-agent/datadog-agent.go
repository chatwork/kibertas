package datadogagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/util"
)

type DatadogAgent struct {
	*cmd.Checker
	// MetricsQuery is the Datadog metrics query to execute on check
	MetricsQuery string
	// WaitTime is the initial wait time before querying metrics
	WaitTime time.Duration
	// DatadogMetrics is the Datadog metrics API provider that provides metrics querying
	DatadogMetrics DatadogMetrics
}

// DatadogMetrics interface abstracts the Datadog metrics API for testing
type DatadogMetrics interface {
	// QueryMetrics executes a metrics query against the Datadog API
	QueryMetrics(ctx context.Context, from, to int64, query string) (datadogV1.MetricsQueryResponse, *http.Response, error)
}

func NewDatadogAgent(checker *cmd.Checker) (*DatadogAgent, error) {
	client, err := newDatadogClientFromEnv()
	if err != nil {
		return nil, err
	}

	return NewDatadogAgentWithClient(checker, client)
}

func newDatadogClientFromEnv() (*DatadogClient, error) {
	apiKey := ""
	appKey := ""
	if v := os.Getenv("DD_API_KEY"); v != "" {
		apiKey = v
	}
	if v := os.Getenv("DD_APP_KEY"); v != "" {
		appKey = v
	}

	if apiKey == "" || appKey == "" {
		return nil, errors.New("DD_API_KEY or DD_APP_KEY is empty")
	}

	return NewDatadogClient(apiKey, appKey), nil
}

func NewDatadogAgentWithClient(checker *cmd.Checker, metrics DatadogMetrics) (*DatadogAgent, error) {
	queryMetrics := "avg:kubernetes.cpu.user.total{*}"
	if v := os.Getenv("QUERY_METRICS"); v != "" {
		queryMetrics = v
	}

	location, _ := time.LoadLocation("Asia/Tokyo")
	checker.Chatwork.AddMessage(fmt.Sprintf("Start in %s at %s\n", checker.ClusterName, time.Now().In(location).Format("2006-01-02 15:04:05")))

	return &DatadogAgent{
		Checker:        checker,
		MetricsQuery:   queryMetrics,
		WaitTime:       3 * 60 * time.Second,
		DatadogMetrics: metrics,
	}, nil
}

func (d *DatadogAgent) Check() error {
	defer d.Chatwork.Send()

	d.Chatwork.AddMessage("datadog-agent check start\n")

	if err := d.checkMetrics(); err != nil {
		d.Chatwork.AddMessage(fmt.Sprintf("checkMetrics error: %s\n", err.Error()))
		return err
	}
	d.Chatwork.AddMessage("datadog-agent check finished\n")
	return nil
}

func (d *DatadogAgent) checkMetrics() error {

	d.Logger().Infof("Querying metrics with query: %s", d.MetricsQuery)
	d.Chatwork.AddMessage(fmt.Sprintf("Querying metrics with query: %s\n", d.MetricsQuery))

	d.Logger().Info("Waiting metrics...")

	if err := util.SleepContext(d.Ctx, d.WaitTime); err != nil {
		return err
	}

	now := time.Now().Unix()
	from := now - 60*2
	err := wait.PollUntilContextTimeout(d.Ctx, 30*time.Second, d.Timeout, true, func(ctx context.Context) (bool, error) {
		resp, r, err := d.DatadogMetrics.QueryMetrics(ctx, from, now, d.MetricsQuery)

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
