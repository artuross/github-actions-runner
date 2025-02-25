package ghactions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

func (r *Repository) PostEvents(ctx context.Context, planID string, jobID string, requestID int64) error {
	ctx, span := r.tracer.Start(ctx, "PostEvents")
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
