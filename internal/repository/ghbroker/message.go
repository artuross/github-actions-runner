package ghbroker

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

var ErrorEmptyBody = errors.New("empty body")

func (r *Repository) GetPoolMessage(
	ctx context.Context,
	baseURL string,
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

	path := "/message"
	response, err := r.doRequest(ctx, baseURL, "GET", path, queryParams, nil)
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

	if string(body) == "" {
		return nil, ErrorEmptyBody
	}

	var responseBody api.Wrapper
	if err := json.Unmarshal(body, &responseBody); err != nil {
		return nil, fmt.Errorf("unmarshal wrapper: %w", err)
	}

	msg, ok := responseBody.Message.(ghapi.BrokerMessage)
	if !ok {
		return nil, fmt.Errorf("unexpected message type")
	}

	return msg, nil
}
