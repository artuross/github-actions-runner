package jobcontroller

import (
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/artuross/github-actions-runner/internal/commands/run/runner"
	"github.com/artuross/github-actions-runner/internal/commands/run/step"
	"github.com/artuross/github-actions-runner/internal/commands/run/timeline"
	"github.com/artuross/github-actions-runner/internal/commands/run/workflowsteps"
	"github.com/artuross/github-actions-runner/internal/defaults"
	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/kr/pretty"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TODO: may want to do it via debug.ReadBuildInfo
	tracerName = "github.com/artuross/github-actions-runner/internal/commands/run/jobcontroller"
)

type JobController struct {
	actionsClient *ghactions.Repository
	workflowSteps *workflowsteps.Controller
	timeline      *timeline.Controller
	tracer        trace.Tracer
	queueMain     []*runner.TaskDefinition
	allJobs       []*runner.TaskDefinition
}

func New(
	timeline *timeline.Controller,
	actionsClient *ghactions.Repository,
	workflowSteps *workflowsteps.Controller,
	options ...func(*JobController),
) *JobController {
	ctrl := JobController{
		actionsClient: actionsClient,
		workflowSteps: workflowSteps,
		timeline:      timeline,
		allJobs:       make([]*runner.TaskDefinition, 0),
		tracer:        defaults.TraceProvider.Tracer(tracerName),
		queueMain:     make([]*runner.TaskDefinition, 0),
	}

	for _, apply := range options {
		apply(&ctrl)
	}

	return &ctrl
}

func (c *JobController) AddTask(ctx context.Context, task runner.Task, parentID *string, name, refName string) {
	fmt.Println("adding with name", name, refName, task.ID(), task.DisplayName())

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

	// if task.Type() == "task" {
	// 	taskCount := 0
	// 	for _, t := range c.allJobs {
	// 		if t.Task.Type() == "task" {
	// 			taskCount++
	// 		}
	// 	}

	// 	c.workflowSteps.SendUpdate(resultsreceiver.Step{
	// 		ExternalID: task.ID(),
	// 		Name:       task.Name(),
	// 		Status:     resultsreceiver.StatusPending,
	// 		Number:     taskCount + 1,
	// 	})
	// }

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

	thisTask := &runner.TaskDefinition{
		ID:       task.ID(),
		ParentID: parentID,
		Task:     task,
	}

	queue := make([]*runner.TaskDefinition, 0, len(c.queueMain)+1)
	queue = append(queue, c.queueMain[:insertIndex]...)
	queue = append(queue, thisTask)
	queue = append(queue, c.queueMain[insertIndex:]...)

	c.queueMain = queue

	c.allJobs = append(c.allJobs, thisTask)

	zerolog.Ctx(ctx).Error().Any("arr", c.queueMain).Msg("post")
}

