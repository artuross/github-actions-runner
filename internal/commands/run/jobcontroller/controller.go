package jobcontroller

import (
	"context"
	"slices"
	"time"

	"github.com/artuross/github-actions-runner/internal/commands/run/runner"
	"github.com/artuross/github-actions-runner/internal/commands/run/timeline"
	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

type JobController struct {
	actionsClient    *ghactions.Repository
	timeline         *timeline.Controller
	tracer           trace.Tracer
	currentTaskStack []*runner.TaskDefinition // last in first out - currently active jobs
	queueMain        []*runner.TaskDefinition
}

func New(
	timeline *timeline.Controller,
	actionsClient *ghactions.Repository,
	traceProvider trace.TracerProvider,
) *JobController {
	return &JobController{
		actionsClient:    actionsClient,
		timeline:         timeline,
		tracer:           traceProvider.Tracer("github.com/artuross/internal/commands/run/jobcontroller"),
		currentTaskStack: make([]*runner.TaskDefinition, 0),
		queueMain:        make([]*runner.TaskDefinition, 0),
	}
}

func (c *JobController) AddTask(ctx context.Context, task runner.Task, parentID *string, name, refName string) {
	timelineRecordID := timeline.ID(task.ID())

	timelineRecordParentID := (*timeline.ID)(nil)
	if parentID != nil {
		timelineRecordParentID = (*timeline.ID)(parentID)
	}

	timelineTaskType := timeline.TypeTask
	if task.Type() == "job" {
		timelineTaskType = timeline.TypeJob
	} else if task.Type() == "task" {
		timelineTaskType = timeline.TypeTask
	} else {
		timelineTaskType = "unknown"
	}

	c.timeline.AddRecord(timelineRecordID, timelineRecordParentID, timelineTaskType, name, refName)

	zerolog.Ctx(ctx).Error().Any("arr", c.queueMain).Msg("snap")
	zerolog.Ctx(ctx).Info().Any("task_id", task.ID()).Any("parent_id", parentID).Msg("task")

	lastIndex := len(c.queueMain) - 1

	insertIndex := 0
	if parentID == nil {
		for i := lastIndex; i >= 0; i-- {
			if c.queueMain[i].ParentID == nil {
				insertIndex = i + 1
				break
			}
		}
	}

	if parentID != nil {
		insertIndex = lastIndex + 1
		for i := lastIndex; i >= 0; i-- {
			if (c.queueMain[i].ParentID != nil && *c.queueMain[i].ParentID == *parentID) || c.queueMain[i].ID == *parentID {
				insertIndex = i + 1
				break
			}
		}
	}

	queue := make([]*runner.TaskDefinition, 0, len(c.queueMain)+1)
	queue = append(queue, c.queueMain[:insertIndex]...)
	queue = append(queue, &runner.TaskDefinition{
		ID:       task.ID(),
		ParentID: parentID,
		Task:     task,
	})
	queue = append(queue, c.queueMain[insertIndex:]...)

	c.queueMain = queue

	zerolog.Ctx(ctx).Error().Any("arr", c.queueMain).Msg("post")
}

func (c *JobController) Run(ctx context.Context, runnerName string, jobRequestMessage *ghapi.PipelineAgentJobRequest) error {
	ctx, span := c.tracer.Start(ctx, "run job")
	defer span.End()

	logger := zerolog.Ctx(ctx)

	if err := c.timeline.Start(ctx); err != nil {
		logger.Error().Err(err).Msg("start timeline controller")
		return err
	}

	// register jobs
	c.AddTask(
		ctx,
		&runner.RunnerJob{Id: jobRequestMessage.JobID},
		nil,
		"hello",
		"__default",
	)

	// TODO: is the ID generated?
	c.AddTask(
		ctx,
		&runner.RunnerTaskInit{Id: "e57bfafe-5896-4d3f-881e-7e298f92fbee", ParentId: jobRequestMessage.JobID, Steps: jobRequestMessage.Steps},
		&jobRequestMessage.JobID,
		"Set up job",
		"JobExtension_Init",
	)

	for len(c.queueMain) > 0 {
		task := c.queueMain[0]
		c.queueMain = c.queueMain[1:]

		c.currentTaskStack = append(c.currentTaskStack, task)

		// update timeline
		c.timeline.RecordStarted(timeline.ID(task.ID), time.Now())

		// run task
		if runnable, ok := task.Task.(runner.Runnable); ok {
			logger.Debug().Str("task_id", task.ID).Msg("running task in controller")

			// ctx, span := e.tracer.Start(ctx, fmt.Sprintf("run step %s", task.ID))
			if err := runnable.Run(ctx, c); err != nil {
				logger.Error().Err(err).Msg("run task")
				continue
			}
			// span.End()
		}

		// update timeline
		for i := len(c.currentTaskStack) - 1; i >= 0; i-- {
			task := c.currentTaskStack[i]

			// find relations
			hasChildren := slices.ContainsFunc(c.queueMain, func(item *runner.TaskDefinition) bool {
				if item.ParentID == nil {
					return false
				}

				return task.ID == *item.ParentID
			})

			if hasChildren {
				break
			}

			// update record and remove from stack
			c.timeline.RecordFinished(timeline.ID(c.currentTaskStack[i].ID), time.Now())
			c.currentTaskStack = c.currentTaskStack[:i]
		}
	}

	// shutdown timeline controller
	if err := c.timeline.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("stop timeline controller")
	}

	// mark action as done
	if err := c.actionsClient.PostEvents(ctx, jobRequestMessage.Plan.PlanID, jobRequestMessage.JobID, jobRequestMessage.RequestID); err != nil {
		logger.Error().Err(err).Msg("post events")
	}

	return nil
}
