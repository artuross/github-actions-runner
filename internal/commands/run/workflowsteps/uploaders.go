package workflowsteps

import (
	"context"
	"fmt"
	"time"

	"github.com/artuross/github-actions-runner/internal/commands/run/log/uploader"
)

const LogFileMaxSize = 2 * 1024 * 1024 // 2MB

type blobStorage interface {
	UploadFile(ctx context.Context, url string, data []byte) error
}

type (
	JobUploaderFunc  func(jobRunID, jobID string) uploader.UploadFunc
	StepUploaderFunc func(jobRunID, jobID, stepID string) uploader.UploadFunc
)

type (
	jobResultsReceiver interface {
		CreateJobLogsMetadata(ctx context.Context, workflowRunID, workflowJobRunID string, lines int, uploadedAt time.Time) error
		GetJobLogSignedBlobURL(ctx context.Context, workflowRunID, workflowJobRunID string) (string, error)
	}

	stepResultsReceiver interface {
		CreateStepLogsMetadata(ctx context.Context, workflowRunID, workflowJobRunID, stepID string, lines int, uploadedAt time.Time) error
		GetStepLogSignedBlobURL(ctx context.Context, workflowRunID, workflowJobRunID, stepID string) (string, error)
	}
)

func JobUploader(resultsClient jobResultsReceiver, blobStorageClient blobStorage) JobUploaderFunc {
	return func(jobRunID, jobID string) uploader.UploadFunc {
		return func(ctx context.Context, chunk []byte, isFinal bool) error {
			blobUploadURL, err := resultsClient.GetJobLogSignedBlobURL(ctx, jobRunID, jobID)
			if err != nil {
				return fmt.Errorf("get step log blob upload URL: %w", err)
			}

			if err := blobStorageClient.UploadFile(ctx, blobUploadURL, chunk); err != nil {
				return fmt.Errorf("upload step log file: %w", err)
			}

			lines := 0
			for _, char := range chunk {
				if char == '\n' {
					lines++
				}
			}

			// TODO: time.Now() should be configurable
			if err := resultsClient.CreateJobLogsMetadata(ctx, jobRunID, jobID, lines, time.Now()); err != nil {
				return fmt.Errorf("create step log metadata: %w", err)
			}

			return nil
		}
	}
}

func StepUploader(resultsClient stepResultsReceiver, blobStorageClient blobStorage) StepUploaderFunc {
	return func(jobRunID, jobID, stepID string) uploader.UploadFunc {
		return func(ctx context.Context, chunk []byte, isFinal bool) error {
			blobUploadURL, err := resultsClient.GetStepLogSignedBlobURL(ctx, jobRunID, jobID, stepID)
			if err != nil {
				return fmt.Errorf("get step log blob upload URL: %w", err)
			}

			if err := blobStorageClient.UploadFile(ctx, blobUploadURL, chunk); err != nil {
				return fmt.Errorf("upload step log file: %w", err)
			}

			lines := 0
			for _, char := range chunk {
				if char == '\n' {
					lines++
				}
			}

			// TODO: time.Now() should be configurable

			if err := resultsClient.CreateStepLogsMetadata(ctx, jobRunID, jobID, stepID, lines, time.Now()); err != nil {
				return fmt.Errorf("create step log metadata: %w", err)
			}

			return nil
		}
	}
}
