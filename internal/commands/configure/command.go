package configure

import (
	"fmt"
	"net/http"
	"os"

	"github.com/artuross/github-actions-runner/internal/commands/configure/config"
	"github.com/artuross/github-actions-runner/internal/commands/configure/exec"
	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
	"github.com/artuross/github-actions-runner/internal/repository/ghrest"
	"github.com/artuross/github-actions-runner/internal/repository/ghrunner"
	"github.com/google/go-github/v69/github"
	cli "github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"golang.org/x/oauth2"
)

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "configure",
		Usage: "Registers a new runner with GitHub.",
		Flags: []cli.Flag{
			// required
			&cli.StringFlag{
				Name:     "name",
				Usage:    "Name of the runner visible in the GitHub UI. Must be unique.",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "label",
				Usage:    "Label to associate with this runner.",
				Required: true,
			},

			// one of these must be set
			&cli.StringFlag{
				Name:  "organization",
				Usage: "The organization to register the runner with.",
			},

			// optional
			&cli.StringFlag{
				Name:     "runner-config-file",
				Usage:    "Destination path for the runner configuration file.",
				Value:    "./.config/runner.json",
				Required: false,
			},
			&cli.StringFlag{
				Name:     "private-key-file",
				Usage:    "Destination path for the runner private key file.",
				Value:    "./.config/key.pem",
				Required: false,
			},
		},
		Action: run,
	}
}

func run(cliCtx *cli.Context) error {
	ctx := cliCtx.Context

	cfg, err := config.Read(cliCtx, os.Getenv)
	if err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	config.Print(cfg)

	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithCompressor("gzip"),
	)
	if err != nil {
		return fmt.Errorf("create OTEL exporter: %w", err)
	}

	resource, err := sdkresource.New(
		ctx,
		sdkresource.WithTelemetrySDK(),
		sdkresource.WithAttributes(
			semconv.ServiceName("runner"),
		),
	)
	if err != nil {
		return fmt.Errorf("create OTEL resource: %w", err)
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			exporter,
			sdktrace.WithBlocking(),
			sdktrace.WithMaxQueueSize(10000),
			sdktrace.WithMaxExportBatchSize(1000),
		),
		sdktrace.WithResource(resource),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer traceProvider.Shutdown(ctx)

	staticTokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: cfg.GithubToken,
		},
	)

	tracer := traceProvider.Tracer("command")

	ctx, span := tracer.Start(ctx, "configure")
	defer span.End()

	ghHTTPClient := oauth2.NewClient(ctx, staticTokenSource)
	ghAPIClient := github.NewClient(ghHTTPClient)

	ghRESTClient := ghrest.New(traceProvider, ghAPIClient)
	ghRunnerClient := ghrunner.New(traceProvider, http.DefaultClient, "https://api.github.com")
	ghActionsClient := ghactions.New(traceProvider, http.DefaultClient, "https://UNKNOWN.actions.githubusercontent.com/")

	execConfig := exec.Config{
		RunnerConfigFilePath: cfg.RunnerConfigFilePath,
		PrivateKeyFilePath:   cfg.PrivateKeyFilePath,
		TargetType:           cfg.TargetType,
		TargetPath:           cfg.TargetPath,
		RunnerName:           cfg.RunnerName,
		RunnerLabel:          cfg.RunnerLabel,
	}

	executor := exec.NewExecutor(ghActionsClient, ghRESTClient, ghRunnerClient, traceProvider)
	err = executor.Run(ctx, execConfig)
	if err != nil {
		return fmt.Errorf("run executor: %w", err)
	}

	return nil
}
