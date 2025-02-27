package step

import (
	"context"
	"fmt"
	"io"
	"time"
)

var _ Step = (*Noop)(nil)

type Noop struct {
	id          string
	parentID    string
	displayName string
	refName     string
}

// for Runner
func (s *Noop) Run(ctx context.Context, logWriter io.Writer) error {
	fmt.Fprintf(logWriter, "%s start task '%s' with id %s\n", getTime(), s.displayName, s.id)
	defer fmt.Fprintf(logWriter, "%s finish task '%s' with id %s\n", getTime(), s.displayName, s.id)

	time.Sleep(time.Second)

	return nil
}

// for Step
func (s *Noop) ID() string          { return s.id }
func (s *Noop) ParentID() string    { return s.parentID }
func (s *Noop) DisplayName() string { return s.displayName }
func (s *Noop) RefName() string     { return s.refName }

// for Typer
func (s *Noop) Type() string { return "task" } // TODO: cleanup

func getTime() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.0000000Z07:00")
}
