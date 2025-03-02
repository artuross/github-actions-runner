package jobworker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/artuross/github-actions-runner/internal/commands/run/jobcontroller"
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
	tracerName = "github.com/artuross/github-actions-runner/internal/commands/run/jobworker"
)

type JobControllerFactory func(ghapi.PipelineAgentJobRequest) (*jobcontroller.JobController, error)

type Worker struct {
	actionsClient        *ghactions.Repository
	brokerClient         *ghbroker.Repository
	tracer               trace.Tracer
	config               *runnerconfig.Config
	jobControllerFactory JobControllerFactory
}

func New(
	actionsClient *ghactions.Repository,
	brokerClient *ghbroker.Repository,
	config *runnerconfig.Config,
	jobControllerFactory JobControllerFactory,
	options ...func(*Worker),
) *Worker {
	worker := Worker{
		actionsClient:        actionsClient,
		brokerClient:         brokerClient,
		tracer:               defaults.TracerProvider.Tracer(tracerName),
		config:               config,
		jobControllerFactory: jobControllerFactory,
	}

	for _, apply := range options {
		apply(&worker)
	}

	return &worker
}

func (w *Worker) Run(ctx context.Context, runnerJobRequest ghapi.MessageRunnerJobRequest) error {
	ctx, span := w.tracer.Start(ctx, "run worker")
	defer span.End()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobRequest, err := w.actionsClient.GetRunnerMessageByID(ctx, runnerJobRequest.RunnerRequestID)
	if err != nil {
		return fmt.Errorf("get runner job request message: %w", err)
	}

	jobDetails, ok := jobRequest.(ghapi.PipelineAgentJobRequest)
	if !ok {
		return fmt.Errorf("unknown message type")
	}

	// initial call to get next iteration time
	acquireJob, err := w.actionsClient.AcquireJob(ctx, w.config.RunnerGroupID, jobDetails.RequestID)
	if !ok {
		return fmt.Errorf("acquire job: %w", err)
	}

	logger := zerolog.Ctx(ctx).With().Int64("request_id", jobDetails.RequestID).Logger()

	// for keeping track of goroutines
	wg := sync.WaitGroup{}
	defer wg.Wait()

	// "lock" the job on interval
	wg.Add(1)
	go func(lockedUntil time.Time) {
		defer wg.Done()

		const lockSafetyMargin = 10 * time.Second

		nextIteration := func(releaseTime time.Time) time.Duration {
			return time.Until(releaseTime) - lockSafetyMargin
		}

		timer := time.NewTimer(nextIteration(lockedUntil))
		defer func() {
			// drain channel
			if !timer.Stop() {
				<-timer.C
			}
		}()

		for {
			select {
			case <-ctx.Done():
				logger.Info().Msg("context canceled")
				return

			case <-timer.C:
				acquireJob, err := w.actionsClient.AcquireJob(ctx, w.config.RunnerGroupID, jobDetails.RequestID)
				if !ok {
					logger.Error().Err(err).Msg("acquire job")

					// sleep for a second to reduce API calls during errors
					time.Sleep(time.Second)

					continue
				}

				timer.Reset(nextIteration(acquireJob.LockedUntil))
			}
		}
	}(acquireJob.LockedUntil)

	jobController, err := w.jobControllerFactory(jobDetails)
	if err != nil {
		logger.Error().Err(err).Msg("create job controller")
		return err
	}

	if err := jobController.Run(ctx, &jobDetails); err != nil {
		logger.Error().Err(err).Msg("run job controller")
		return err
	}

	// cancel to release the job lock
	cancel()

	return nil
}

func WithTracerProvider(tp trace.TracerProvider) func(*Worker) {
	return func(r *Worker) {
		r.tracer = tp.Tracer(tracerName)
	}
}
