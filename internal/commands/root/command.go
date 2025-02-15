package root

import (
	"github.com/artuross/github-actions-runner/internal/commands/configure"
	cli "github.com/urfave/cli/v2"
)

func NewCommand() *cli.App {
	return &cli.App{
		Name: "runner",
		Commands: []*cli.Command{
			configure.NewCommand(),
		},
	}
}
