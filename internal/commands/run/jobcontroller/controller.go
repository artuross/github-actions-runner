package jobcontroller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/artuross/github-actions-runner/internal/commands/run/jobcontroller/internal/queue"
	"github.com/artuross/github-actions-runner/internal/commands/run/jobcontroller/internal/stack"
	"github.com/artuross/github-actions-runner/internal/commands/run/log/buffer"
	"github.com/artuross/github-actions-runner/internal/commands/run/log/uploader"
	"github.com/artuross/github-actions-runner/internal/commands/run/step"
	"github.com/artuross/github-actions-runner/internal/commands/run/timeline"
	"github.com/artuross/github-actions-runner/internal/commands/run/workflowsteps"
	"github.com/artuross/github-actions-runner/internal/defaults"
	"github.com/artuross/github-actions-runner/internal/log/semconv"
	"github.com/artuross/github-actions-runner/internal/repository/blobstorage"
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
	blobStorage     *blobstorage.Repository
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
	blobStorage *blobstorage.Repository,
	options ...func(*JobController),
) *JobController {
	ctrl := JobController{
		runnerName:      runnerName,
		actionsClient:   actionsClient,
		workflowSteps:   workflowSteps,
		resultsReceiver: resultsReceiver,
		blobStorage:     blobStorage,
		timeline:        timeline,
		tracer:          defaults.TracerProvider.Tracer(tracerName),
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
	wg := sync.WaitGroup{}

	ctx, span := c.tracer.Start(ctx, "run job")
	defer span.End()

	logger := zerolog.Ctx(ctx)

	c.timeline.Start(ctx)
	c.workflowSteps.Start(ctx)

	logWriter := buffer.NewBuffer()

	// create job logger
	jobLogWriter := logWriter.WindowedBuffer()

	// upload logs to results service
	jobResultsLogReader := jobLogWriter.Reader()
	jobResultsLogUploader := uploader.New(
		jobResultsLogReader,
		workflowsteps.LogFileMaxSize,
		workflowsteps.JobUploader(
			c.resultsReceiver,
			c.blobStorage,
			jobDetails.Plan.PlanID,
			jobDetails.JobID,
		),
	)

	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := jobResultsLogUploader.Start(ctx); err != nil {
			logger.Error().Err(err).Msg("upload job logs to results")
		}
	}()

	// upload logs to timeline service
	jobTimelineLogReader := jobLogWriter.Reader()
	jobTimelineLogUploader := uploader.New(
		jobTimelineLogReader,
		workflowsteps.LogFileMaxSize,
		timeline.LogUploader(
			c.timeline,
			c.actionsClient,
			jobDetails.Plan.PlanID,
			jobDetails.JobID,
		),
	)

	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := jobTimelineLogUploader.Start(ctx); err != nil {
			logger.Error().Err(err).Msg("upload job logs to timeline")
		}
	}()

	// register job
	c.timeline.JobStarted(jobDetails.JobID, jobDetails.JobDisplayName, jobDetails.JobName, time.Now())

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

		parentLogWriter := stack.LogWriter(jobLogWriter)
		if parent := executionContext.Peek(); parent != nil {
			parentLogWriter = parent.LogWriter
		}

		stepLogWriter := parentLogWriter.WindowedBuffer()

		// upload step logs to results service
		stepResultsLogReader := stepLogWriter.Reader()
		stepResultsLogUploader := uploader.New(
			stepResultsLogReader,
			workflowsteps.LogFileMaxSize,
			workflowsteps.StepUploader(
				c.resultsReceiver,
				c.blobStorage,
				jobDetails.Plan.PlanID,
				jobDetails.JobID,
				currentStep.ID(),
			),
		)

		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := stepResultsLogUploader.Start(ctx); err != nil {
				logger.Error().Err(err).Msg("upload step logs to results")
			}
		}()

		// upload logs to timeline service
		stepTimelineLogReader := stepLogWriter.Reader()
		stepTimelineLogUploader := uploader.New(
			stepTimelineLogReader,
			workflowsteps.LogFileMaxSize,
			timeline.LogUploader(
				c.timeline,
				c.actionsClient,
				jobDetails.Plan.PlanID,
				currentStep.ID(),
			),
		)

		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := stepTimelineLogUploader.Start(ctx); err != nil {
				logger.Error().Err(err).Msg("upload step logs to timeline")
			}
		}()

		executionContext.Push(ctx, currentStep, span, stepLogWriter, stepResultsLogReader, stepTimelineLogReader)

		// update timeline
		c.timeline.RecordStarted(currentStep.ID(), time.Now())

		// prepare: resolves next steps
		if step, ok := currentStep.(step.Preparer); ok {
			logger.Debug().Str(semconv.StepID, currentStep.ID()).Msg("running prepare step in controller")

			stepsToAdd, err := step.Prepare(ctx, stepLogWriter)
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
			logger.Debug().Str(semconv.StepID, currentStep.ID()).Msg("running runner step in controller")

			if err := step.Run(ctx, stepLogWriter); err != nil {
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
			stepContext.LogWriter.Close()
			stepContext.ResultsLogReader.Close()
			stepContext.TimelineLogReader.Close()

			// update record and remove from stack
			c.timeline.RecordFinished(stepContext.Step.ID(), time.Now())

			// remove last item
			_ = executionContext.Pop()
		}
	}

	c.timeline.JobCompleted(jobDetails.JobID, time.Now())

	if err := jobResultsLogReader.Close(); err != nil {
		logger.Error().Err(err).Msg("cannot close job log results reader")
	}

	if err := jobTimelineLogReader.Close(); err != nil {
		logger.Error().Err(err).Msg("cannot close job log timeline reader")
	}

	if err := jobLogWriter.Close(); err != nil {
		logger.Error().Err(err).Msg("cannot close job log writer")
	}

	// wait for logs to be shipped
	wg.Wait()

	if err := c.timeline.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("stop timeline controller")
	}

	if err := c.workflowSteps.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("stop workflow steps controller")
	}

	// mark action as done
	// TODO: must be sent after final timeline update
	if err := c.actionsClient.SendEventJobCompleted(ctx, jobDetails.Plan.PlanID, jobDetails.JobID, jobDetails.RequestID); err != nil {
		logger.Error().Err(err).Msg("post events")
	}

	logger.Info().Msg("job finished")

	return nil
}

func WithTracerProvider(tp trace.TracerProvider) func(*JobController) {
	return func(r *JobController) {
		r.tracer = tp.Tracer(tracerName)
	}
}
