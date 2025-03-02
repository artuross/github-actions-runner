package blobstorage

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/artuross/github-actions-runner/internal/defaults"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TODO: may want to do it via debug.ReadBuildInfo
	tracerName = "github.com/artuross/github-actions-runner/internal/repository/blobstorage"
)

type Repository struct {
	httpClient *http.Client
	tracer     trace.Tracer
}

func New(options ...func(*Repository)) *Repository {
	repository := Repository{
		httpClient: defaults.HTTPClient,
		tracer:     defaults.TracerProvider.Tracer(tracerName),
	}

	for _, apply := range options {
		apply(&repository)
	}

	return &repository
}

func (r *Repository) UploadFile(ctx context.Context, url string, data []byte) error {
	ctx, span := r.tracer.Start(ctx, "UploadFile")
	defer span.End()

	headers := http.Header{}
	headers.Set("x-ms-blob-type", "BlockBlob")
	headers.Set("x-ms-blob-content-type", "text/plain") // TODO: from params?

	response, err := r.doRequest(ctx, http.MethodPut, url, headers, data)
	if err != nil {
		return fmt.Errorf("do HTTP request: %w", err)
	}

	if response.StatusCode != 201 {
		return fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	return nil
}

func (r *Repository) doRequest(
	ctx context.Context,
	method,
	url string,
	headers http.Header,
	body []byte,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create HTTP request: %w", err)
	}

	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("Content-Length", strconv.Itoa(len(body)))

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
