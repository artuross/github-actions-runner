package ghrest

import (
	"context"
	"fmt"
	"time"
)

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
