package resultsreceiver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type workflowUpdateStepsRequest struct {
	ChangeID         int    `json:"change_order"`
	WorkflowRunID    string `json:"workflow_run_backend_id"`
	WorkflowJobRunID string `json:"workflow_job_run_backend_id"`
	Steps            []Step `json:"steps"`
}

func (r *Repository) UpdateWorkflowSteps(ctx context.Context, changeID int, workflowRunID, workflowJobRunID string, steps []Step) error {
	ctx, span := r.tracer.Start(ctx, "UpdateWorkflowSteps")
	defer span.End()

	requestBody := workflowUpdateStepsRequest{
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
