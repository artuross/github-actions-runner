package resultsreceiver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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

func (r *Repository) UpdateWorkflowSteps(ctx context.Context, changeID int, workflowRunID, workflowJobRunID string, steps []Step) error {
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

type GetStepLogSignedBlobURLRequest struct {
	WorkflowRunID    string `json:"workflow_run_backend_id"`
	WorkflowJobRunID string `json:"workflow_job_run_backend_id"`
	StepID           string `json:"step_backend_id"`
}

type GetStepLogSignedBlobURLResponse struct {
	UploadURL       string `json:"logs_url"`
	SoftSizeLimit   string `json:"soft_size_limit"`
	BlobStorageType string `json:"blob_storage_type"`
}

func (r *Repository) GetStepLogSignedBlobURL(ctx context.Context, workflowRunID, workflowJobRunID, stepID string) (*GetStepLogSignedBlobURLResponse, error) {
	ctx, span := r.tracer.Start(ctx, "UpdateWorkflowSteps")
	defer span.End()

	requestBody := GetStepLogSignedBlobURLRequest{
		WorkflowRunID:    workflowRunID,
		WorkflowJobRunID: workflowJobRunID,
		StepID:           stepID,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode JSON body: %w", err)
	}

	url := "/twirp/results.services.receiver.Receiver/GetStepLogsSignedBlobURL"
	response, err := r.doRequest(ctx, "POST", url, bytes.NewReader(encodedBody))
	if err != nil {
		return nil, fmt.Errorf("do HTTP request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var responseBodyData GetStepLogSignedBlobURLResponse
	if err := json.Unmarshal(responseBody, &responseBodyData); err != nil {
		return nil, fmt.Errorf("unmarshal response body: %w", err)
	}

	return &responseBodyData, nil
}

type CreateStepLogsMetadataRequest struct {
	WorkflowRunID    string    `json:"workflow_run_backend_id"`
	WorkflowJobRunID string    `json:"workflow_job_run_backend_id"`
	StepID           string    `json:"step_backend_id"`
	Lines            int       `json:"line_count"`
	UploadedAt       time.Time `json:"uploaded_at"`
}

func (r *Repository) CreateStepLogsMetadata(ctx context.Context, workflowRunID, workflowJobRunID, stepID string, lines int, uploadedAt time.Time) error {
	ctx, span := r.tracer.Start(ctx, "UpdateWorkflowSteps")
	defer span.End()

	requestBody := CreateStepLogsMetadataRequest{
		WorkflowRunID:    workflowRunID,
		WorkflowJobRunID: workflowJobRunID,
		StepID:           stepID,
		Lines:            lines,
		UploadedAt:       uploadedAt,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("encode JSON body: %w", err)
	}

	url := "/twirp/results.services.receiver.Receiver/CreateStepLogsMetadata"
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
