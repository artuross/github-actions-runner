package exec

import (
	"context"
	"errors"
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
}

func NewExecutor(
	actionsClient *ghactions.Repository,
	brokerClient *ghbroker.Repository,
	traceProvider trace.TracerProvider,
) *Executor {
	return &Executor{
		actionsClient: actionsClient,
		brokerClient:  brokerClient,
		tracer:        traceProvider.Tracer("github.com/artuross/internal/commands/run/exec"),
	}
}

func (e *Executor) Run(ctx context.Context, config *runnerconfig.Config) error {
	ctx, span := e.tracer.Start(ctx, "run")
	defer span.End()

	logger := zerolog.Ctx(ctx)

	var sessionID *string
	defer func() {
		if sessionID == nil {
			return
		}

		ctx := context.WithoutCancel(ctx)

		logger.Debug().Str("sessionID", *sessionID).Msg("deleting session")

		if err := e.actionsClient.DeleteSession(ctx, config.RunnerGroupID, *sessionID); err != nil {
			logger.Error().Err(err).Msg("delete session")
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)

		default:
		}

		logger.Debug().Int64("runnerID", config.RunnerID).Msg("creating session")

		session, err := e.actionsClient.CreateSession(ctx, config.RunnerGroupID, config.RunnerID, config.RunnerName)
		if err != nil {
			logger.Error().Err(err).Msg("create session")

			logger.Warn().Msg("retrying in 10 seconds")
			time.Sleep(10 * time.Second)

			// TODO: handle alternative session issues
			continue
		}

		sessionID = &session.SessionID

		resetMessageClient := time.NewTimer(10 * time.Minute)
		resetMessageClient.Stop()

		getPoolMessageProvider := "actions"
		getPoolMessage := e.actionsClient.GetPoolMessage
		resetGetPoolMessage := func() {
			resetMessageClient.Stop()

			select {
			case <-resetMessageClient.C:
			default:
			}

			getPoolMessageProvider = "actions"
			getPoolMessage = e.actionsClient.GetPoolMessage
		}

		for {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)

			default:
			}

			select {
			// if timer expired, reset getPoolMessage cache
			case <-resetMessageClient.C:
				resetGetPoolMessage()

			// otherwise proceed
			default:
			}

			logger.Debug().
				Int64("runnerID", config.RunnerID).
				Str("provider", getPoolMessageProvider).
				Msg("fetching pool message")

			// TODO: needs to happen on repeat even when there's an active job running
			message, err := getPoolMessage(ctx, session.SessionID, config.RunnerGroupID, ghapi.RunnerStatusOnline)
			if errors.Is(err, ghbroker.ErrorEmptyBody) {
				logger.Info().Msg("no message")
				continue
			}

			if err != nil {
				// on errors, reset getPoolMessage cache
				resetGetPoolMessage()

				logger.Error().Err(err).Msg("read pool message")
				continue
			}

			switch msg := message.(type) {
			case ghapi.BrokerMigration:
				if getPoolMessageProvider == "broker" {
					logger.Warn().Msg("received BrokerMigration message from broker")
				}

				getPoolMessageProvider = "broker"
				getPoolMessage = func(ctx context.Context, sessionID string, poolID int64, status ghapi.RunnerStatus) (ghapi.Message, error) {
					return e.brokerClient.GetPoolMessage(ctx, msg.BaseURL, sessionID, poolID, status)
				}

				resetMessageClient.Reset(10 * time.Minute)
				continue

			case ghapi.BrokerMessage:
				logger.Info().Int64("message_id", msg.MessageID).Msg("received BrokerMessage")

				runnerJobRequest, ok := msg.Message.(ghapi.MessageRunnerJobRequest)
				if !ok {
					logger.Error().Msg("unknown message type")
					continue
				}

				if err := e.startJob(ctx, config, *sessionID, runnerJobRequest); err != nil {
					logger.Error().Err(err).Msg("failed to start job")
					continue
				}

				// TODO: must happen right after job details are fetched
				if err := e.actionsClient.DeletePoolMessage(ctx, *sessionID, config.RunnerGroupID, msg.MessageID); err != nil {
					logger.Error().Err(err).Msg("delete pool message")
					continue
				}

				continue
			}

			logger.Warn().Any("message", message).Msg("unknown message")

			time.Sleep(10 * time.Second)
		}
	}
}

func (e *Executor) startJob(ctx context.Context, config *runnerconfig.Config, sessionID string, runnerJobRequest ghapi.MessageRunnerJobRequest) error {
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
	acquireJob, err := e.actionsClient.AcquireJob(ctx, config.RunnerGroupID, jobRequestMessage.RequestID)
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
				acquireJob, err := e.actionsClient.AcquireJob(ctx, config.RunnerGroupID, jobRequestMessage.RequestID)
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
		config.RunnerName,
		jobRequestMessage.Plan.PlanID,
		jobRequestMessage.Timeline.ID,
	)

	if err := timelineController.Start(ctx); err != nil {
		logger.Error().Err(err).Msg("start timeline controller")
		return err
	}

	jobController := &JobController{
		timelineController: timelineController,
		CurrentTaskStack:   []*runner.TaskDefinition{},
		QueueMain:          nil,
	}

	// register jobs
	jobController.AddTask(
		ctx,
		&runner.RunnerJob{Id: jobRequestMessage.JobID},
		nil,
		"hello",
		"__default",
	)

	// TODO: is the ID generated?
	jobController.AddTask(
		ctx,
		&runner.RunnerTaskInit{Id: "e57bfafe-5896-4d3f-881e-7e298f92fbee", ParentId: jobRequestMessage.JobID, Steps: jobRequestMessage.Steps},
		&jobRequestMessage.JobID,
		"Set up job",
		"JobExtension_Init",
	)

	ctx, span := e.tracer.Start(ctx, "run action")
	defer span.End()

	if err := jobController.Run(ctx); err != nil {
		logger.Error().Err(err).Msg("run job controller")
		return err
	}

	// shutdown timeline controller
	if err := timelineController.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("stop timeline controller")
	}

	// mark action as done
	if err := jobActionsClient.PostEvents(ctx, jobRequestMessage.Plan.PlanID, jobRequestMessage.JobID, jobRequestMessage.RequestID); err != nil {
		logger.Error().Err(err).Msg("post events")
	}

	cancel()

	return nil
}

type JobController struct {
	timelineController *timeline.Controller
	CurrentTaskStack   []*runner.TaskDefinition // last in first out - currently active jobs
	QueueMain          []*runner.TaskDefinition
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

func (c *JobController) Run(ctx context.Context) error {
	logger := zerolog.Ctx(ctx)

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

	return nil
}

type Controller interface {
	AddTask(ctx context.Context, task runner.Task, parentID *string)
}
