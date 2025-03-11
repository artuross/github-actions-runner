package workflowsteps_test

import (
	"context"
	"testing"

	"github.com/artuross/github-actions-runner/internal/commands/run/workflowsteps"
	"github.com/artuross/github-actions-runner/internal/commands/run/workflowsteps/fakes"
	"github.com/artuross/github-actions-runner/internal/util/timeutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestController(t *testing.T) {
	// TODO: add test case for error handling
	// TODO: add test case for step state transitions
	// TODO: add test case for context cancellation sync
	// TODO: verify sync payload

	const workflowJobRunID = "workflow_job_run_id"
	const workflowRunID = "workflow_run_id"

	t.Run("start & shutdown", func(t *testing.T) {
		fakeResultsReceiver := fakes.FakeResultsReceiver{}

		t.Run("with empty queue", func(t *testing.T) {
			// init
			ctrl := workflowsteps.NewController(&fakeResultsReceiver, workflowJobRunID, workflowRunID)

			// start
			ctrl.Start(context.Background())

			// shutdown
			err := ctrl.Shutdown(context.Background())
			require.NoError(t, err)
		})

		t.Run("with non empty queue", func(t *testing.T) {
			// init
			ctrl := workflowsteps.NewController(&fakeResultsReceiver, workflowJobRunID, workflowRunID)

			// start
			ctrl.Start(context.Background())

			// add steps
			ctrl.EventAddStep("step1", "parent1", "Step 1", 1)
			ctrl.EventAddStep("step2", "parent1", "Step 2", 2)
			ctrl.EventAddStep("step3", "parent1", "Step 3", 3)

			// shutdown
			err := ctrl.Shutdown(context.Background())
			require.NoError(t, err)
		})
	})

	t.Run("syncing", func(t *testing.T) {
		t.Run("before shutdown", func(t *testing.T) {
			t.Run("doesn't sync when queue is empty", func(t *testing.T) {
				fakeResultsReceiver := fakes.FakeResultsReceiver{}

				// init
				ctrl := workflowsteps.NewController(
					&fakeResultsReceiver,
					workflowJobRunID,
					workflowRunID,
					workflowsteps.WithNewTickerFunc(timeutil.WrapFakeTicker(timeutil.NewFakeTicker())),
				)

				// start
				ctrl.Start(context.Background())

				// shutdown
				err := ctrl.Shutdown(context.Background())
				require.NoError(t, err)

				// assert no calls
				assert.Equal(t, 0, fakeResultsReceiver.UpdateWorkflowStepsCallCount())
			})

			t.Run("syncs when queue is not empty", func(t *testing.T) {
				fakeResultsReceiver := fakes.FakeResultsReceiver{}

				// init
				ctrl := workflowsteps.NewController(
					&fakeResultsReceiver,
					workflowJobRunID,
					workflowRunID,
					workflowsteps.WithNewTickerFunc(timeutil.WrapFakeTicker(timeutil.NewFakeTicker())),
				)

				// start
				ctrl.Start(context.Background())

				ctrl.EventAddStep("step1", "parent1", "Step 1", 1)

				// shutdown
				err := ctrl.Shutdown(context.Background())
				require.NoError(t, err)

				// assert single call
				assert.Equal(t, 1, fakeResultsReceiver.UpdateWorkflowStepsCallCount())
			})
		})

		t.Run("on tick", func(t *testing.T) {
			t.Run("doesn't sync when queue is empty", func(t *testing.T) {
				ticker := timeutil.NewFakeTicker()
				fakeResultsReceiver := fakes.FakeResultsReceiver{}

				// init
				ctrl := workflowsteps.NewController(
					&fakeResultsReceiver,
					workflowJobRunID,
					workflowRunID,
					workflowsteps.WithNewTickerFunc(timeutil.WrapFakeTicker(ticker)),
				)

				// start
				ctrl.Start(context.Background())

				// shutdown
				err := ctrl.Shutdown(context.Background())
				require.NoError(t, err)

				// assert no calls
				assert.Equal(t, 0, fakeResultsReceiver.UpdateWorkflowStepsCallCount())
			})

			t.Run("syncs when queue is not empty", func(t *testing.T) {
				ticker := timeutil.NewFakeTicker()
				fakeResultsReceiver := fakes.FakeResultsReceiver{}

				eventProcessedChan := make(chan struct{})

				// init
				ctrl := workflowsteps.NewController(
					&fakeResultsReceiver,
					workflowJobRunID,
					workflowRunID,
					workflowsteps.WithHookEventProcessed(eventProcessedChan),
					workflowsteps.WithNewTickerFunc(timeutil.WrapFakeTicker(ticker)),
				)

				// start
				ctrl.Start(context.Background())
				assert.Zero(t, 0, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 0 (before sending events)")

				// add step and wait for it to be processed
				ctrl.EventAddStep("step1", "parent1", "Step 1", 1)
				<-eventProcessedChan
				assert.Zero(t, 0, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 0 (before tick)")

				// tick twice, 2nd tick MUST be performed after 1st tick guaranteeing that 1st tick already happened
				ticker.Tick()
				ticker.Tick()
				assert.Equal(t, 1, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 1 (after tick)")

				// shutdown
				err := ctrl.Shutdown(context.Background())
				require.NoError(t, err)

				// no more syncs
				assert.Equal(t, 1, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 1 (after shutdown)")
			})

			t.Run("syncs max 1 time per tick", func(t *testing.T) {
				ticker := timeutil.NewFakeTicker()
				fakeResultsReceiver := fakes.FakeResultsReceiver{}

				eventProcessedChan := make(chan struct{})

				// init
				ctrl := workflowsteps.NewController(
					&fakeResultsReceiver,
					workflowJobRunID,
					workflowRunID,
					workflowsteps.WithHookEventProcessed(eventProcessedChan),
					workflowsteps.WithNewTickerFunc(timeutil.WrapFakeTicker(ticker)),
				)

				// start
				ctrl.Start(context.Background())
				assert.Zero(t, 0, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 0 (before sending events)")

				// add steps
				ctrl.EventAddStep("step1", "parent1", "Step 1", 1)
				<-eventProcessedChan
				ctrl.EventAddStep("step2", "parent1", "Step 2", 2)
				<-eventProcessedChan
				ctrl.EventAddStep("step3", "parent1", "Step 3", 3)
				<-eventProcessedChan

				// tick twice, 2nd tick MUST be performed after 1st tick guaranteeing that 1st tick already happened
				ticker.Tick()
				ticker.Tick()
				assert.Equal(t, 1, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 1 (after tick)")

				// shutdown
				err := ctrl.Shutdown(context.Background())
				require.NoError(t, err)

				// no more syncs
				assert.Equal(t, 1, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 1 (after shutdown)")
			})

			t.Run("syncs after each tick", func(t *testing.T) {
				ticker := timeutil.NewFakeTicker()
				fakeResultsReceiver := fakes.FakeResultsReceiver{}

				eventProcessedChan := make(chan struct{})

				// init
				ctrl := workflowsteps.NewController(
					&fakeResultsReceiver,
					workflowJobRunID,
					workflowRunID,
					workflowsteps.WithHookEventProcessed(eventProcessedChan),
					workflowsteps.WithNewTickerFunc(timeutil.WrapFakeTicker(ticker)),
				)

				// start
				ctrl.Start(context.Background())
				assert.Zero(t, 0, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 0 (before sending events)")

				// add step, tick twice and assert
				ctrl.EventAddStep("step1", "parent1", "Step 1", 1)
				<-eventProcessedChan
				ticker.Tick()
				ticker.Tick()
				assert.Equal(t, 1, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 1 (after tick)")

				// add step, tick twice and assert
				ctrl.EventAddStep("step2", "parent1", "Step 2", 2)
				<-eventProcessedChan
				ticker.Tick()
				ticker.Tick()
				assert.Equal(t, 2, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 2 (after tick)")

				// add step, tick twice and assert
				ctrl.EventAddStep("step3", "parent1", "Step 3", 3)
				<-eventProcessedChan
				ticker.Tick()
				ticker.Tick()
				assert.Equal(t, 3, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 3 (after tick)")

				// shutdown
				err := ctrl.Shutdown(context.Background())
				require.NoError(t, err)

				// no more syncs
				assert.Equal(t, 3, fakeResultsReceiver.UpdateWorkflowStepsCallCount(), "should be 3 (after shutdown)")
			})
		})
	})
}
