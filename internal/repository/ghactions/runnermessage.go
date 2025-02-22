package ghactions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/artuross/github-actions-runner/internal/repository/internal/api"
)

// TODO: parse message fields
func (r *Repository) GetRunnerMessageByID(ctx context.Context, runnerRequestID string) (ghapi.Message, error) {
	ctx, span := r.tracer.Start(ctx, "GetRunnerMessageByID")
	defer span.End()

	path := fmt.Sprintf("/_apis/distributedtask/runnermessages/%s", runnerRequestID)
	response, err := r.doRequest(ctx, "GET", path, nil, nil, nil)
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

	var wrapper api.Wrapper
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}

	msg, ok := wrapper.Message.(ghapi.PipelineAgentJobRequest)
	if !ok {
		return nil, errors.New("unsupported message type")
	}

	return msg, nil
}
