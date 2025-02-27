package runner

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/rs/zerolog/log"
)

type StepWithParentID struct {
	Step     ghapi.Step
	ParentID *string
}

type Runnable interface {
	Run(ctx context.Context, ctrl controller, logWriter io.Writer) error
}

var (
	_ Task = (*RunnerTaskComposite)(nil)
	_ Task = (*RunnerTaskFake)(nil)
)

type (
	RunnerTaskComposite struct {
		id    string
		Namee string
	}

	RunnerTaskFake struct {
		id    string
		Namee string
	}
)

func (j *RunnerTaskComposite) ID() string { return j.id }
func (j *RunnerTaskFake) ID() string      { return j.id }

func (j *RunnerTaskComposite) DisplayName() string { return j.Namee }
func (j *RunnerTaskFake) DisplayName() string      { return j.Namee }

func (t *RunnerTaskComposite) Type() string { return "task" }
func (t *RunnerTaskFake) Type() string      { return "task" }

func (r *RunnerTaskFake) Run(ctx context.Context, _ controller, logWriter io.Writer) error {
	logger := log.Ctx(ctx)

	logger.Debug().Msg("running simple task")
	defer logger.Debug().Msg("completed simple task")

	logWriter.Write([]byte(fmt.Sprintf("%s running simple task\n", time.Now().UTC().Format("2006-01-02T15:04:05.0000000Z07:00"))))
	logWriter.Write([]byte(fmt.Sprintf("%s completed simple task\n", time.Now().UTC().Format("2006-01-02T15:04:05.0000000Z07:00"))))

	time.Sleep(time.Second*time.Duration(rand.Intn(10)) + time.Second)

	return nil
}

// TODO: delete
func fakeChildStep(parentID string, index int) StepWithParentID {
	return StepWithParentID{
		ParentID: &parentID,
		Step: ghapi.Step{
			ID:   fmt.Sprintf("%s_%d", parentID, index),
			Type: "action",
			Reference: ghapi.StepReference{
				Type: "script",
			},
			ContextName: fmt.Sprintf("%s_%d", parentID, index),
			DisplayName: fmt.Sprintf("%s_%d", parentID, index),
		},
	}
}
