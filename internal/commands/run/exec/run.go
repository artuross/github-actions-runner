package exec

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/artuross/github-actions-runner/internal/commands/run/runner"
	"github.com/artuross/github-actions-runner/internal/commands/run/timeline"
	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/artuross/github-actions-runner/internal/repository/ghbroker"
	"github.com/artuross/github-actions-runner/internal/runnerconfig"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
)

type Executor struct {
	actionsClient *ghactions.Repository
	brokerClient  *ghbroker.Repository
	tracer        trace.Tracer
	config        *runnerconfig.Config
}

func NewExecutor(
	actionsClient *ghactions.Repository,
	brokerClient *ghbroker.Repository,
	traceProvider trace.TracerProvider,
	config *runnerconfig.Config,
) *Executor {
	return &Executor{
		actionsClient: actionsClient,
		brokerClient:  brokerClient,
		tracer:        traceProvider.Tracer("github.com/artuross/internal/commands/run/exec"),
		config:        config,
	}
}

func (e *Executor) Run(ctx context.Context, runnerJobRequest ghapi.MessageRunnerJobRequest) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobRequest, err := e.actionsClient.GetRunnerMessageByID(ctx, runnerJobRequest.RunnerRequestID)
	if err != nil {
		return fmt.Errorf("get runner job request message: %w", err)
	}

	jobRequestMessage, ok := jobRequest.(ghapi.PipelineAgentJobRequest)
	if !ok {
		return fmt.Errorf("unknown message type")
	}

	// initial call to get next iteration time
	acquireJob, err := e.actionsClient.AcquireJob(ctx, e.config.RunnerGroupID, jobRequestMessage.RequestID)
	if !ok {
		return fmt.Errorf("acquire job: %w", err)
	}

	logger := zerolog.Ctx(ctx).With().Int64("request_id", jobRequestMessage.RequestID).Logger()

	// for keeping track of goroutines
	wg := sync.WaitGroup{}
	defer wg.Wait()

	// "lock" the job on interval
	wg.Add(1)
	go func(lockedUntil time.Time) {
		defer wg.Done()

		const lockSafetyMargin = 10 * time.Second

		nextIteration := func(releaseTime time.Time) time.Duration {
			return time.Until(releaseTime) - lockSafetyMargin
		}

		timer := time.NewTimer(nextIteration(lockedUntil))
		defer func() {
			// drain channel
			if !timer.Stop() {
				<-timer.C
			}
		}()

		for {
			select {
			case <-ctx.Done():
				logger.Info().Msg("context canceled")
				return

			case <-timer.C:
				acquireJob, err := e.actionsClient.AcquireJob(ctx, e.config.RunnerGroupID, jobRequestMessage.RequestID)
				if !ok {
					logger.Error().Err(err).Msg("acquire job")

					// sleep for a second to reduce API calls during errors
					time.Sleep(time.Second)

					continue
				}

				timer.Reset(nextIteration(acquireJob.LockedUntil))
			}
		}
	}(acquireJob.LockedUntil)

	httpClient := oauth2.NewClient(
		ctx,
		oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: jobRequestMessage.Resources.Endpoints[0].Authorization.Parameters.AccessToken,
			},
		),
	)

	jobActionsClient := ghactions.New(
		jobRequestMessage.Resources.Endpoints[0].URL,
		ghactions.WithHTTPClient(httpClient),
	)

	timelineController := timeline.NewController(
		jobActionsClient,
		e.config.RunnerName,
		jobRequestMessage.Plan.PlanID,
		jobRequestMessage.Timeline.ID,
	)

	jobController := &JobController{
		timelineController: timelineController,
		jobActionsClient:   jobActionsClient,
		CurrentTaskStack:   []*runner.TaskDefinition{},
		QueueMain:          nil,
		Tracer:             e.tracer, // TODO: create new tracer
	}

	if err := jobController.Run(ctx, e.config.RunnerName, &jobRequestMessage); err != nil {
		logger.Error().Err(err).Msg("run job controller")
		return err
	}

	cancel()

	return nil
}

type JobController struct {
	timelineController *timeline.Controller
	jobActionsClient   *ghactions.Repository
	CurrentTaskStack   []*runner.TaskDefinition // last in first out - currently active jobs
	QueueMain          []*runner.TaskDefinition
	Tracer             trace.Tracer
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

	c.timelineController.AddRecord(timelineRecordID, timelineRecordParentID, timelineTaskType, name, refName)

	zerolog.Ctx(ctx).Error().Any("arr", c.QueueMain).Msg("snap")
	zerolog.Ctx(ctx).Info().Any("task_id", task.ID()).Any("parent_id", parentID).Msg("task")

	lastIndex := len(c.QueueMain) - 1

	insertIndex := 0
	if parentID == nil {
		for i := lastIndex; i >= 0; i-- {
			if c.QueueMain[i].ParentID == nil {
				insertIndex = i + 1
				break
			}
		}
	}

	if parentID != nil {
		insertIndex = lastIndex + 1
		for i := lastIndex; i >= 0; i-- {
			if (c.QueueMain[i].ParentID != nil && *c.QueueMain[i].ParentID == *parentID) || c.QueueMain[i].ID == *parentID {
				insertIndex = i + 1
				break
			}
		}
	}

	queue := make([]*runner.TaskDefinition, 0, len(c.QueueMain)+1)
	queue = append(queue, c.QueueMain[:insertIndex]...)
	queue = append(queue, &runner.TaskDefinition{
		ID:       task.ID(),
		ParentID: parentID,
		Task:     task,
	})
	queue = append(queue, c.QueueMain[insertIndex:]...)

	c.QueueMain = queue

	zerolog.Ctx(ctx).Error().Any("arr", c.QueueMain).Msg("post")
}

func (c *JobController) Run(ctx context.Context, runnerName string, jobRequestMessage *ghapi.PipelineAgentJobRequest) error {
	ctx, span := c.Tracer.Start(ctx, "run action")
	defer span.End()

	logger := zerolog.Ctx(ctx)

	if err := c.timelineController.Start(ctx); err != nil {
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

	for len(c.QueueMain) > 0 {
		task := c.QueueMain[0]
		c.QueueMain = c.QueueMain[1:]

		c.CurrentTaskStack = append(c.CurrentTaskStack, task)

		// update timeline
		c.timelineController.RecordStarted(timeline.ID(task.ID), time.Now())

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
		for i := len(c.CurrentTaskStack) - 1; i >= 0; i-- {
			task := c.CurrentTaskStack[i]

			// find relations
			hasChildren := slices.ContainsFunc(c.QueueMain, func(item *runner.TaskDefinition) bool {
				if item.ParentID == nil {
					return false
				}

				return task.ID == *item.ParentID
			})

			if hasChildren {
				break
			}

			// update record and remove from stack
			c.timelineController.RecordFinished(timeline.ID(c.CurrentTaskStack[i].ID), time.Now())
			c.CurrentTaskStack = c.CurrentTaskStack[:i]
		}
	}

	// shutdown timeline controller
	if err := c.timelineController.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("stop timeline controller")
	}

	// mark action as done
	if err := c.jobActionsClient.PostEvents(ctx, jobRequestMessage.Plan.PlanID, jobRequestMessage.JobID, jobRequestMessage.RequestID); err != nil {
		logger.Error().Err(err).Msg("post events")
	}

	return nil
}

type Controller interface {
	AddTask(ctx context.Context, task runner.Task, parentID *string)
}
