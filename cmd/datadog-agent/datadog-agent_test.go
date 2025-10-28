package datadogagent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/chatwork/kibertas/cmd"
	"github.com/chatwork/kibertas/util/notify"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestDatadogAgentNew(t *testing.T) {
	t.Parallel()
	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}
	chatwork := &notify.Chatwork{}
	checker := cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 3*time.Minute)
	datadogAgent, err := NewDatadogAgent(checker)

	require.EqualError(t, err, "DD_API_KEY or DD_APP_KEY is empty")
	require.Nil(t, datadogAgent)

	apiKey, appKey := os.Getenv("DD_API_KEY"), os.Getenv("DD_APP_KEY")
	t.Cleanup(func() {
		if err := os.Setenv("DD_API_KEY", apiKey); err != nil {
			t.Errorf("Failed to restore DD_API_KEY: %s", err)
		}
		if err := os.Setenv("DD_APP_KEY", appKey); err != nil {
			t.Errorf("Failed to restore DD_APP_KEY: %s", err)
		}
	})
	mustSetenv(t, "DD_API_KEY", "dummyAPIKey")
	mustSetenv(t, "DD_APP_KEY", "dummyAppKey")

	datadogAgent, err = NewDatadogAgent(checker)
	require.NoError(t, err)
	require.NotNil(t, datadogAgent)
}

func TestDatadogAgentCheck_200(t *testing.T) {
	t.Parallel()

	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}

	chatwork := &notify.Chatwork{ApiToken: "token", RoomId: "test", Logger: logger}

	// Error due to emptyu API metrics query response and 1ms timeout
	datadogAgent, err := NewDatadogAgentWithClient(
		// Check timeout to 100ms to speed up the test
		cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 1*time.Millisecond),
		// Fake client returning empty response
		&fakeDatadogClient{},
	)
	require.NoError(t, err)
	// Initial delay to 1ms to speed up the test
	datadogAgent.WaitTime = 1 * time.Millisecond
	require.Error(t, datadogAgent.Check())

	// Successful metrics query
	okClient := &fakeDatadogClient{
		httpErr:      nil,
		httpResponse: &http.Response{StatusCode: 200},
		ddSeries: []datadogV1.MetricsQueryMetadata{
			{
				Scope: datadog.PtrString("*"),
			},
		},
	}
	datadogAgent, err = NewDatadogAgentWithClient(cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 3*time.Minute), okClient)
	require.NoError(t, err)
	datadogAgent.MetricsQuery = "avg:kubernetes.cpu.user.total{*}"
	datadogAgent.WaitTime = 1 * time.Millisecond
	require.NoError(t, datadogAgent.Check())
}

func TestDatadogAgentCheck_403(t *testing.T) {
	t.Parallel()

	logger := func() *logrus.Entry {
		return logrus.NewEntry(logrus.New())
	}

	chatwork := &notify.Chatwork{ApiToken: "token", RoomId: "test", Logger: logger}

	// 403 Forbidden error
	forbiddenClient := &fakeDatadogClient{
		httpErr:      errors.New("403 Forbidden"),
		httpResponse: &http.Response{StatusCode: 403},
		ddSeries: []datadogV1.MetricsQueryMetadata{
			{
				Scope: datadog.PtrString("*"),
			},
		},
	}
	datadogAgent, err := NewDatadogAgentWithClient(cmd.NewChecker(context.Background(), false, logger, chatwork, "test", 3*time.Minute), forbiddenClient)
	require.NoError(t, err)
	datadogAgent.MetricsQuery = "avg:kubernetes.cpu.user.total{*}"
	datadogAgent.WaitTime = 1 * time.Second
	require.EqualError(t, datadogAgent.Check(), "error waiting for query metrics results: 403 Forbidden")
}

type fakeDatadogClient struct {
	httpResponse *http.Response
	httpErr      error
	ddSeries     []datadogV1.MetricsQueryMetadata
	ddErr        *string
}

func (m *fakeDatadogClient) QueryMetrics(ctx context.Context, from, to int64, query string) (datadogV1.MetricsQueryResponse, *http.Response, error) {
	resp := datadogV1.MetricsQueryResponse{}
	var series []datadogV1.MetricsQueryMetadata
	series = append(series, m.ddSeries...)
	resp.SetSeries(series)
	resp.Error = m.ddErr
	return resp, m.httpResponse, m.httpErr
}

func mustSetenv(t *testing.T, key, value string) {
	t.Helper()
	require.NoError(t, os.Setenv(key, value), "failed to set env %s", key)
}
