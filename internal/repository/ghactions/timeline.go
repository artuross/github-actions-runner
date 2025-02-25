package ghactions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Log struct {
	ID       int    `json:"id"`
	Location string `json:"location"`
}

type TimelineRecord struct {
	ID              string     `json:"id"`
	ParentID        *string    `json:"parentId"`
	Type            string     `json:"type"`
	Name            string     `json:"name"`
	StartTime       *time.Time `json:"startTime"`
	FinishTime      *time.Time `json:"finishTime"`
	PercentComplete int        `json:"percentComplete"`
	State           string     `json:"state"`
	Result          *string    `json:"result"`
	ChangeID        int        `json:"changeId"`
	Order           int        `json:"order,omitempty"`
	RefName         string     `json:"refName"`
	Log             *Log       `json:"log"`
	Location        *string    `json:"location"`
	Identifier      *string    `json:"identifier"`
}

type TimelineRecordDiff struct {
	ID               string            `json:"id"`
	ParentID         *string           `json:"parentId"`
	Type             *string           `json:"type"`
	Name             *string           `json:"name"`
	StartTime        *time.Time        `json:"startTime"`
	FinishTime       *time.Time        `json:"finishTime"`
	CurrentOperation *string           `json:"currentOperation"` // ??
	PercentComplete  int               `json:"percentComplete"`  // ??
	State            *string           `json:"state"`
	Result           *string           `json:"result"`          // ??
	ResultCode       *string           `json:"resultCode"`      // ??
	ChangeID         *int              `json:"changeId"`        // ??
	LastModified     time.Time         `json:"lastModified"`    // ??
	WorkerName       *string           `json:"workerName"`      // ??
	Order            *int              `json:"order,omitempty"` // ??
	RefName          *string           `json:"refName"`
	Log              *json.RawMessage  `json:"log"`                     // ??
	Details          *string           `json:"details"`                 // ??
	ErrorCount       int               `json:"errorCount"`              // ??
	WarningCount     int               `json:"warningCount"`            // ??
	NoticeCount      int               `json:"noticeCount"`             // ??
	Issues           []string          `json:"issues"`                  // ??
	Variables        map[string]string `json:"variables"`               // ??
	Location         *string           `json:"location"`                // ??
	PreviousAttempts []int             `json:"previousAttempts"`        // ??
	Attempt          int               `json:"attempt"`                 // ??
	Identifier       *string           `json:"identifier"`              // ??
	AgentPlatform    *string           `json:"agentPlatform,omitempty"` // ??
}

type PatchTimelineRequest struct {
	Count int                  `json:"count"`
	Value []TimelineRecordDiff `json:"value"`
}

type PatchTimelineResponse struct {
	Count int              `json:"count"`
	Value []TimelineRecord `json:"value"`
}

func (r *Repository) PatchTimeline(ctx context.Context, planID, timelineID string, diffs []TimelineRecordDiff) ([]TimelineRecord, error) {
	ctx, span := r.tracer.Start(ctx, "PatchTimeline")
	defer span.End()

	requestBody := PatchTimelineRequest{
		Count: len(diffs),
		Value: diffs,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode JSON body: %w", err)
	}

	headers := http.Header{}
	headers.Set("Accept", acceptJSON_51P1)
	headers.Set("Content-Type", contentTypeJSON_51P1)

	path := fmt.Sprintf("/00000000-0000-0000-0000-000000000000/_apis/distributedtask/hubs/Actions/plans/%s/timelines/%s/records", planID, timelineID)
	response, err := r.doRequest(ctx, "PATCH", path, nil, headers, bytes.NewReader(encodedBody))
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

	var responseBody PatchTimelineResponse
	if err := json.Unmarshal(body, &responseBody); err != nil {
		return nil, fmt.Errorf("decode JSON body: %w", err)
	}

	return responseBody.Value, nil
}
