package ghactions

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/artuross/github-actions-runner/internal/defaults"
	"go.opentelemetry.io/otel/trace"
)

const (
	acceptJSON_20P1 = "application/json; api-version=2.0-preview.1"
	acceptJSON_51   = "application/json; api-version=5.1"
	acceptJSON_51P1 = "application/json; api-version=5.1-preview.1"
	acceptJSON_60P2 = "application/json; api-version=6.0-preview.2"

	contentTypeJSON      = "application/json"
	contentTypeJSON_20P1 = "application/json; charset=utf-8; api-version=2.0-preview.1"
	contentTypeJSON_51P1 = "application/json; charset=utf-8; api-version=5.1-preview.1"
	contentTypeJSON_60P2 = "application/json; charset=utf-8; api-version=6.0-preview.2"
)

const (
	// TODO: may want to do it via debug.ReadBuildInfo
	tracerName = "github.com/artuross/github-actions-runner/internal/repository/ghactions"
)

type Repository struct {
	httpClient *http.Client
	tracer     trace.Tracer
	baseURL    string
}

func New(baseURL string, options ...func(*Repository)) *Repository {
	repository := Repository{
		httpClient: defaults.HTTPClient,
		tracer:     defaults.TracerProvider.Tracer(tracerName),
		baseURL:    getBaseURL(baseURL),
	}

	for _, apply := range options {
		apply(&repository)
	}

	return &repository
}

func (r *Repository) WithBaseURL(baseURL string) *Repository {
	return &Repository{
		httpClient: r.httpClient,
		tracer:     r.tracer,
		baseURL:    getBaseURL(baseURL),
	}
}

func (r *Repository) doRequest(
	ctx context.Context,
	method,
	path string,
	queryParams url.Values,
	headers http.Header,
	body io.Reader,
) (*http.Response, error) {
	fullURL := r.baseURL + path
	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	request, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("create HTTP request: %w", err)
	}

	request.Header.Set("Accept", acceptJSON_51)
	request.Header.Set("Content-Type", contentTypeJSON)

	for key, values := range headers {
		request.Header.Del(key)

		for _, value := range values {
			request.Header.Add(key, value)
		}
	}

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
