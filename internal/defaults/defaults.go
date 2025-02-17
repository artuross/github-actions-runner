package defaults

import (
	"net/http"

	"go.opentelemetry.io/otel/trace/noop"
)

var (
	HTTPClient    = http.DefaultClient
	TraceProvider = noop.NewTracerProvider()
)
