package datadogagent

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

// DatadogClient implements DatadogMetrics using the actual Datadog API client
type DatadogClient struct {
	api    *datadogV1.MetricsApi
	apiKey string
	appKey string
}

func NewDatadogClient(apiKey, appKey string) *DatadogClient {
	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)
	api := datadogV1.NewMetricsApi(apiClient)

	return &DatadogClient{
		api:    api,
		apiKey: apiKey,
		appKey: appKey,
	}
}

// QueryMetrics calls the Datadog API to query metrics
func (c *DatadogClient) QueryMetrics(ctx context.Context, from, to int64, query string) (datadogV1.MetricsQueryResponse, *http.Response, error) {
	// NewDefaultContext sets apiKeyAuth and appKeyAuth
	// in the context if the corresponding environment variables, DD_API_KEY and DD_APP_KEY, are set.
	ddctx := datadog.NewDefaultContext(ctx)

	// In case we had non-empty apiKey and appKey set along with
	// the environment variables, the user intent is to use the
	// API keys from the DatadogClient struct.
	keys := map[string]datadog.APIKey{}
	if c.apiKey != "" {
		keys["apiKeyAuth"] = datadog.APIKey{Key: c.apiKey}
	}
	if c.appKey != "" {
		keys["appKeyAuth"] = datadog.APIKey{Key: c.appKey}
	}
	if len(keys) > 0 {
		// We need to wrap NewDefaultContext with context.WithValue to override the apiKeyAuth and appKeyAuth set by NewDefaultContext,
		// not the vice versa, to prefer the API keys from DatadogClient struct.
		//
		// In other words, the below does not work as intended:
		//
		// datadog.NewDefaultContext(context.WithValue(
		//	ctx,
		//	datadog.ContextAPIKeys,
		//	keys,
		//))
		ddctx = context.WithValue(
			ddctx,
			datadog.ContextAPIKeys,
			keys,
		)
	}

	return c.api.QueryMetrics(ddctx, from, to, query)
}
