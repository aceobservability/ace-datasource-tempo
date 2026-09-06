package tempo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aceobservability/ace-datasource-tempo/tracing"
)

func TestNew_requiresHTTPClient(t *testing.T) {
	t.Parallel()

	client, err := New("http://localhost:3200", nil)
	if err == nil {
		t.Fatal("expected error for nil http client")
	}
	if client != nil {
		t.Fatal("expected nil client when http client is missing")
	}
	if !strings.Contains(err.Error(), "http client is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNew_requiresURL(t *testing.T) {
	t.Parallel()

	client, err := New("", http.DefaultClient)
	if err == nil {
		t.Fatal("expected error for empty url")
	}
	if client != nil {
		t.Fatal("expected nil client when url is missing")
	}
}

func TestBuildTempoTraceSearchParams_DefaultsToTraceQLMatchAll(t *testing.T) {
	params := buildTempoTraceSearchParams(tracing.TraceSearchRequest{Limit: 25})

	if got := params.Get("q"); got != "{}" {
		t.Fatalf("expected q to be {}, got %q", got)
	}

	if got := params.Get("query"); got != "{}" {
		t.Fatalf("expected query to be {}, got %q", got)
	}

	if got := params.Get("limit"); got != "25" {
		t.Fatalf("expected limit to be 25, got %q", got)
	}
}

func TestBuildTempoTraceSearchParams_BuildsServiceTraceQLWhenQueryEmpty(t *testing.T) {
	params := buildTempoTraceSearchParams(tracing.TraceSearchRequest{Service: `api"edge`})

	if got := params.Get("q"); got != `{ .service.name = "api\\"edge" }` {
		t.Fatalf("expected escaped service traceql query, got %q", got)
	}

	if got := params.Get("query"); got != `{ .service.name = "api\\"edge" }` {
		t.Fatalf("expected escaped service traceql query alias, got %q", got)
	}
}

func TestClient_GetTrace(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/traces/trace-123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"traceID":"trace-123","spans":[{"traceID":"trace-123","spanID":"root","operationName":"GET /","references":[],"startTime":1700000000000000,"duration":1000,"tags":[],"processID":"p1"}],"processes":{"p1":{"serviceName":"frontend"}}}]}`))
	}))
	t.Cleanup(server.Close)

	client, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	trace, err := client.GetTrace(context.Background(), "trace-123")
	if err != nil {
		t.Fatalf("GetTrace returned error: %v", err)
	}

	if trace.TraceID != "trace-123" {
		t.Fatalf("expected trace id trace-123, got %q", trace.TraceID)
	}
}

func TestQueryAndTestConnection_againstFixtureHTTP(t *testing.T) {
	t.Parallel()

	var sawReady, sawTrace, sawSearch bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ready":
			sawReady = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
		case strings.HasPrefix(r.URL.Path, "/api/traces/"):
			sawTrace = true
			_, _ = w.Write([]byte(`{"data":[{"traceID":"trace-123","spans":[{"traceID":"trace-123","spanID":"root","operationName":"GET /","references":[],"startTime":1700000000000000,"duration":1000,"tags":[],"processID":"p1"}],"processes":{"p1":{"serviceName":"frontend"}}}]}`))
		case strings.HasPrefix(r.URL.Path, "/api/search"):
			sawSearch = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"traces":[{"traceID":"trace-tempo","rootServiceName":"frontend","rootTraceName":"GET /api","startTimeUnixNano":"1700000000000000000","durationMs":12.5,"spanSet":[{},{}]}]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Query(ctx, "{}", time.Now().Add(-time.Hour), time.Now(), time.Minute, 0); err == nil {
		t.Fatal("expected Query to reject tracing datasources")
	}

	trace, err := client.GetTrace(ctx, "trace-123")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if trace.TraceID != "trace-123" {
		t.Fatalf("GetTrace id=%q", trace.TraceID)
	}
	if !sawTrace {
		t.Fatal("expected fixture to receive /api/traces/")
	}

	summaries, err := client.SearchTraces(ctx, tracing.TraceSearchRequest{Limit: 10})
	if err != nil {
		t.Fatalf("SearchTraces: %v", err)
	}
	if len(summaries) != 1 || summaries[0].TraceID != "trace-tempo" {
		t.Fatalf("SearchTraces=%#v", summaries)
	}
	if !sawSearch {
		t.Fatal("expected fixture to receive /api/search")
	}

	if err := client.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if !sawReady {
		t.Fatal("expected TestConnection to hit /ready")
	}
}

func TestTestConnection_usesReadyThenSearchFallback(t *testing.T) {
	t.Parallel()

	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/ready" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/search") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"traces":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if len(paths) < 2 || paths[0] != "/ready" {
		t.Fatalf("paths=%v, want /ready then search", paths)
	}
}
