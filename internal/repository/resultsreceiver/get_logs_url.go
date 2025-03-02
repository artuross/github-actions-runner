package resultsreceiver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type (
	getJobLogSignedBlobURLRequest struct {
		WorkflowRunID    string `json:"workflow_run_backend_id"`
		WorkflowJobRunID string `json:"workflow_job_run_backend_id"`
	}

	getJobLogSignedBlobURLResponse struct {
		UploadURL       string `json:"logs_url"`
		SoftSizeLimit   string `json:"soft_size_limit"`
		BlobStorageType string `json:"blob_storage_type"`
	}
)

func (r *Repository) GetJobLogSignedBlobURL(ctx context.Context, workflowRunID, workflowJobRunID string) (string, error) {
	ctx, span := r.tracer.Start(ctx, "GetJobLogSignedBlobURL")
	defer span.End()

	requestBody := getJobLogSignedBlobURLRequest{
		WorkflowRunID:    workflowRunID,
		WorkflowJobRunID: workflowJobRunID,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("encode JSON body: %w", err)
	}

	url := "/twirp/results.services.receiver.Receiver/GetJobLogsSignedBlobURL"
	response, err := r.doRequest(ctx, "POST", url, bytes.NewReader(encodedBody))
	if err != nil {
		return "", fmt.Errorf("do HTTP request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	var responseBodyData getJobLogSignedBlobURLResponse
	if err := json.Unmarshal(responseBody, &responseBodyData); err != nil {
		return "", fmt.Errorf("unmarshal response body: %w", err)
	}

	return responseBodyData.UploadURL, nil
}

type (
	getStepLogSignedBlobURLRequest struct {
		WorkflowRunID    string `json:"workflow_run_backend_id"`
		WorkflowJobRunID string `json:"workflow_job_run_backend_id"`
		StepID           string `json:"step_backend_id"`
	}

	getStepLogSignedBlobURLResponse struct {
		UploadURL       string `json:"logs_url"`
		SoftSizeLimit   string `json:"soft_size_limit"`
		BlobStorageType string `json:"blob_storage_type"`
	}
)

func (r *Repository) GetStepLogSignedBlobURL(ctx context.Context, workflowRunID, workflowJobRunID, stepID string) (string, error) {
	ctx, span := r.tracer.Start(ctx, "GetStepLogSignedBlobURL")
	defer span.End()

	requestBody := getStepLogSignedBlobURLRequest{
		WorkflowRunID:    workflowRunID,
		WorkflowJobRunID: workflowJobRunID,
		StepID:           stepID,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("encode JSON body: %w", err)
	}

	url := "/twirp/results.services.receiver.Receiver/GetStepLogsSignedBlobURL"
	response, err := r.doRequest(ctx, "POST", url, bytes.NewReader(encodedBody))
	if err != nil {
		return "", fmt.Errorf("do HTTP request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	var responseBodyData getStepLogSignedBlobURLResponse
	if err := json.Unmarshal(responseBody, &responseBodyData); err != nil {
		return "", fmt.Errorf("unmarshal response body: %w", err)
	}

	return responseBodyData.UploadURL, nil
}
