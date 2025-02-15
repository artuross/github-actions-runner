package ghrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type Repository struct {
	baseUrl    string
	httpClient *http.Client
	tracer     trace.Tracer
}

func New(traceProvider trace.TracerProvider, httpClient *http.Client, baseUrl string) *Repository {
	return &Repository{
		baseUrl:    strings.TrimSuffix(baseUrl, "/"),
		httpClient: httpClient,
		tracer:     traceProvider.Tracer("github.com/artuross/github-actions-runner/internal/repository/ghrunner"),
	}
}

func (r *Repository) Register(ctx context.Context, path string, token string) (*RunnerRegistrationResponse200, error) {
	ctx, span := r.tracer.Start(ctx, "Register")
	defer span.End()

	requestBody := runnerRegistrationRequest{
		Url:         fmt.Sprintf("https://github.com/%s", path),
		RunnerEvent: "register",
	}

	requestBodyData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("ghrunner.Register marshal request body: %w", err)
	}

	url := fmt.Sprintf("%s/actions/runner-registration", r.baseUrl)
	request, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(requestBodyData))
	if err != nil {
		return nil, fmt.Errorf("ghrunner.Register create request: %w", err)
	}

	request.Header.Set("Authorization", fmt.Sprintf("RemoteAuth %s", token))

	response, err := r.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("ghrunner.Register do request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ghrunner.Register unexpected status code: %d", response.StatusCode)
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("ghrunner.Register read response body: %w", err)
	}

	var responseBodyData RunnerRegistrationResponse200
	if err := json.Unmarshal(responseBody, &responseBodyData); err != nil {
		return nil, fmt.Errorf("ghrunner.Register unmarshal response body: %w", err)
	}

	return &responseBodyData, nil
}

type runnerRegistrationRequest struct {
	Url         string `json:"url"`
	RunnerEvent string `json:"runner_event"`
}

type RunnerRegistrationResponse200 struct {
	Token       string `json:"token"`
	TokenSchema string `json:"token_schema"`
	Url         string `json:"url"`
}
