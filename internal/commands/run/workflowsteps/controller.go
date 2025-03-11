package workflowsteps

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/artuross/github-actions-runner/internal/repository/resultsreceiver"
	"github.com/artuross/github-actions-runner/internal/util/timeutil"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.11.2 -o fakes . ResultsReceiver
type ResultsReceiver interface {
	UpdateWorkflowSteps(ctx context.Context, changeID int, workflowRunID, workflowJobRunID string, steps []resultsreceiver.Step) error
}

type event any

type (
	eventAddStep struct {
		ID          string
		Order       int
		DisplayName string
	}

	eventStepCompleted struct {
		ID          string
		Conclusion  resultsreceiver.Conclusion
		CompletedAt time.Time
	}

	eventStepInProgress struct {
		ID        string
		StartedAt time.Time
	}
)

type Controller struct {
	resultsClient    ResultsReceiver
	workflowJobRunID string
	workflowRunID    string
	nextChangeID     int
	mu               sync.Mutex
	wg               sync.WaitGroup
	shutdown         context.CancelFunc
	eventsChan       chan event
	state            []resultsreceiver.Step
	pendingSync      map[string]struct{}
	newTicker        timeutil.NewTickerFunc

	hookEventProcessed chan<- struct{}
}

func NewController(
	resultsClient ResultsReceiver,
	workflowJobRunID,
	workflowRunID string,
	options ...func(*Controller),
) *Controller {
	ctrl := Controller{
		resultsClient:      resultsClient,
		workflowJobRunID:   workflowJobRunID,
		workflowRunID:      workflowRunID,
		mu:                 sync.Mutex{},
		wg:                 sync.WaitGroup{},
		state:              make([]resultsreceiver.Step, 0),
		eventsChan:         make(chan event),
		pendingSync:        make(map[string]struct{}),
		shutdown:           nil,
		newTicker:          timeutil.Generic(timeutil.NewTicker),
		hookEventProcessed: nil,
	}

	for _, apply := range options {
		apply(&ctrl)
	}

	return &ctrl
}

func (c *Controller) EventAddStep(id string, _ string, displayName string, order int) {
	c.eventsChan <- eventAddStep{
		ID:          id,
		Order:       order,
		DisplayName: displayName,
	}
}

func (c *Controller) EventStatusCompleted(id string, completedAt time.Time, conclusion resultsreceiver.Conclusion) {
	c.eventsChan <- eventStepCompleted{
		ID:          id,
		Conclusion:  conclusion,
		CompletedAt: completedAt,
	}
}

func (c *Controller) EventStatusInProgress(id string, startedAt time.Time) {
	c.eventsChan <- eventStepInProgress{
		ID:        id,
		StartedAt: startedAt,
	}
}

func (c *Controller) Shutdown(_ context.Context) error {
	// notify goroutines time to exit
	c.shutdown()

	// wait for goroutine to finish
	c.wg.Wait()

	return nil
}

func (c *Controller) Start(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// sync worker must exit after the events worker to ensure all events have been synced
	// therefore sync worker gets a clean context which is cancelled when worker exits

	eventsCtx, eventCancel := context.WithCancel(ctx)
	syncCtx, syncCancel := context.WithCancel(context.WithoutCancel(ctx))

	// for Shutdown method
	c.shutdown = eventCancel

	c.wg.Add(1)
	go c.workerEvents(eventsCtx, syncCancel)

	c.wg.Add(1)
	go c.workerSync(syncCtx)
}

func (c *Controller) doSync(ctx context.Context) error {
	c.mu.Lock()
	if len(c.pendingSync) == 0 {
		c.mu.Unlock()
		return nil
	}

	// collect ids to sync
	ids := make([]string, 0, len(c.pendingSync))
	for id := range c.pendingSync {
		ids = append(ids, id)
	}

	// clear list
	c.pendingSync = make(map[string]struct{})

	// collect steps to sync
	steps := make([]resultsreceiver.Step, 0, len(ids))
	for _, id := range ids {
		index := slices.IndexFunc(c.state, stepWithID(id))
		if index == -1 {
			continue
		}

		step := c.state[index]
		steps = append(steps, step)
	}
	c.mu.Unlock()

	c.mu.Lock()
	c.nextChangeID++
	nextChangeID := c.nextChangeID
	c.mu.Unlock()

	// TODO: add backoff in case of failure
	if err := c.resultsClient.UpdateWorkflowSteps(ctx, nextChangeID, c.workflowJobRunID, c.workflowRunID, steps); err != nil {
		c.mu.Lock()
		defer c.mu.Unlock()

		// mark all items as needing sync
		for _, id := range ids {
			c.pendingSync[id] = struct{}{}
		}

		return fmt.Errorf("update workflow steps: %w", err)
	}

	return nil
}

func (c *Controller) handleEvent(evt event) {
	// for testing
	if c.hookEventProcessed != nil {
		defer func() {
			c.hookEventProcessed <- struct{}{}
		}()
	}

	switch evt := evt.(type) {
	case eventAddStep:
		step := resultsreceiver.Step{
			ExternalID: evt.ID,
			Number:     evt.Order,
			Name:       evt.DisplayName,
			Status:     resultsreceiver.StatusPending,
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		c.state = append(c.state, step)
		c.pendingSync[evt.ID] = struct{}{}

	case eventStepCompleted:
		c.updateStep(evt.ID, func(step resultsreceiver.Step) resultsreceiver.Step {
			step.Status = resultsreceiver.StatusCompleted
			step.Conclusion = evt.Conclusion
			step.CompletedAt = &evt.CompletedAt

			return step
		})

	case eventStepInProgress:
		c.updateStep(evt.ID, func(step resultsreceiver.Step) resultsreceiver.Step {
			step.Status = resultsreceiver.StatusInProgress
			step.StartedAt = &evt.StartedAt

			return step
		})
	}
}

func (c *Controller) updateStep(id string, callback func(step resultsreceiver.Step) resultsreceiver.Step) {
	c.mu.Lock()
	defer c.mu.Unlock()

	index := slices.IndexFunc(c.state, stepWithID(id))
	if index == -1 {
		return
	}

	step := c.state[index]
	c.state[index] = callback(step)
	c.pendingSync[id] = struct{}{}
}

func (c *Controller) workerEvents(ctx context.Context, done func()) {
	defer c.wg.Done()
	defer done()

	for {
		select {
		case <-ctx.Done():
			return

		case evt := <-c.eventsChan:
			c.handleEvent(evt)
		}
	}
}

func (c *Controller) workerSync(ctx context.Context) {
	defer c.wg.Done()

	ticker := c.newTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// do one more sync before exiting
			// clean ctx, otherwise request will fail
			ctx := context.WithoutCancel(ctx)

			// TODO: handle error
			_ = c.doSync(ctx)

			return

		case <-ticker.Chan():
			// TODO: handle error
			_ = c.doSync(ctx)
		}
	}
}

func stepWithID(id string) func(resultsreceiver.Step) bool {
	return func(step resultsreceiver.Step) bool {
		return step.ExternalID == id
	}
}

func WithHookEventProcessed(ch chan<- struct{}) func(*Controller) {
	return func(c *Controller) {
		c.hookEventProcessed = ch
	}
}

func WithNewTickerFunc[T timeutil.Ticker](newTicker func(d time.Duration) T) func(*Controller) {
	return func(c *Controller) {
		c.newTicker = timeutil.Generic(newTicker)
	}
}
