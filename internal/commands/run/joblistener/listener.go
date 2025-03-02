package joblistener

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/artuross/github-actions-runner/internal/commands/run/manager"
	"github.com/artuross/github-actions-runner/internal/defaults"
	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/artuross/github-actions-runner/internal/repository/ghbroker"
	"github.com/artuross/github-actions-runner/internal/runnerconfig"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TODO: may want to do it via debug.ReadBuildInfo
	tracerName = "github.com/artuross/github-actions-runner/internal/commands/run/joblistener"
)

type Listener struct {
	actionsClient *ghactions.Repository
	brokerClient  *ghbroker.Repository
	config        *runnerconfig.Config
	tracer        trace.Tracer
}

func New(
	actionsClient *ghactions.Repository,
	brokerClient *ghbroker.Repository,
	config *runnerconfig.Config,
	options ...func(*Listener),
) *Listener {
	listener := Listener{
		actionsClient: actionsClient,
		brokerClient:  brokerClient,
		tracer:        defaults.TracerProvider.Tracer(tracerName),
		config:        config,
	}

	for _, apply := range options {
		apply(&listener)
	}

	return &listener
}

func (l *Listener) Run(ctx context.Context, getState func() manager.State, startJob func(jobRequest ghapi.MessageRunnerJobRequest) error) error {
	ctx, span := l.tracer.Start(ctx, "run")
	defer span.End()

	logger := zerolog.Ctx(ctx)

	var sessionID string
	defer func() {
		if sessionID == "" {
			return
		}

		ctx := context.WithoutCancel(ctx)

		logger.Debug().Str("sessionID", sessionID).Msg("deleting session")

		if err := l.actionsClient.DeleteSession(ctx, l.config.RunnerGroupID, sessionID); err != nil {
			logger.Error().Err(err).Msg("delete session")
		}
	}()

	sessionID, err := l.createSession(ctx, l.config.RunnerGroupID, l.config.RunnerID, l.config.RunnerName)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	messageClient := NewTransparentBrokerMigrationClient(l.actionsClient, l.brokerClient, 10*time.Minute)

	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)

		default:
		}

		logger.Debug().
			Int64("runnerID", l.config.RunnerID).
			Msg("fetching pool message")

		message, err := messageClient.GetPoolMessage(ctx, sessionID, l.config.RunnerGroupID, ghapi.RunnerStatusOnline)
		if errors.Is(err, ghbroker.ErrorEmptyBody) {
			logger.Info().Msg("no message")
			continue
		}

		if err != nil {
			logger.Error().Err(err).Msg("read pool message")
			continue
		}

		logger.Info().Int64("message_id", message.MessageID).Msg("received BrokerMessage")

		runnerJobRequest, ok := message.Message.(ghapi.MessageRunnerJobRequest)
		if !ok {
			logger.Error().Msg("unknown message type")
			continue
		}

		// starts job - this function returns immediately
		// and job continues in the background
		if err := startJob(runnerJobRequest); err != nil {
			logger.Error().Err(err).Msg("failed to start job")
			continue
		}

		// delete message
		if err := l.actionsClient.DeletePoolMessage(ctx, sessionID, l.config.RunnerGroupID, message.MessageID); err != nil {
			logger.Error().Err(err).Msg("delete pool message")
			continue
		}
	}
}

// createSession creates a new session for the runner. If the session creation fails, it will be silently retried.
// Returns when the context is cancelled.
func (l *Listener) createSession(ctx context.Context, poolID int64, runnerID int64, runnerName string) (string, error) {
	logger := zerolog.Ctx(ctx)

	ticker := time.NewTimer(0)
	tickerConsumed := false
	defer func() {
		if !tickerConsumed && !ticker.Stop() {
			<-ticker.C
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return "", context.Cause(ctx)

		case <-ticker.C:
			tickerConsumed = true
		}

		logger.Debug().Int64("runner_id", runnerID).Msg("creating session")

		session, err := l.actionsClient.CreateSession(ctx, poolID, runnerID, runnerName)
		// TODO: handle context exit
		if err != nil {
			logger.Error().Err(err).Msg("create session")

			logger.Warn().Msg("retrying in 10 seconds")
			ticker.Reset(10 * time.Second)
			tickerConsumed = false

			// TODO: handle alternative session issues
			continue
		}

		return session.SessionID, nil
	}
}

