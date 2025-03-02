package run

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/artuross/github-actions-runner/internal/commandinit"
	"github.com/artuross/github-actions-runner/internal/commands/run/jobcontroller"
	"github.com/artuross/github-actions-runner/internal/commands/run/joblistener"
	"github.com/artuross/github-actions-runner/internal/commands/run/jobworker"
	"github.com/artuross/github-actions-runner/internal/commands/run/manager"
	"github.com/artuross/github-actions-runner/internal/commands/run/timeline"
	"github.com/artuross/github-actions-runner/internal/commands/run/workflowsteps"
	"github.com/artuross/github-actions-runner/internal/oauth/actions"
	"github.com/artuross/github-actions-runner/internal/repository/blobstorage"
	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/artuross/github-actions-runner/internal/repository/ghbroker"
	"github.com/artuross/github-actions-runner/internal/repository/resultsreceiver"
	"github.com/artuross/github-actions-runner/internal/runnerconfig"
	"github.com/rs/zerolog"
	cli "github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
)

var ErrCommandFailed = errors.New("command failed")

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Starts a runner.",
		Flags: []cli.Flag{
			// TODO: add flags
		},
		Action: run,
	}
}

func run(cliCtx *cli.Context) error {
	ctx := cliCtx.Context

	logger := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger().With().Str("command", "run").Logger()

	// cfg, err := config.Read(cliCtx, os.Getenv)
	// if err != nil {
	// 	return fmt.Errorf("invalid config: %w", err)
	// }

	// config.Print(cfg)

	// TODO: move to config.Read
	runnerConfig, err := runnerconfig.ReadConfigFile("./.config/runner.json")
	if err != nil {
		logger.Error().Err(err).Msg("read runner config file")
		return ErrCommandFailed
	}

	// TODO: move to config.Read
	privateKey, err := runnerconfig.ReadPrivateKeyFile("./.config/key.pem")
	if err != nil {
		logger.Error().Err(err).Msg("read private key file")
		return ErrCommandFailed
	}

	traceProvider, tpShutdown, err := commandinit.NewOpenTelemetry(ctx, "runner")
	if err != nil {
		logger.Error().Err(err).Msg("init OTEL provider")
		return ErrCommandFailed
	}
	defer tpShutdown(ctx)

	httpClient := oauth2.NewClient(
		ctx,
		actions.NewTokenSource(
			runnerConfig.AuthURL,
			runnerConfig.AuthClientID,
			privateKey,
		),
	)

	ghActionsClient := ghactions.New(
		runnerConfig.ShardURL,
		ghactions.WithHTTPClient(httpClient),
		ghactions.WithTracerProvider(traceProvider),
	)

	ghBrokerClient := ghbroker.New(
		ghbroker.WithHTTPClient(httpClient),
		ghbroker.WithTracerProvider(traceProvider),
	)

	ctx, cancel := context.WithCancelCause(ctx)
	stopChan := make(chan os.Signal, 1)

	errInterrupted := errors.New("interrupted")

	go func() {
		signal.Notify(stopChan, os.Interrupt, syscall.SIGINT)

		<-stopChan
		logger.Info().Msg("received cancel signal")

		// TODO: extract to another package
		cancel(errInterrupted)
	}()

	ctx = logger.WithContext(ctx)

	jobListener := joblistener.New(
		ghActionsClient,
		ghBrokerClient,
		runnerConfig,
		joblistener.WithTracerProvider(traceProvider),
	)

	jobWorker := jobworker.New(
		ghActionsClient,
		ghBrokerClient,
		runnerConfig,
		createJobControllerFactory(ctx, runnerConfig.RunnerName, traceProvider),
		jobworker.WithTracerProvider(traceProvider),
	)

	jobManager := manager.New(
		jobListener,
		jobWorker,
		manager.WithTracerProvider(traceProvider),
	)

	if err := jobManager.Run(ctx); err != nil && !errors.Is(err, errInterrupted) {
		return fmt.Errorf("run command: %w", err)
	}

	return nil
}

type JobControllerFactory func(jobDetails ghapi.PipelineAgentJobRequest) (*jobcontroller.JobController, error)

func createJobControllerFactory(ctx context.Context, runnerName string, traceProvider trace.TracerProvider) jobworker.JobControllerFactory {
	return func(jobDetails ghapi.PipelineAgentJobRequest) (*jobcontroller.JobController, error) {
		httpClient := oauth2.NewClient(
			ctx,
			oauth2.StaticTokenSource(
				&oauth2.Token{
					AccessToken: jobDetails.Resources.Endpoints[0].Authorization.Parameters.AccessToken,
				},
			),
		)

		noAuthHttpClient := http.Client{}

		actionsClient := ghactions.New(
			jobDetails.Resources.Endpoints[0].ActionsServiceURL,
			ghactions.WithHTTPClient(httpClient),
			ghactions.WithTracerProvider(traceProvider),
		)

		resultsClient := resultsreceiver.New(
			jobDetails.Resources.Endpoints[0].ResultsServiceURL,
			resultsreceiver.WithHTTPClient(httpClient),
			resultsreceiver.WithTracerProvider(traceProvider),
		)

		blobClient := blobstorage.New(
			blobstorage.WithHTTPClient(&noAuthHttpClient),
			blobstorage.WithTracerProvider(traceProvider),
		)

		timelineController := timeline.NewController(
			actionsClient,
			runnerName,
			jobDetails.Plan.PlanID,
			jobDetails.Timeline.ID,
		)

		wsController := workflowsteps.NewController(
			resultsClient,
			jobDetails.Plan.PlanID,
			jobDetails.JobID,
		)

		jobController := jobcontroller.New(
			runnerName,
			timelineController,
			actionsClient,
			wsController,
			resultsClient,
			blobClient,
			jobcontroller.WithTracerProvider(traceProvider),
		)

		return jobController, nil
	}
}
