package workflowsteps

import (
	"context"
	"fmt"
	"time"

	"github.com/artuross/github-actions-runner/internal/commands/run/log/uploader"
)

// TODO: should be read from data
const linesInLogFile = 10

const LogFileMaxSize = 2 * 1024 * 1024 // 2MB

type blobStorage interface {
	UploadFile(ctx context.Context, url string, data []byte) error
}

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

func JobUploader(resultsClient jobResultsReceiver, blobStorageClient blobStorage, jobRunID, jobID string) uploader.UploadFunc {
	return func(ctx context.Context, chunk []byte, isFinal bool) error {
		blobUploadURL, err := resultsClient.GetJobLogSignedBlobURL(ctx, jobRunID, jobID)
		if err != nil {
			return fmt.Errorf("get step log blob upload URL: %w", err)
		}

		if err := blobStorageClient.UploadFile(ctx, blobUploadURL, chunk); err != nil {
			return fmt.Errorf("upload step log file: %w", err)
		}

		// TODO: calculate how many lines are in the log
		// TODO: time.Now() should be configurable
		if err := resultsClient.CreateJobLogsMetadata(ctx, jobRunID, jobID, linesInLogFile, time.Now()); err != nil {
			return fmt.Errorf("create step log metadata: %w", err)
		}

		return nil
	}
}

func StepUploader(resultsClient stepResultsReceiver, blobStorageClient blobStorage, jobRunID, jobID, stepID string) uploader.UploadFunc {
	return func(ctx context.Context, chunk []byte, isFinal bool) error {
		blobUploadURL, err := resultsClient.GetStepLogSignedBlobURL(ctx, jobRunID, jobID, stepID)
		if err != nil {
			return fmt.Errorf("get step log blob upload URL: %w", err)
		}

		if err := blobStorageClient.UploadFile(ctx, blobUploadURL, chunk); err != nil {
			return fmt.Errorf("upload step log file: %w", err)
		}

		// TODO: calculate how many lines are in the log
		// TODO: time.Now() should be configurable
		if err := resultsClient.CreateStepLogsMetadata(ctx, jobRunID, jobID, stepID, linesInLogFile, time.Now()); err != nil {
			return fmt.Errorf("create step log metadata: %w", err)
		}

		return nil
	}
}