// TransparentBrokerMigrationClient makes requests to the GitHub Actions API for messages. When a message BrokerMigration is received,
// it will redo the request with the broker URL. When cacheDuration > 0, the broker will cache the broker URL
// for the specified duration avoiding the first request.
type TransparentBrokerMigrationClient struct {
	actionsClient *ghactions.Repository
	brokerClient  *ghbroker.Repository
	cacheDuration time.Duration
	cachedURL     string
	cachedUntil   *time.Time
}

func NewTransparentBrokerMigrationClient(actionsClient *ghactions.Repository, brokerClient *ghbroker.Repository, cacheDuration time.Duration) *TransparentBrokerMigrationClient {
	return &TransparentBrokerMigrationClient{
		actionsClient: actionsClient,
		brokerClient:  brokerClient,
		cacheDuration: cacheDuration,
	}
}

func (b *TransparentBrokerMigrationClient) GetPoolMessage(ctx context.Context, sessionID string, runnerID int64, status ghapi.RunnerStatus) (*ghapi.BrokerMessage, error) {
	brokerGetPoolMessage := func(url string) func(ctx context.Context, sessionID string, runnerID int64, status ghapi.RunnerStatus) (ghapi.Message, error) {
		return func(ctx context.Context, sessionID string, runnerID int64, status ghapi.RunnerStatus) (ghapi.Message, error) {
			return b.brokerClient.GetPoolMessage(ctx, url, sessionID, runnerID, status)
		}
	}

	getPoolMessage := b.actionsClient.GetPoolMessage
	usingCachedURL := false

	// reset cached URL if it's expired
	if b.cachedUntil != nil && b.cachedUntil.Before(time.Now()) {
		b.cachedURL = ""
		b.cachedUntil = nil
	}

	// if has cached url, use it first
	if b.cachedURL != "" {
		getPoolMessage = brokerGetPoolMessage(b.cachedURL)
		usingCachedURL = true
	}

	maxTries := 2
	for i := 0; i < maxTries; i++ {
		message, err := getPoolMessage(ctx, sessionID, runnerID, status)
		if errors.Is(err, ghbroker.ErrorEmptyBody) {
			return nil, ghbroker.ErrorEmptyBody
		}
		if err != nil {
			// if error when using cached url, retry with actions client
			if usingCachedURL {
				maxTries++
				getPoolMessage = b.actionsClient.GetPoolMessage
				usingCachedURL = false

				continue
			}

			return nil, fmt.Errorf("get pool message: %w", err)
		}

		switch msg := message.(type) {
		case ghapi.BrokerMigration:
			// if message returned by broker, return error - this is unexpected
			if b.cachedURL != "" {
				return nil, errors.New("unexpected BrokerMigration message")
			}

			// save broker server URL
			if b.cacheDuration > 0 {
				cachedUntil := time.Now().Add(b.cacheDuration)

				b.cachedURL = msg.BaseURL
				b.cachedUntil = &cachedUntil
			}

			getPoolMessage = brokerGetPoolMessage(msg.BaseURL)

			continue

		case ghapi.BrokerMessage:
			return &msg, nil
		}

		return nil, fmt.Errorf("unexpected message type")
	}

	return nil, fmt.Errorf("max tries exceeded")
}

func WithTracerProvider(tp trace.TracerProvider) func(*Listener) {
	return func(r *Listener) {
		r.tracer = tp.Tracer(tracerName)
	}
}
