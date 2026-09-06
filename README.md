# ace-datasource-tempo

Compile-time Tempo datasource module for [Ace](https://github.com/aceobservability/ace).

Ace keeps the datasource contract and registry in
`github.com/aceobservability/ace/backend/pkg/datasource`. This module implements
that `Client` plus Tempo tracing (`GetTrace`, `SearchTraces`, `Services`). Ace
registers the factory at `init` and injects its SSRF-safe HTTP client — this
module does not import Ace `internal/` packages and does not construct an
unpolicy'd client.

Shared Tempo/Jaeger parse helpers live in `./tracing` and are imported by
`ace-datasource-victoriatraces`.

## Contract

| Surface | Package |
| --- | --- |
| Query / result types | `github.com/aceobservability/ace/backend/pkg/datasource` |
| Trace types / helpers | `github.com/aceobservability/ace-datasource-tempo/tracing` |
| Registry type key | `tempo` (`Type`) |
| Factory | `New(url string, httpClient *http.Client)` |

`httpClient` is required. Ace passes `ssrf.DatasourceClient` wrapped with stored
datasource credentials.

## Tests

```
go test ./...
```

Query, parse, and connection tests speak to an `httptest` fixture. No live Tempo
is required.