func (c *JobController) Run(ctx context.Context, runnerName string, jobRequestMessage *ghapi.PipelineAgentJobRequest) error {
	ctx, span := c.tracer.Start(ctx, "run job")
	defer span.End()

	pretty.Println(jobRequestMessage)

	logger := zerolog.Ctx(ctx)

	if err := c.timeline.Start(ctx); err != nil {
		logger.Error().Err(err).Msg("start timeline controller")
		return err
	}

	// if err := c.workflowSteps.Start(ctx); err != nil {
	// 	logger.Error().Err(err).Msg("start workflow steps controller")
	// 	return err
	// }

	// register job
	c.timeline.JobStarted(
		timeline.ID(jobRequestMessage.JobID),
		jobRequestMessage.JobDisplayName,
		jobRequestMessage.JobName,
		time.Now(),
	)

	{
		initStep, err := step.NewInternalStart(jobRequestMessage.JobID, jobRequestMessage.Steps)
		if err != nil {
			logger.Error().Err(err).Msg("create init step")
			return err
		}

		// c.queueMain = append(c.queueMain, &runner.TaskDefinition{
		// 	ID:       initStep.ID(),
		// 	ParentID: initStep.ParentID(),
		// 	Task:     initStep,
		// })
		//

		c.AddTask(
			ctx,
			initStep,
			initStep.ParentID(),
			initStep.DisplayName(),
			initStep.RefName(),
		)
	}

	type StackedTaskDefinition struct {
		td        *runner.TaskDefinition
		ctx       context.Context
		span      trace.Span
		logWriter io.WriteCloser
	}

	currentTaskStack := make([]*StackedTaskDefinition, 0)

	for len(c.queueMain) > 0 {
		task := c.queueMain[0]
		c.queueMain = c.queueMain[1:]

		parentTaskCtx := ctx
		if len(currentTaskStack) > 0 {
			parentTaskCtx = currentTaskStack[len(currentTaskStack)-1].ctx
		}

		ctx, span := c.tracer.Start(parentTaskCtx, fmt.Sprintf("run step %s", task.ID))

		var parentLogWriters []io.Writer
		for _, task := range currentTaskStack {
			if task.logWriter == nil {
				continue
			}

			parentLogWriters = append(parentLogWriters, task.logWriter)
		}

		logWriter := c.timeline.GetLogWriter(ctx, timeline.ID(task.ID))
		logWriter.Write([]byte{0xEF, 0xBB, 0xBF})

		parentLogWriters = append(parentLogWriters, logWriter)

		currentTaskStack = append(currentTaskStack, &StackedTaskDefinition{
			td:        task,
			ctx:       ctx,
			span:      span,
			logWriter: logWriter,
		})

		// update timeline
		c.timeline.RecordStarted(timeline.ID(task.ID), time.Now())

		// prepare: resolves next steps
		if step, ok := task.Task.(step.Preparer); ok {
			logger.Debug().Str("task_id", task.ID).Msg("running prepare step in controller")

			taskLogWriter := timeline.NewMultiLogWriter(parentLogWriters...)

			stepsToAdd, err := step.Prepare(ctx, taskLogWriter)
			if err != nil {
				logger.Error().Err(err).Msg("run step")
				continue
			}

			for _, step := range stepsToAdd {
				c.AddTask(ctx, step, step.ParentID(), step.DisplayName(), step.RefName())
			}
		}

		// run task
		if runnable, ok := task.Task.(runner.Runnable); ok {
			logger.Debug().Str("task_id", task.ID).Msg("running task in controller")

			taskLogWriter := timeline.NewMultiLogWriter(parentLogWriters...)

			if err := runnable.Run(ctx, c, taskLogWriter); err != nil {
				logger.Error().Err(err).Msg("run task")
				continue
			}
		}

		// update timeline
		for i := len(currentTaskStack) - 1; i >= 0; i-- {
			stackedTD := currentTaskStack[i]

			// find relations
			hasChildren := slices.ContainsFunc(c.queueMain, func(item *runner.TaskDefinition) bool {
				if item.ParentID == nil {
					return false
				}

				return stackedTD.td.ID == *item.ParentID
			})

			if hasChildren {
				break
			}

			// if stackedTD.td.Task.Type() == "task" {
			// 	c.workflowSteps.SendUpdate(workflowsteps.UpdateCompleted{
			// 		ID:          stackedTD.td.Task.ID(),
			// 		CompletedAt: time.Now(),
			// 		Status:      resultsreceiver.StatusCompleted,
			// 		Conclusion:  resultsreceiver.ConclusionSuccess,
			// 	})
			// }

			// end span
			stackedTD.span.End()

			// close logger
			stackedTD.logWriter.Close()

			// update record and remove from stack
			c.timeline.RecordFinished(timeline.ID(currentTaskStack[i].td.ID), time.Now())
			currentTaskStack = currentTaskStack[:i]
		}
	}

	// mark job as completed
	c.timeline.JobCompleted(timeline.ID(jobRequestMessage.JobID), time.Now())

	// shutdown workflow steps controller
	// if err := c.workflowSteps.Shutdown(ctx); err != nil {
	// 	logger.Error().Err(err).Msg("stop workflow steps controller")
	// }

	// shutdown timeline controller
	if err := c.timeline.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("stop timeline controller")
	}

	// mark action as done
	if err := c.actionsClient.SendEventJobCompleted(ctx, jobRequestMessage.Plan.PlanID, jobRequestMessage.JobID, jobRequestMessage.RequestID); err != nil {
		logger.Error().Err(err).Msg("post events")
	}

	return nil
}

func WithTracerProvider(tp trace.TracerProvider) func(*JobController) {
	return func(r *JobController) {
		r.tracer = tp.Tracer(tracerName)
	}
}
