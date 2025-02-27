package jobcontroller

import (
	"context"
	"fmt"
	"time"

	"github.com/artuross/github-actions-runner/internal/commands/run/jobcontroller/internal/queue"
	"github.com/artuross/github-actions-runner/internal/commands/run/jobcontroller/internal/stack"
	"github.com/artuross/github-actions-runner/internal/commands/run/step"
	"github.com/artuross/github-actions-runner/internal/commands/run/timeline"
	"github.com/artuross/github-actions-runner/internal/commands/run/workflowsteps"
	"github.com/artuross/github-actions-runner/internal/defaults"
	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TODO: may want to do it via debug.ReadBuildInfo
	tracerName = "github.com/artuross/github-actions-runner/internal/commands/run/jobcontroller"
)

type JobController struct {
	runnerName    string
	actionsClient *ghactions.Repository
	workflowSteps *workflowsteps.Controller
	timeline      *timeline.Controller
	tracer        trace.Tracer
	queueMain     *queue.OrderedQueue
}

func New(
	runnerName string,
	timeline *timeline.Controller,
	actionsClient *ghactions.Repository,
	workflowSteps *workflowsteps.Controller,
	options ...func(*JobController),
) *JobController {
	ctrl := JobController{
		runnerName:    runnerName,
		actionsClient: actionsClient,
		workflowSteps: workflowSteps,
		timeline:      timeline,
		tracer:        defaults.TraceProvider.Tracer(tracerName),
		queueMain:     &queue.OrderedQueue{},
	}

	for _, apply := range options {
		apply(&ctrl)
	}

	return &ctrl
}

func (c *JobController) AddStep(ctx context.Context, stepToAdd step.Step) {
	// add a step to the timeline
	c.timeline.AddRecord(stepToAdd.ID(), stepToAdd.ParentID(), stepToAdd.DisplayName(), stepToAdd.RefName())

	// add a step to the queue
	c.queueMain.Push(stepToAdd)

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
}

func (c *JobController) Run(ctx context.Context, jobDetails *ghapi.PipelineAgentJobRequest) error {
	ctx, span := c.tracer.Start(ctx, "run job")
	defer span.End()

	logger := zerolog.Ctx(ctx)

	{
		// timeline
		c.timeline.Start(ctx)
		defer func() {
			if err := c.timeline.Shutdown(ctx); err != nil {
				logger.Error().Err(err).Msg("stop timeline controller")
			}
		}()
	}

	// if err := c.workflowSteps.Start(ctx); err != nil {
	// 	logger.Error().Err(err).Msg("start workflow steps controller")
	// 	return err
	// }

	{
		// register job
		c.timeline.JobStarted(jobDetails.JobID, jobDetails.JobDisplayName, jobDetails.JobName, time.Now())

		// mark job as completed
		// TODO: must send real job results
		defer func() {
			c.timeline.JobCompleted(jobDetails.JobID, time.Now())
		}()
	}

	{
		// register init step that creates steps listed in workflow file
		initStep, err := step.NewInternalStart(jobDetails.JobID, jobDetails.Steps)
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

		c.AddStep(ctx, initStep)
	}

	executionContext := stack.NewExecutionContext(ctx)

	for c.queueMain.HasNext() {
		currentStep := c.queueMain.Pop()

		ctx, span := c.tracer.Start(executionContext.Context(), fmt.Sprintf("run step %s", currentStep.ID()))

		parentLogWriters := executionContext.LogWriters()

		logWriter := c.timeline.GetLogWriter(ctx, currentStep.ID())
		logWriter.Write([]byte{0xEF, 0xBB, 0xBF})

		executionContext.Push(ctx, currentStep, span, logWriter)

		// update timeline
		c.timeline.RecordStarted(currentStep.ID(), time.Now())

		taskLogWriter := timeline.NewMultiLogWriter(logWriter, parentLogWriters...)

		// prepare: resolves next steps
		if step, ok := currentStep.(step.Preparer); ok {
			logger.Debug().Str("task_id", currentStep.ID()).Msg("running prepare step in controller")

			stepsToAdd, err := step.Prepare(ctx, taskLogWriter)
			if err != nil {
				logger.Error().Err(err).Msg("run step")
				continue
			}

			for _, step := range stepsToAdd {
				c.AddStep(ctx, step)
			}
		}

		// runner: runs the main thing
		if step, ok := currentStep.(step.Runner); ok {
			logger.Debug().Str("task_id", currentStep.ID()).Msg("running runner step in controller")

			if err := step.Run(ctx, taskLogWriter); err != nil {
				logger.Error().Err(err).Msg("run step")
				continue
			}
		}

		// update timeline
		for !executionContext.Empty() {
			stepContext := executionContext.Peek()

			// find relations
			if c.queueMain.HasChildren(stepContext.Step.ID()) {
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
			stepContext.Span.End()

			// close logger
			stepContext.LogWriter.Close()

			// update record and remove from stack
			c.timeline.RecordFinished(stepContext.Step.ID(), time.Now())

			// remove last item
			_ = executionContext.Pop()
		}
	}

	// shutdown workflow steps controller
	// if err := c.workflowSteps.Shutdown(ctx); err != nil {
	// 	logger.Error().Err(err).Msg("stop workflow steps controller")
	// }

	// mark action as done
	if err := c.actionsClient.SendEventJobCompleted(ctx, jobDetails.Plan.PlanID, jobDetails.JobID, jobDetails.RequestID); err != nil {
		logger.Error().Err(err).Msg("post events")
	}

	return nil
}

func WithTracerProvider(tp trace.TracerProvider) func(*JobController) {
	return func(r *JobController) {
		r.tracer = tp.Tracer(tracerName)
	}
}
