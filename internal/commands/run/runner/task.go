package runner

import (
	"context"
)

type controller interface {
	AddTask(ctx context.Context, task Task, parentID *string, name, refName string)
}

// TODO: rename to RunnerDefinition?
type TaskDefinition struct {
	ID       string
	ParentID *string
	Task     Task
}

type Task interface {
	ID() string
	Name() string
	Type() string
}
