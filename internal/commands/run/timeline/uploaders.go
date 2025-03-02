package timeline

import (
	"context"
	"fmt"

	"github.com/artuross/github-actions-runner/internal/commands/run/log/uploader"
	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
)

type timeline interface {
	RecordLogUploaded(id string, logID int64)
}

type actionsService interface {
	PostLogMetadata(ctx context.Context, planID string, taskID string) (*ghactions.LogMetadata, error)
	PostLogs(ctx context.Context, planID string, logID int64, data []byte) (*ghactions.LogMetadata, error)
}

func LogUploader(timeline timeline, actionsService actionsService, workflowJobRunID, recordID string) uploader.UploadFunc {
	return func(ctx context.Context, chunk []byte, isFinal bool) error {
		metadata, err := actionsService.PostLogMetadata(ctx, workflowJobRunID, recordID)
		if err != nil {
			return fmt.Errorf("fetch log metadata: %w", err)
		}

		metadata, err = actionsService.PostLogs(ctx, workflowJobRunID, metadata.ID, chunk)
		if err != nil {
			return fmt.Errorf("upload logs: %w", err)
		}

		timeline.RecordLogUploaded(recordID, metadata.ID)

		return nil
	}
}
