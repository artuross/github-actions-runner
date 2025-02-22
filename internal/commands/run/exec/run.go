package exec

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"github.com/artuross/github-actions-runner/internal/repository/ghbroker"
	"github.com/artuross/github-actions-runner/internal/runnerconfig"
	"github.com/kr/pretty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
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

			fmt.Println("channel drained")
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

	// report timeline to server
	// TODO: send items to server
	wg.Add(1)
	go func() {
		defer wg.Done()

		syncedTimelineItems := make([]TimelineItem, 0)

		// create initial timeline
		timelineItems := make([]TimelineItem, 0)

		timelineItems = append(timelineItems, TimelineItem{
			ID:          jobRequestMessage.JobID,
			ParentID:    nil,
			Type:        TypeJob,
			Name:        jobRequestMessage.JobDisplayName,
			ContextName: jobRequestMessage.JobName,
			State:       StatePending,
			Order:       1,
		})

		for index, step := range jobRequestMessage.Steps {
			timelineItems = append(timelineItems, TimelineItem{
				ID:          step.ID,
				ParentID:    &jobRequestMessage.JobID,
				Type:        TypeStep,
				Name:        step.Name,
				ContextName: step.ContextName,
				State:       StatePending,
				Order:       index + 1,
			})
		}

		sortTimelineItems := func() {
			slices.SortFunc(timelineItems, func(a, b TimelineItem) int {
				// items without parents are top level items (beginning)
				if a.ParentID == nil && b.ParentID != nil {
					return -1
				}

				if a.ParentID != nil && b.ParentID == nil {
					return 1
				}

				// A's parent is B
				if a.ParentID != nil && *a.ParentID == b.ID {
					return -1
				}

				// B's parent is A
				if b.ParentID != nil && *b.ParentID == a.ID {
					return 1
				}

				// otherwise sort by order
				if a.Order < b.Order {
					return -1
				}

				if a.Order > b.Order {
					return 1
				}

				return 0
			})
		}

		sortTimelineItems()

		logger.Debug().Any("timeline", timelineItems).Msg("initial timeline")

		_ = syncedTimelineItems

		jobController := &JobController{
			CurrentTaskStack: []*TaskDefinition{},
			QueueMain: []*TaskDefinition{
				{
					ID:       jobRequestMessage.JobID,
					ParentID: nil,
					Task:     &Job{},
				},
				{
					ID:       "pre",
					ParentID: nil,
					Task:     &PreTaskRunner{steps: jobRequestMessage.Steps},
				},
			},
		}

		for len(jobController.QueueMain) > 0 {
			task := jobController.QueueMain[0]
			jobController.QueueMain = jobController.QueueMain[1:]

			jobController.CurrentTaskStack = append(jobController.CurrentTaskStack, task)

			if runnable, ok := task.Task.(Runnable); ok {
				logger.Debug().Str("task_id", task.ID).Msg("running task in controller")

				if err := runnable.Run(ctx, jobController); err != nil {
					logger.Error().Err(err).Msg("run task")
					continue
				}

				time.Sleep(2 * time.Second)

				continue
			}

			logger.Debug().Str("task_id", task.ID).Type("type", task.Task).Msg("skipping task in controller")

			// TODO: remove from stack
		}
	}()

	pretty.Println(jobRequestMessage.Steps)

	return nil
}

type TaskDefinition struct {
	ID       string
	ParentID *string
	Task     Task
}

type JobController struct {
	CurrentTaskStack []*TaskDefinition // last in first out - currently active jobs
	QueueMain        []*TaskDefinition
}

func (c *JobController) AddTask(ctx context.Context, task Task, parentID *string) {
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

	queue := make([]*TaskDefinition, 0, len(c.QueueMain)+1)
	queue = append(queue, c.QueueMain[:insertIndex]...)
	queue = append(queue, &TaskDefinition{
		ID:       task.ID(),
		ParentID: parentID,
		Task:     task,
	})
	queue = append(queue, c.QueueMain[insertIndex:]...)

	c.QueueMain = queue

	zerolog.Ctx(ctx).Error().Any("arr", c.QueueMain).Msg("post")
}

type Controller interface {
	AddTask(ctx context.Context, task Task, parentID *string)
}

type Runnable interface {
	Run(ctx context.Context, ctrl Controller) error
}

