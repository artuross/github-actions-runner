package run

import (
	"fmt"
	"net/http"
	"os"

	"github.com/artuross/github-actions-runner/internal/commandinit"
	"github.com/artuross/github-actions-runner/internal/commands/configure/config"
	"github.com/artuross/github-actions-runner/internal/commands/configure/exec"
	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
	"github.com/artuross/github-actions-runner/internal/repository/ghrest"
	"github.com/artuross/github-actions-runner/internal/repository/ghrunner"
	"github.com/google/go-github/v69/github"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/oauth2"
)

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

	cfg, err := config.Read(cliCtx, os.Getenv)
	if err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	config.Print(cfg)

	traceProvider, tpShutdown, err := commandinit.NewOpenTelemetry(ctx, "runner")
	if err != nil {
		return fmt.Errorf("create OTEL provider: %w", err)
	}
	defer tpShutdown(ctx)

	ghAPIClient := github.NewClient(
		oauth2.NewClient(
			ctx,
			oauth2.StaticTokenSource(
				&oauth2.Token{
					AccessToken: cfg.GithubToken,
				},
			),
		),
	)

	ghRESTClient := ghrest.New(
		ghAPIClient,
		ghrest.WithTracerProvider(traceProvider),
	)

	ghRunnerClient := ghrunner.New(
		"https://api.github.com",
		ghrunner.WithHTTPClient(http.DefaultClient), // TODO: use something better
		ghrunner.WithTracerProvider(traceProvider),
	)

	ghActionsClient := ghactions.New(
		"https://UNKNOWN.actions.githubusercontent.com/",
		ghactions.WithHTTPClient(http.DefaultClient), // TODO: use something better
		ghactions.WithTracerProvider(traceProvider),
	)

	execConfig := exec.Config{
		RunnerConfigFilePath: cfg.RunnerConfigFilePath,
		PrivateKeyFilePath:   cfg.PrivateKeyFilePath,
		TargetType:           cfg.TargetType,
		TargetPath:           cfg.TargetPath,
		RunnerName:           cfg.RunnerName,
		RunnerLabel:          cfg.RunnerLabel,
	}

	executor := exec.NewExecutor(ghActionsClient, ghRESTClient, ghRunnerClient, traceProvider)
	if err := executor.Run(ctx, execConfig); err != nil {
		return fmt.Errorf("run command: %w", err)
	}

	return nil
}
