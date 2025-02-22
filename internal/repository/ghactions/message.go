package ghactions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/artuross/github-actions-runner/internal/meta/version"
	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/artuross/github-actions-runner/internal/repository/internal/api"
)

func (r *Repository) DeletePoolMessage(
	ctx context.Context,
	sessionID string,
	poolID int64,
	messageID int64,
) error {
	ctx, span := r.tracer.Start(ctx, "DeletePoolMessage")
	defer span.End()

	path := fmt.Sprintf("/_apis/distributedtask/pools/%d/messages/%d?sessionId=%s", poolID, messageID, sessionID)
	response, err := r.doRequest(ctx, "DELETE", path, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("do HTTP request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	return nil
}

func (r *Repository) GetPoolMessage(
	ctx context.Context,
	sessionID string,
	poolID int64,
	status ghapi.RunnerStatus,
) (ghapi.Message, error) {
	ctx, span := r.tracer.Start(ctx, "GetPoolMessage")
	defer span.End()

	os := "Linux"
	architecture := "ARM64"
	disableUpdate := false // change to true

	queryParams := url.Values{}
	queryParams.Set("sessionId", sessionID)
	queryParams.Set("status", string(status))
	queryParams.Set("runnerVersion", version.RunnerCompatibilityVersion)
	queryParams.Set("os", os)
	queryParams.Set("architecture", architecture)
	queryParams.Set("disableUpdate", fmt.Sprintf("%t", disableUpdate))

	path := fmt.Sprintf("/_apis/distributedtask/pools/%d/messages", poolID)
	response, err := r.doRequest(ctx, "GET", path, queryParams, nil, nil)
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

	var responseBody api.Wrapper
	if err := json.Unmarshal(body, &responseBody); err != nil {
		return nil, fmt.Errorf("decode JSON body: %w", err)
	}

	msg, ok := responseBody.Message.(ghapi.BrokerMigration)
	if !ok {
		return nil, errors.New("unsupported message type")
	}

	return msg, nil
}
