package jobcontroller

import (
	"context"
	"fmt"
	"io"
	"slices"
	"time"

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
	runnerName    string
	actionsClient *ghactions.Repository
	workflowSteps *workflowsteps.Controller
	timeline      *timeline.Controller
	tracer        trace.Tracer
	queueMain     []step.Step
	allJobs       []step.Step
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
		allJobs:       make([]step.Step, 0),
		tracer:        defaults.TraceProvider.Tracer(tracerName),
		queueMain:     make([]step.Step, 0),
	}

	for _, apply := range options {
		apply(&ctrl)
	}

	return &ctrl
}

func (c *JobController) AddStep(ctx context.Context, stepToAdd step.Step) {
	c.timeline.AddRecord(stepToAdd.ID(), stepToAdd.ParentID(), stepToAdd.DisplayName(), stepToAdd.RefName())

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

	lastIndex := len(c.queueMain) - 1

	insertIndex := 0
	insertIndex = lastIndex + 1
	for i := lastIndex; i >= 0; i-- {
		if isParentOrSibling(stepToAdd.ID(), c.queueMain[i]) {
			insertIndex = i + 1
			break
		}
	}

	queue := make([]step.Step, 0, len(c.queueMain)+1)
	queue = append(queue, c.queueMain[:insertIndex]...)
	queue = append(queue, stepToAdd)
	queue = append(queue, c.queueMain[insertIndex:]...)

	c.queueMain = queue

	c.allJobs = append(c.allJobs, stepToAdd)
}

func (c *JobController) Run(ctx context.Context, jobRequestMessage *ghapi.PipelineAgentJobRequest) error {
	ctx, span := c.tracer.Start(ctx, "run job")
	defer span.End()

	pretty.Println(jobRequestMessage)

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
		c.timeline.JobStarted(
			jobRequestMessage.JobID,
			jobRequestMessage.JobDisplayName,
			jobRequestMessage.JobName,
			time.Now(),
		)

		// mark job as completed
		// TODO: must send real job results
		defer func() {
			c.timeline.JobCompleted(jobRequestMessage.JobID, time.Now())
		}()
	}

	{
		// register init step that creates steps listed in workflow file
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

		c.AddStep(ctx, initStep)
	}

	type StepContext struct {
		step      step.Step
		ctx       context.Context
		span      trace.Span
		logWriter io.WriteCloser
	}

	stepsInProgressStack := make([]*StepContext, 0)

	for len(c.queueMain) > 0 {
		currentStep := c.queueMain[0]
		c.queueMain = c.queueMain[1:]

		parentStepCtx := ctx
		if len(stepsInProgressStack) > 0 {
			parentStepCtx = stepsInProgressStack[len(stepsInProgressStack)-1].ctx
		}

		ctx, span := c.tracer.Start(parentStepCtx, fmt.Sprintf("run step %s", currentStep.ID()))

		var parentLogWriters []io.Writer
		for _, parentStep := range stepsInProgressStack {
			if parentStep.logWriter == nil {
				continue
			}

			parentLogWriters = append(parentLogWriters, parentStep.logWriter)
		}

		logWriter := c.timeline.GetLogWriter(ctx, currentStep.ID())
		logWriter.Write([]byte{0xEF, 0xBB, 0xBF})

		parentLogWriters = append(parentLogWriters, logWriter)

		stepsInProgressStack = append(stepsInProgressStack, &StepContext{
			step:      currentStep,
			ctx:       ctx,
			span:      span,
			logWriter: logWriter,
		})

		// update timeline
		c.timeline.RecordStarted(currentStep.ID(), time.Now())

		taskLogWriter := timeline.NewMultiLogWriter(parentLogWriters...)

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
		for i := len(stepsInProgressStack) - 1; i >= 0; i-- {
			stackedTD := stepsInProgressStack[i]

			// find relations
			hasChildren := slices.ContainsFunc(c.queueMain, func(item step.Step) bool {
				return stackedTD.step.ID() == item.ParentID()
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
			c.timeline.RecordFinished(stepsInProgressStack[i].step.ID(), time.Now())
			stepsInProgressStack = stepsInProgressStack[:i]
		}
	}

	// shutdown workflow steps controller
	// if err := c.workflowSteps.Shutdown(ctx); err != nil {
	// 	logger.Error().Err(err).Msg("stop workflow steps controller")
	// }

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

func isParentOrSibling(id string, s step.Step) bool {
	return id == s.ParentID() || id == s.ID()
}
