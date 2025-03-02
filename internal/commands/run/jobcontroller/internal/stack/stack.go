package stack

import (
	"context"
	"io"

	"github.com/artuross/github-actions-runner/internal/commands/run/log/windowedbuffer"
	"github.com/artuross/github-actions-runner/internal/commands/run/step"
	"go.opentelemetry.io/otel/trace"
)

type LogWriter interface {
	io.WriteCloser
	WindowedBuffer() *windowedbuffer.WindowedBuffer
}

type StepContext struct {
	Step              step.Step
	Ctx               context.Context
	Span              trace.Span
	LogWriter         LogWriter
	ResultsLogReader  io.Closer
	TimelineLogReader io.Closer
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

func (ec *ExecutionContext) Push(ctx context.Context, step step.Step, span trace.Span, logWriter LogWriter, resultsLogReader, timelineLogReader io.Closer) {
	ec.stack = append(ec.stack, &StepContext{
		Step:              step,
		Ctx:               ctx,
		Span:              span,
		LogWriter:         logWriter,
		ResultsLogReader:  resultsLogReader,
		TimelineLogReader: timelineLogReader,
	})
}

func (ec *ExecutionContext) Context() context.Context {
	if len(ec.stack) == 0 {
		return ec.ctx
	}

	return ec.stack[len(ec.stack)-1].Ctx
}
