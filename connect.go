package tempo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aceobservability/ace-datasource-tempo/tracing"
)

func (c *Client) TestConnection(ctx context.Context) error {
	return runHTTPConnectionCheck(ctx, c.httpClient, c.url, []string{"/ready", "/api/search?limit=1", "/"})
}

func runHTTPConnectionCheck(ctx context.Context, httpClient *http.Client, baseURL string, endpoints []string) error {
	if httpClient == nil {
		return fmt.Errorf("http client is required")
	}

	var lastErr error
	for _, endpoint := range endpoints {
		targetURL, err := tracing.ResolveEndpoint(baseURL, endpoint)
		if err != nil {
			lastErr = err
			continue
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("authentication failed with status %d", resp.StatusCode)
		}

		if resp.StatusCode == http.StatusNotFound {
			lastErr = fmt.Errorf("endpoint %s not found", endpoint)
			continue
		}

		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}

		lastErr = fmt.Errorf("endpoint %s returned status %d: %s", endpoint, resp.StatusCode, message)
	}

	if lastErr != nil {
		return lastErr
	}

	return fmt.Errorf("connection test failed")
}
