package step

import (
	"context"
	"io"
)

type Preparer interface {
	Prepare(ctx context.Context, logWriter io.Writer) ([]Step, error)
}

type Runner interface {
	Run(ctx context.Context, logWriter io.Writer) error
}

type Step interface {
	Typer // TODO: remove
	ID() string
	ParentID() *string
	DisplayName() string
	RefName() string
}

// TODO: cleanup
type Typer interface {
	Type() string
}
