package defaults

import (
	"net/http"

	"go.opentelemetry.io/otel/trace/noop"
)

var (
	HTTPClient     = http.DefaultClient
	TracerProvider = noop.NewTracerProvider()
)
