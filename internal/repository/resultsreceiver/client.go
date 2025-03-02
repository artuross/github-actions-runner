package resultsreceiver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/artuross/github-actions-runner/internal/defaults"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TODO: may want to do it via debug.ReadBuildInfo
	tracerName = "github.com/artuross/github-actions-runner/internal/repository/resultsreceiver"
)

type Repository struct {
	httpClient *http.Client
	tracer     trace.Tracer
	baseURL    string
}

func New(baseURL string, options ...func(*Repository)) *Repository {
	repository := Repository{
		httpClient: defaults.HTTPClient,
		tracer:     defaults.TraceProvider.Tracer(tracerName),
		baseURL:    getBaseURL(baseURL),
	}

	for _, apply := range options {
		apply(&repository)
	}

	return &repository
}

func (r *Repository) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, r.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create HTTP request: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json; charset=utf-8")

	response, err := r.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send HTTP request: %w", err)
	}

	return response, nil
}

func WithHTTPClient(httpClient *http.Client) func(*Repository) {
	return func(r *Repository) {
		r.httpClient = httpClient
	}
}

func WithTracerProvider(tp trace.TracerProvider) func(*Repository) {
	return func(r *Repository) {
		r.tracer = tp.Tracer(tracerName)
	}
}

func getBaseURL(baseUrl string) string {
	return strings.TrimSuffix(baseUrl, "/")
}
