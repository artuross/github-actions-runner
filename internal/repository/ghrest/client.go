package ghrest

import (
	"github.com/artuross/github-actions-runner/internal/defaults"
	"github.com/google/go-github/v69/github"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TODO: may want to do it via debug.ReadBuildInfo
	tracerName = "github.com/artuross/github-actions-runner/internal/repository/ghrest"
)

type Repository struct {
	tracer trace.Tracer
	client *github.Client
}

func New(client *github.Client, options ...func(*Repository)) *Repository {
	repository := Repository{
		client: client,
		tracer: defaults.TracerProvider.Tracer(tracerName),
	}

	for _, apply := range options {
		apply(&repository)
	}

	return &repository
}

func WithTracerProvider(tp trace.TracerProvider) func(*Repository) {
	return func(r *Repository) {
		r.tracer = tp.Tracer(tracerName)
	}
}