type (
	CompositeTaskRunner struct {
		id   string
		name string
	}

	Job struct {
		id   string
		name string
	}

	PreTaskRunner struct {
		id    string
		name  string
		steps []ghapi.Step
	}

	SimpleTaskRunner struct {
		id   string
		name string
	}
)

type Task interface {
	ID() string
	Name() string
	Type() string
}

func (j *CompositeTaskRunner) ID() string { return j.id }
func (j *Job) ID() string                 { return j.id }
func (j *PreTaskRunner) ID() string       { return j.id }
func (j *SimpleTaskRunner) ID() string    { return j.id }

func (j *CompositeTaskRunner) Name() string { return j.name }
func (j *Job) Name() string                 { return j.name }
func (j *PreTaskRunner) Name() string       { return j.name }
func (j *SimpleTaskRunner) Name() string    { return j.name }

func (j *Job) Type() string                 { return "job" }
func (t *CompositeTaskRunner) Type() string { return "action" }
func (t *PreTaskRunner) Type() string       { return "task" }
func (t *SimpleTaskRunner) Type() string    { return "task" }

func (r *PreTaskRunner) Run(ctx context.Context, ctrl Controller) error {
	logger := log.Ctx(ctx)

	logger.Debug().Msg("running pre task")
	defer logger.Debug().Msg("completed pre task")

	toProcess := make([]StepWithParentID, 0)
	for _, step := range r.steps {
		toProcess = append(toProcess, StepWithParentID{
			Step: step,
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
		if current.Step.Name == "__run_3" || current.Step.ID == "f5002f43-49c5-5559-6892-ae598c073a1e_2" {
			current.Step.Type = "composite"
		}

		var task Task
		if current.Step.Type == "composite" {
			task = &CompositeTaskRunner{
				id:   current.Step.ID,
				name: current.Step.Name,
			}
		} else {
			task = &SimpleTaskRunner{
				id:   current.Step.ID,
				name: current.Step.Name,
			}
		}

		// adds task to processing queue
		// add runner, not a step
		ctrl.AddTask(ctx, task, current.ParentID)

		// TODO: remove
		if current.Step.Name == "__run_3" {
			toProcess = append(
				toProcess,
				fakeChildStep(current.Step.ID, 1),
				fakeChildStep(current.Step.ID, 2),
			)

			continue
		}

		if current.Step.ID == "f5002f43-49c5-5559-6892-ae598c073a1e_2" {
			toProcess = append(
				toProcess,
				fakeChildStep(current.Step.ID, 1),
				fakeChildStep(current.Step.ID, 2),
				fakeChildStep(current.Step.ID, 3),
			)

			continue
		}
	}

	return nil
}

func (r *SimpleTaskRunner) Run(ctx context.Context, _ Controller) error {
	logger := log.Ctx(ctx)

	logger.Debug().Msg("running simple task")
	defer logger.Debug().Msg("completed simple task")

	return nil
}

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

type StepWithParentID struct {
	Step     ghapi.Step
	ParentID *string
}

type TimelineItem struct {
	ID          string
	ParentID    *string
	Type        Type
	Name        string
	ContextName string
	State       State
	Order       int
}

type Type string

const (
	TypeJob  Type = "job"
	TypeStep Type = "step"
)

type State string

const (
	StatePending   State = "pending"
	StateRunning   State = "running"
	StateCompleted State = "completed"
)

type StateBase struct {
	ID               string
	ParentID         *string
	Name             string
	StartTime        *time.Time
	FinishTime       *time.Time
	CurrentOperation string // ??
	PercentComplete  int
	State            string
	Result           string
	ResultCode       struct{} // ??
	ChangeID         int64
	LastModified     time.Time
	WorkerName       string
	Order            int
	RefName          string
	Log              *struct {
		ID       int64
		Location *string
	}
	Details         struct{}            // ??
	ErrorCount      int                 // ??
	WarningCount    int                 // ??
	NoticeCount     int                 // ??
	Issues          []struct{}          // ??
	Variables       map[string]struct{} // ??
	Location        string
	PreviousAttemps []struct{} // ??
	Attempt         int
	Identifier      *string // ??
}

type (
	StateTask struct {
		StateBase
	}

	StateJob struct {
		StateBase
		AgentPlatform string
	}

	StateStage struct {
		StateBase
	}

	StatePhase struct {
		StateBase
	}
)

type TimelineController struct{}
