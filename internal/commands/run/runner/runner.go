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
	_ Task = (*RunnerTaskInit)(nil)
)

type (
	RunnerTaskComposite struct {
		id    string
		Namee string
	}

	RunnerTaskInit struct {
		Id       string
		Namee    string
		ParentId string
		Steps    []ghapi.Step
	}

	RunnerTaskFake struct {
		id    string
		Namee string
	}
)

func (j *RunnerTaskComposite) ID() string { return j.id }
func (j *RunnerTaskFake) ID() string      { return j.id }
func (j *RunnerTaskInit) ID() string      { return j.Id }

func (j *RunnerTaskComposite) Name() string { return j.Namee }
func (j *RunnerTaskFake) Name() string      { return j.Namee }
func (j *RunnerTaskInit) Name() string      { return j.Namee }

func (t *RunnerTaskComposite) Type() string { return "task" }
func (t *RunnerTaskFake) Type() string      { return "task" }
func (t *RunnerTaskInit) Type() string      { return "task" }

func (r *RunnerTaskFake) Run(ctx context.Context, _ controller, logWriter io.Writer) error {
	logger := log.Ctx(ctx)

	logger.Debug().Msg("running simple task")
	defer logger.Debug().Msg("completed simple task")

	logWriter.Write([]byte(fmt.Sprintf("%s running simple task\n", time.Now().UTC().Format("2006-01-02T15:04:05.0000000Z07:00"))))
	logWriter.Write([]byte(fmt.Sprintf("%s completed simple task\n", time.Now().UTC().Format("2006-01-02T15:04:05.0000000Z07:00"))))

	time.Sleep(time.Second*time.Duration(rand.Intn(10)) + time.Second)

	return nil
}

func (r *RunnerTaskInit) Run(ctx context.Context, ctrl controller, logWriter io.Writer) error {
	logger := log.Ctx(ctx)

	logger.Debug().Msg("running pre task")
	defer logger.Debug().Msg("completed pre task")

	logWriter.Write([]byte(fmt.Sprintf("%s running pre task\n", time.Now().UTC().Format("2006-01-02T15:04:05.0000000Z07:00"))))
	logWriter.Write([]byte(fmt.Sprintf("%s completed pre task\n", time.Now().UTC().Format("2006-01-02T15:04:05.0000000Z07:00"))))

	toProcess := make([]StepWithParentID, 0)
	for _, step := range r.Steps {
		toProcess = append(toProcess, StepWithParentID{
			ParentID: &r.ParentId,
			Step:     step,
		})
	}

	for len(toProcess) > 0 {
		current := toProcess[0]
		toProcess = toProcess[1:]

		parentID := ""
		if current.ParentID != nil {
			parentID = *current.ParentID
		}

		logger.Debug().
			Str("step_id", current.Step.ID).
			Str("parent_id", parentID).
			Msg("processing step in pre")

		// TODO: remove
		// if current.Step.ID == "6ef8fe4d-f1ed-5af7-65b4-6d8688e04def" {
		// 	current.Step.Type = "composite"
		// }

		var task Task
		if current.Step.Type == "composite" {
			task = &RunnerTaskComposite{
				id:    current.Step.ID,
				Namee: current.Step.Name,
			}
		} else {
			task = &RunnerTaskFake{
				id:    current.Step.ID,
				Namee: current.Step.Type,
			}
		}

		// adds task to processing queue
		// add runner, not a step
		ctrl.AddTask(ctx, task, current.ParentID, current.Step.Name, fmt.Sprintf("_%s", current.Step.ID))

		// TODO: remove
		// if current.Step.ID == "6ef8fe4d-f1ed-5af7-65b4-6d8688e04def" {
		// 	toProcess = append(
		// 		toProcess,
		// 		fakeChildStep(current.Step.ID, 1),
		// 		fakeChildStep(current.Step.ID, 2),
		// 	)

		// 	continue
		// }
	}

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
			Name:        fmt.Sprintf("%s_%d", parentID, index),
		},
	}
}
