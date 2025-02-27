package step

import (
	"context"
	"fmt"
	"io"

	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/google/uuid"
)

var (
	_ Preparer = (*InternalStart)(nil)
	_ Step     = (*InternalStart)(nil)
)

type InternalStart struct {
	id          string
	parentID    string
	displayName string
	refName     string

	initialSteps []ghapi.Step
}

func NewInternalStart(jobID string, initialSteps []ghapi.Step) (*InternalStart, error) {
	stepID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate step ID: %w", err)
	}

	step := InternalStart{
		id:           stepID.String(),
		parentID:     jobID,
		displayName:  "Set up job",
		refName:      "JobExtension_Init",
		initialSteps: initialSteps,
	}

	return &step, nil
}

// for Preparer
// TODO: actually return true implementations
func (s *InternalStart) Prepare(_ context.Context, _ io.Writer) ([]Step, error) {
	steps := make([]Step, 0, len(s.initialSteps))

	for _, stepDef := range s.initialSteps {
		step := Noop{
			id:          stepDef.ID,
			parentID:    s.parentID,
			displayName: stepDef.DisplayName,
			refName:     stepDef.ContextName,
		}

		steps = append(steps, &step)
	}

	return steps, nil
}

// for Step
func (s *InternalStart) ID() string          { return s.id }
func (s *InternalStart) ParentID() string    { return s.parentID }
func (s *InternalStart) DisplayName() string { return s.displayName }
func (s *InternalStart) RefName() string     { return s.refName }

// for Typer
func (s *InternalStart) Type() string { return "task" } // TODO: cleanup
