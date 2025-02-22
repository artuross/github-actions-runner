package root

import (
	"github.com/artuross/github-actions-runner/internal/commands/configure"
	"github.com/artuross/github-actions-runner/internal/commands/run"
	cli "github.com/urfave/cli/v2"
)

func NewCommand() *cli.App {
	return &cli.App{
		Name: "runner",
		Commands: []*cli.Command{
			configure.NewCommand(),
			run.NewCommand(),
		},
	}
}
