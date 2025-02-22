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

// TODO: parse message fields
func (r *Repository) AcquireJob(ctx context.Context, poolID int64, requestID int64) (*AcquireJobResponseBody, error) {
	ctx, span := r.tracer.Start(ctx, "AcquireJob")
	defer span.End()

	requestBody := acquireJobRequestBody{
		RequestID: requestID,
	}

	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("encode JSON body: %w", err)
	}

	path := fmt.Sprintf("/_apis/distributedtask/pools/%d/jobrequests/%d?lockToken=00000000-0000-0000-0000-000000000000", poolID, requestID)
	response, err := r.doRequest(ctx, "PATCH", path, nil, nil, bytes.NewReader(encodedBody))
	if err != nil {
		return nil, fmt.Errorf("do HTTP request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var responseBody internalAcquireJobResponseBody
	if err := json.Unmarshal(body, &responseBody); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}

	lockedUntil, err := time.Parse(time.RFC3339Nano, responseBody.LockedUntil)
	if err != nil {
		return nil, fmt.Errorf("parse lock time: %w", err)
	}

	data := AcquireJobResponseBody{
		LockedUntil: lockedUntil,
	}

	return &data, nil
}

type acquireJobRequestBody struct {
	RequestID int64               `json:"requestId"`
	Data      map[string]struct{} `json:"data"`
}

type AcquireJobResponseBody struct {
	LockedUntil time.Time
}

type internalAcquireJobResponseBody struct {
	LockedUntil string `json:"lockedUntil"`
}
