package resultsreceiver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type createJobLogsMetadataRequest struct {
	WorkflowRunID    string    `json:"workflow_run_backend_id"`
	WorkflowJobRunID string    `json:"workflow_job_run_backend_id"`
	Lines            int       `json:"line_count"`
	UploadedAt       time.Time `json:"uploaded_at"`
}

func (r *Repository) CreateJobLogsMetadata(ctx context.Context, workflowRunID, workflowJobRunID string, lines int, uploadedAt time.Time) error {
	ctx, span := r.tracer.Start(ctx, "CreateJobLogsMetadata")
	defer span.End()

	requestBody := createJobLogsMetadataRequest{
		WorkflowRunID:    workflowRunID,
		WorkflowJobRunID: workflowJobRunID,
		Lines:            lines,
		UploadedAt:       uploadedAt,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("encode JSON body: %w", err)
	}

	url := "/twirp/results.services.receiver.Receiver/CreateJobLogsMetadata"
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

type createStepLogsMetadataRequest struct {
	WorkflowRunID    string    `json:"workflow_run_backend_id"`
	WorkflowJobRunID string    `json:"workflow_job_run_backend_id"`
	StepID           string    `json:"step_backend_id"`
	Lines            int       `json:"line_count"`
	UploadedAt       time.Time `json:"uploaded_at"`
}

func (r *Repository) CreateStepLogsMetadata(ctx context.Context, workflowRunID, workflowJobRunID, stepID string, lines int, uploadedAt time.Time) error {
	ctx, span := r.tracer.Start(ctx, "CreateStepLogsMetadata")
	defer span.End()

	requestBody := createStepLogsMetadataRequest{
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
