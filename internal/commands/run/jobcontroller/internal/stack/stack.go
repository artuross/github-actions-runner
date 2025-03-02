package stack

import (
	"context"
	"io"

	"github.com/artuross/github-actions-runner/internal/commands/run/step"
	"go.opentelemetry.io/otel/trace"
)

type StepContext struct {
	Step              step.Step
	Ctx               context.Context
	Span              trace.Span
	TimelineLogWriter io.WriteCloser
	ResultsLogWriter  io.WriteCloser
}

type ExecutionContext struct {
	ctx   context.Context
	stack []*StepContext
}

func NewExecutionContext(ctx context.Context) ExecutionContext {
	return ExecutionContext{
		ctx:   ctx,
		stack: make([]*StepContext, 0),
	}
}

func (ec *ExecutionContext) Empty() bool {
	return len(ec.stack) == 0
}

func (ec *ExecutionContext) Peek() *StepContext {
	if len(ec.stack) == 0 {
		return nil
	}

	return ec.stack[len(ec.stack)-1]
}

func (ec *ExecutionContext) Pop() *StepContext {
	if len(ec.stack) == 0 {
		return nil
	}

	last := ec.stack[len(ec.stack)-1]
	ec.stack = ec.stack[:len(ec.stack)-1]

	return last
}

func (ec *ExecutionContext) Push(ctx context.Context, step step.Step, span trace.Span, timelineLogWriter, resultsLogWriter io.WriteCloser) {
	ec.stack = append(ec.stack, &StepContext{
		Step:              step,
		Ctx:               ctx,
		Span:              span,
		TimelineLogWriter: timelineLogWriter,
		ResultsLogWriter:  resultsLogWriter,
	})
}

func (ec *ExecutionContext) Context() context.Context {
	if len(ec.stack) == 0 {
		return ec.ctx
	}

	return ec.stack[len(ec.stack)-1].Ctx
}

func (ec *ExecutionContext) LogWriters() []io.Writer {
	if len(ec.stack) == 0 {
		return nil
	}

	writers := make([]io.Writer, 0, len(ec.stack))
	for _, writer := range ec.stack {
		writers = append(writers, writer.TimelineLogWriter, writer.ResultsLogWriter)
	}

	return writers
}
