package ghrest

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v69/github"
	"go.opentelemetry.io/otel/trace"
)

type Repository struct {
	client *github.Client
	tracer trace.Tracer
}

func New(traceProvider trace.TracerProvider, client *github.Client) *Repository {
	return &Repository{
		client: client,
		tracer: traceProvider.Tracer("github.com/artuross/github-actions-runner/internal/repository/ghrest"),
	}
}

type RegistrationToken struct {
	Token     string
	ExpiresAt time.Time // not used
}

func (r *Repository) CreateOrganizationRunnerRegistrationToken(ctx context.Context, org string) (*RegistrationToken, error) {
	ctx, span := r.tracer.Start(ctx, "CreateOrganizationRunnerRegistrationToken")
	defer span.End()

	tokenResponse, _, err := r.client.Actions.CreateOrganizationRegistrationToken(ctx, org)
	if err != nil {
		return nil, fmt.Errorf("ghrest.CreateOrganizationRunnerRegistrationToken create token: %w", err)
	}

	token := RegistrationToken{
		Token:     *tokenResponse.Token,
		ExpiresAt: tokenResponse.ExpiresAt.Time,
	}

	return &token, nil
}
