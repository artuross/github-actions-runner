package resultsreceiver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/artuross/github-actions-runner/internal/defaults"
	"go.opentelemetry.io/otel/trace"
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
		tracer:     defaults.TraceProvider.Tracer(tracerName),
		baseURL:    getBaseURL(baseURL),
	}

	for _, apply := range options {
		apply(&repository)
	}

	return &repository
}

type WorkflowUpdateStepsRequest struct {
	ChangeID         int    `json:"change_order"`
	WorkflowRunID    string `json:"workflow_run_backend_id"`
	WorkflowJobRunID string `json:"workflow_job_run_backend_id"`
	Steps            []Step `json:"steps"`
}

func (r *Repository) UpdateWorkflowSteps(ctx context.Context, changeID int, workflowRunID string, workflowJobRunID string, steps []Step) error {
	ctx, span := r.tracer.Start(ctx, "UpdateWorkflowSteps")
	defer span.End()

	requestBody := WorkflowUpdateStepsRequest{
		ChangeID:         changeID,
		WorkflowRunID:    workflowRunID,
		WorkflowJobRunID: workflowJobRunID,
		Steps:            steps,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("encode JSON body: %w", err)
	}

	url := "/twirp/github.actions.results.api.v1.WorkflowStepUpdateService/WorkflowStepsUpdate"
	response, err := r.doRequest(ctx, "POST", url, bytes.NewReader(encodedBody))
	if err != nil {
		return fmt.Errorf("do HTTP request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	return nil
}

func (r *Repository) doRequest(
	ctx context.Context,
	method,
	path string,
	body io.Reader,
) (*http.Response, error) {
	fullURL := r.baseURL + path
	request, err := http.NewRequestWithContext(ctx, method, fullURL, body)
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
