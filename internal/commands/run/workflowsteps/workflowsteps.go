package workflowsteps

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/artuross/github-actions-runner/internal/repository/resultsreceiver"
	"github.com/kr/pretty"
)

type Update any

type UpdateCompleted struct {
	ID          string
	Status      resultsreceiver.Status
	Conclusion  resultsreceiver.Conclusion
	CompletedAt time.Time
}

type Controller struct {
	resultsClient    *resultsreceiver.Repository
	workflowJobRunID string
	workflowRunID    string
	nextChangeID     int

	shutdownSignal chan struct{}
	wg             sync.WaitGroup

	updatesChan chan Update
	state       []resultsreceiver.Step
	unsyncedIDs []string
}

func NewController(resultsClient *resultsreceiver.Repository, workflowJobRunID, workflowRunID string) *Controller {
	return &Controller{
		resultsClient:    resultsClient,
		workflowJobRunID: workflowJobRunID,
		workflowRunID:    workflowRunID,

		shutdownSignal: make(chan struct{}),
		wg:             sync.WaitGroup{},

		updatesChan: make(chan Update),
		state:       make([]resultsreceiver.Step, 0),
		unsyncedIDs: make([]string, 0),
	}
}

func (c *Controller) SendUpdate(update Update) {
	c.updatesChan <- update
}

func (c *Controller) Start(ctx context.Context) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		// create a timer and stop it right away
		isSet := false
		timer := time.NewTimer(time.Hour)
		if !timer.Stop() {
			<-timer.C
		}

		processUpdate := func(update Update) {
			switch update := update.(type) {
			case resultsreceiver.Step:
				if !isSet {
					isSet = true
					timer.Reset(100 * time.Millisecond)
					c.nextChangeID++
				}

				c.state = append(c.state, update)

				pretty.Println(c.state)

				if slices.ContainsFunc(c.state, stepWithID(update.ExternalID)) {
					c.unsyncedIDs = append(c.unsyncedIDs, update.ExternalID)
				}

			case UpdateCompleted:
				if !isSet {
					isSet = true
					timer.Reset(100 * time.Millisecond)
					c.nextChangeID++
				}

				index := slices.IndexFunc(c.state, stepWithID(update.ID))
				if index == -1 {
					return
				}

				step := c.state[index]

				step.Status = update.Status
				step.Conclusion = update.Conclusion
				step.CompletedAt = &update.CompletedAt

				c.state[index] = step

				if slices.ContainsFunc(c.state, stepWithID(update.ID)) {
					c.unsyncedIDs = append(c.unsyncedIDs, update.ID)
				}

			default:
				fmt.Println("Unexpected update type:", update)
			}
		}

		sendToServer := func() {
			// mark as timer not set
			isSet = false

			steps := make([]resultsreceiver.Step, 0, len(c.unsyncedIDs))
			for _, step := range c.state {
				if !slices.Contains(c.unsyncedIDs, step.ExternalID) {
					continue
				}

				steps = append(steps, step)
			}

			if len(steps) == 0 {
				return
			}

			if err := c.resultsClient.UpdateWorkflowSteps(ctx, c.nextChangeID, c.workflowJobRunID, c.workflowRunID, steps); err != nil {
				fmt.Println("Failed to sync steps:", err)

				isSet = true
				timer.Reset(100 * time.Millisecond)

				return
			}

			c.unsyncedIDs = make([]string, 0)
		}

		for {
			select {
			case <-c.shutdownSignal:
			loop:
				for {
					select {
					case update := <-c.updatesChan:
						processUpdate(update)

					default:
						break loop
					}
				}

				isSet = false
				if isSet && !timer.Stop() {
					<-timer.C
				}

				sendToServer()

				// exit
				return

			case update := <-c.updatesChan:
				processUpdate(update)

			case <-timer.C:
				// mark as timer not set
				isSet = false

				sendToServer()
			}
		}
	}()
}

func (c *Controller) Shutdown(ctx context.Context) error {
	// close channel to notify goroutine it's time to exit
	close(c.shutdownSignal)

	// wait for goroutine to finish
	c.wg.Wait()

	return nil
}

func stepWithID(id string) func(resultsreceiver.Step) bool {
	return func(step resultsreceiver.Step) bool {
		return step.ExternalID == id
	}
}
