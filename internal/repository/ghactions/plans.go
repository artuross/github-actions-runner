package ghactions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type PlanEventRequest struct {
	RequestID             int64             `json:"requestId"`
	Result                string            `json:"result"`
	Outputs               map[string]string `json:"outputs"`
	ActionsStepsTelemetry []string          `json:"actionsStepsTelemetry"`
	JobTelemetry          []string          `json:"jobTelemetry"`
	Name                  string            `json:"name"`
	JobID                 string            `json:"jobId"`
}

func (r *Repository) SendEventJobCompleted(ctx context.Context, planID string, jobID string, requestID int64) error {
	ctx, span := r.tracer.Start(ctx, "SendEventJobCompleted")
	defer span.End()

	requestBody := PlanEventRequest{
		RequestID:             requestID,
		Result:                "succeeded",
		Outputs:               map[string]string{},
		ActionsStepsTelemetry: []string{},
		JobTelemetry:          []string{},
		Name:                  "JobCompleted",
		JobID:                 jobID,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("encode JSON body: %w", err)
	}

	headers := http.Header{}
	headers.Set("Accept", acceptJSON_20P1)
	headers.Set("Content-Type", contentTypeJSON_20P1)

	path := fmt.Sprintf("/00000000-0000-0000-0000-000000000000/_apis/distributedtask/hubs/Actions/plans/%s/events", planID)
	response, err := r.doRequest(ctx, "POST", path, nil, headers, bytes.NewReader(encodedBody))
	if err != nil {
		return fmt.Errorf("do HTTP request: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	return nil
}

type LogMetadata struct {
	Path          string    `json:"path"`
	LineCount     int64     `json:"lineCount"`
	CreatedOn     time.Time `json:"createdOn"`
	LastChangedOn time.Time `json:"lastChangedOn"`
	ID            int64     `json:"id"`
}

func (r *Repository) PostLogMetadata(ctx context.Context, planID string, taskID string) (*LogMetadata, error) {
	ctx, span := r.tracer.Start(ctx, "PostLogMetadata")
	defer span.End()

	requestBody := LogMetadata{
		Path: fmt.Sprintf("logs\\%s", taskID),
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode JSON body: %w", err)
	}

	headers := http.Header{}
	headers.Set("Accept", acceptJSON_51P1)
	headers.Set("Content-Type", contentTypeJSON_51P1)

	path := fmt.Sprintf("/00000000-0000-0000-0000-000000000000/_apis/distributedtask/hubs/Actions/plans/%s/logs", planID)
	response, err := r.doRequest(ctx, "POST", path, nil, headers, bytes.NewReader(encodedBody))
	if err != nil {
		return nil, fmt.Errorf("do HTTP request: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var responseBody LogMetadata
	if err := json.Unmarshal(body, &responseBody); err != nil {
		return nil, fmt.Errorf("decode JSON body: %w", err)
	}

	return &responseBody, nil
}

func (r *Repository) PostLogs(ctx context.Context, planID string, logID int64, data []byte) (*LogMetadata, error) {
	ctx, span := r.tracer.Start(ctx, "PostLogs")
	defer span.End()

	headers := http.Header{}
	headers.Set("Accept", acceptJSON_51P1)
	headers.Set("Content-Type", "application/octet-stream; api-version=5.1-preview.1")
	headers.Set("Content-Length", strconv.Itoa(len(data)))

	path := fmt.Sprintf("/00000000-0000-0000-0000-000000000000/_apis/distributedtask/hubs/Actions/plans/%s/logs/%d", planID, logID)
	response, err := r.doRequest(ctx, "POST", path, nil, headers, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("do HTTP request: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var responseBody LogMetadata
	if err := json.Unmarshal(body, &responseBody); err != nil {
		return nil, fmt.Errorf("decode JSON body: %w", err)
	}

	return &responseBody, nil
}
