package tempo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aceobservability/ace-datasource-tempo/tracing"
	"github.com/aceobservability/ace/backend/pkg/datasource"
)

// Type is the RegisterDatasource key Ace uses for this module.
const Type = "tempo"

// Client implements the Ace Tempo query and tracing datasource.
type Client struct {
	url        string
	httpClient *http.Client
}

// New constructs a Tempo datasource client.
// httpClient is required so Ace can inject DatasourceClient (dial/redirect policy + auth).
func New(tempoURL string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(tempoURL) == "" {
		return nil, fmt.Errorf("datasource url is required")
	}
	if httpClient == nil {
		return nil, fmt.Errorf("http client is required")
	}

	return &Client{
		url:        tempoURL,
		httpClient: httpClient,
	}, nil
}

// HTTPClient returns the injected HTTP client. Ace SSRF tests inspect policy wiring.
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

func (c *Client) Query(ctx context.Context, query string, start, end time.Time, step time.Duration, limit int) (*datasource.QueryResult, error) {
	_ = ctx
	_ = query
	_ = start
	_ = end
	_ = step
	_ = limit

	return nil, fmt.Errorf("tempo datasource does not support /query; use tracing endpoints")
}

func (c *Client) GetTrace(ctx context.Context, traceID string) (*tracing.Trace, error) {
	trimmedTraceID := strings.TrimSpace(traceID)
	if trimmedTraceID == "" {
		return nil, fmt.Errorf("trace id is required")
	}

	payload, err := tracing.DoTracingRequest(ctx, c.httpClient, c.url, http.MethodGet, "/api/traces/"+url.PathEscape(trimmedTraceID), nil)
	if err != nil {
		return nil, err
	}

	return tracing.ParseTrace(payload)
}

func (c *Client) SearchTraces(ctx context.Context, req tracing.TraceSearchRequest) ([]tracing.TraceSummary, error) {
	params := buildTempoTraceSearchParams(req)
	payload, err := tracing.DoTracingRequest(ctx, c.httpClient, c.url, http.MethodGet, "/api/search?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	traces, err := tracing.ParseTraceSearchResponse(payload)
	if err != nil {
		return nil, err
	}

	return tracing.NormalizeTraceSearchResults(traces, req.Limit), nil
}

func buildTempoTraceSearchParams(req tracing.TraceSearchRequest) url.Values {
	params := tracing.BuildTraceSearchParams(req)

	if strings.TrimSpace(req.Query) != "" {
		return params
	}

	traceQLFilters := make([]string, 0, 1)
	if service := strings.TrimSpace(req.Service); service != "" {
		traceQLFilters = append(traceQLFilters, `.service.name = "`+escapeTraceQLString(service)+`"`)
	}

	query := "{}"
	if len(traceQLFilters) > 0 {
		query = "{ " + strings.Join(traceQLFilters, " && ") + " }"
	}

	params.Set("q", query)
	params.Set("query", query)

	return params
}

func escapeTraceQLString(value string) string {
	replacer := strings.NewReplacer(`\\`, `\\\\`, `"`, `\\"`)
	return replacer.Replace(value)
}

func (c *Client) Services(ctx context.Context) ([]string, error) {
	endpoints := []string{
		"/api/search/tags/service.name/values",
		"/api/services",
	}

	var lastErr error
	for _, endpoint := range endpoints {
		payload, err := tracing.DoTracingRequest(ctx, c.httpClient, c.url, http.MethodGet, endpoint, nil)
		if err != nil {
			lastErr = err
			continue
		}

		services, err := tracing.ParseStringSlicePayload(payload)
		if err != nil {
			lastErr = err
			continue
		}

		return services, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("failed to fetch trace services")
}

var (
	_ datasource.Client     = (*Client)(nil)
	_ tracing.TracingClient = (*Client)(nil)
)
