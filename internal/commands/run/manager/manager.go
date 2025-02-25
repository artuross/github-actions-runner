package manager

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"golang.org/x/sync/errgroup"
)

type Status string

const (
	StatusOnline Status = "Online"
	StatusBusy   Status = "Busy"
)

type Listener interface {
	// TODO: get state should be method
	Run(ctx context.Context, getState func() State, startJob func(jobRequest ghapi.MessageRunnerJobRequest) error) error
}

type Worker interface {
	Run(ctx context.Context, jobRequest ghapi.MessageRunnerJobRequest) error
}

type State struct {
	Status Status
}

type Manager struct {
	mu       sync.Mutex
	state    State
	listener Listener
	worker   Worker
}

func New(listener Listener, worker Worker) *Manager {
	return &Manager{
		mu: sync.Mutex{},
		state: State{
			Status: StatusOnline,
		},
		listener: listener,
		worker:   worker,
	}
}

func (m *Manager) GetState() State {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.state
}

func (m *Manager) SetBusy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.Status == StatusBusy {
		return false
	}

	m.state.Status = StatusBusy
	return true
}

func (m *Manager) SetOnline() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.Status = StatusOnline
}

func (m *Manager) Run(ctx context.Context) error {
	// TODO: probably don't need this?
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	group, ctx := errgroup.WithContext(ctx)

	// starts a worker with a job request
	startJob := func(jobRequest ghapi.MessageRunnerJobRequest) error {
		if updated := m.SetBusy(); !updated {
			return errors.New("agent is already busy")
		}

		group.Go(func() error {
			defer m.SetOnline()

			if err := m.worker.Run(ctx, jobRequest); err != nil {
				return fmt.Errorf("running worker with job: %w", err)
			}

			return nil
		})

		return nil
	}

	// starts a listener
	group.Go(func() error {
		if err := m.listener.Run(ctx, m.GetState, startJob); err != nil {
			return fmt.Errorf("running listener: %w", err)
		}

		return nil
	})

	if err := group.Wait(); err != nil {
		return fmt.Errorf("running manager: %w", err)
	}

	return nil
}
