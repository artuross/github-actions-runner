package jobcontroller

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/artuross/github-actions-runner/internal/commands/run/jobcontroller/internal/queue"
	"github.com/artuross/github-actions-runner/internal/commands/run/jobcontroller/internal/stack"
	"github.com/artuross/github-actions-runner/internal/commands/run/step"
	"github.com/artuross/github-actions-runner/internal/commands/run/timeline"
	"github.com/artuross/github-actions-runner/internal/commands/run/workflowsteps"
	"github.com/artuross/github-actions-runner/internal/defaults"
	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/artuross/github-actions-runner/internal/repository/resultsreceiver"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TODO: may want to do it via debug.ReadBuildInfo
	tracerName = "github.com/artuross/github-actions-runner/internal/commands/run/jobcontroller"
)

type JobController struct {
	runnerName      string
	actionsClient   *ghactions.Repository
	workflowSteps   *workflowsteps.Controller
	resultsReceiver *resultsreceiver.Repository
	timeline        *timeline.Controller
	tracer          trace.Tracer
	allSteps        *queue.OrderedQueue
	queueMain       *queue.OrderedQueue
}

func New(
	runnerName string,
	timeline *timeline.Controller,
	actionsClient *ghactions.Repository,
	workflowSteps *workflowsteps.Controller,
	resultsReceiver *resultsreceiver.Repository,
	options ...func(*JobController),
) *JobController {
	ctrl := JobController{
		runnerName:      runnerName,
		actionsClient:   actionsClient,
		workflowSteps:   workflowSteps,
		resultsReceiver: resultsReceiver,
		timeline:        timeline,
		tracer:          defaults.TraceProvider.Tracer(tracerName),
		allSteps:        &queue.OrderedQueue{},
		queueMain:       &queue.OrderedQueue{},
	}

	for _, apply := range options {
		apply(&ctrl)
	}

	return &ctrl
}

func (c *JobController) AddStep(ctx context.Context, stepToAdd step.Step) {
	// append only
	c.allSteps.Push(stepToAdd)

	// add a step to the queue
	c.queueMain.Push(stepToAdd)

	// add a step to the timeline
	c.timeline.AddRecord(stepToAdd.ID(), stepToAdd.ParentID(), stepToAdd.DisplayName(), stepToAdd.RefName())

	// add a step to the workflow steps
	c.workflowSteps.SendUpdate(resultsreceiver.Step{
		ExternalID: stepToAdd.ID(),
		Name:       stepToAdd.DisplayName(),
		Status:     resultsreceiver.StatusPending,
		Number:     c.allSteps.Length(), // 1..N
	})
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

	{
		// workflow steps
		c.workflowSteps.Start(ctx)
		defer func() {
			if err := c.workflowSteps.Shutdown(ctx); err != nil {
				logger.Error().Err(err).Msg("stop workflow steps controller")
			}
		}()
	}

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

		c.AddStep(ctx, initStep)
	}

	executionContext := stack.NewExecutionContext(ctx)

	for c.queueMain.HasNext() {
		currentStep := c.queueMain.Pop()

		ctx, span := c.tracer.Start(executionContext.Context(), fmt.Sprintf("run step %s", currentStep.ID()))

		parentLogWriters := executionContext.LogWriters()

		// for writing timeline logs
		timelineLogWriter := c.timeline.GetLogWriter(ctx, currentStep.ID())
		timelineLogWriter.Write([]byte{0xEF, 0xBB, 0xBF})

		// for writing results logs
		resultsLogWriter := workflowsteps.NewLogWriter(ctx, c.resultsReceiver, jobDetails.Plan.PlanID, jobDetails.JobID, currentStep.ID())
		resultsLogWriter.Write([]byte{0xEF, 0xBB, 0xBF})

		executionContext.Push(ctx, currentStep, span, timelineLogWriter, timelineLogWriter)

		// update timeline
		c.timeline.RecordStarted(currentStep.ID(), time.Now())

		taskLogWriter := NewMultiLogWriter(append([]io.Writer{timelineLogWriter, resultsLogWriter}, parentLogWriters...)...)

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

			// TODO: send real result
			c.workflowSteps.SendUpdate(workflowsteps.UpdateCompleted{
				ID:          stepContext.Step.ID(),
				CompletedAt: time.Now(),
				Status:      resultsreceiver.StatusCompleted,
				Conclusion:  resultsreceiver.ConclusionSuccess,
			})

			// end span
			stepContext.Span.End()

			// close loggers
			resultsLogWriter.Close()
			stepContext.TimelineLogWriter.Close()

			// update record and remove from stack
			c.timeline.RecordFinished(stepContext.Step.ID(), time.Now())

			// remove last item
			_ = executionContext.Pop()
		}
	}

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
