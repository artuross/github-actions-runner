package manager

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/artuross/github-actions-runner/internal/defaults"
	"github.com/artuross/github-actions-runner/internal/repository/ghapi"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

const (
	// TODO: may want to do it via debug.ReadBuildInfo
	tracerName = "github.com/artuross/github-actions-runner/internal/commands/run/manager"
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
	tracer   trace.Tracer
}

func New(listener Listener, worker Worker, options ...func(*Manager)) *Manager {
	manager := Manager{
		mu: sync.Mutex{},
		state: State{
			Status: StatusOnline,
		},
		listener: listener,
		worker:   worker,
		tracer:   defaults.TracerProvider.Tracer(tracerName),
	}

	for _, apply := range options {
		apply(&manager)
	}

	return &manager
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
	ctx, span := m.tracer.Start(ctx, "run manager")
	defer span.End()

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

			ctx, span := m.tracer.Start(ctx, "run job")
			defer span.End()

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

func WithTracerProvider(tp trace.TracerProvider) func(*Manager) {
	return func(r *Manager) {
		r.tracer = tp.Tracer(tracerName)
	}
}
