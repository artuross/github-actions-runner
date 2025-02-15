package main

import (
	"fmt"
	"os"

	"github.com/artuross/github-actions-runner/internal/commands/root"
)

func main() {
	rootCmd := root.NewCommand()

	if err := rootCmd.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
