package uploader

import (
	"context"
	"io"

	"github.com/rs/zerolog"
)

type Reader interface {
	ReadLines(maxLength int) ([]byte, error)
}

type UploadFunc func(ctx context.Context, data []byte, isFinal bool) error

type LogUploader struct {
	reader    Reader
	chunkSize int
	upload    UploadFunc
}

func New(reader Reader, chunkSize int, upload UploadFunc) *LogUploader {
	return &LogUploader{
		reader:    reader,
		chunkSize: chunkSize,
		upload:    upload,
	}
}

func (u *LogUploader) Start(ctx context.Context) error {
	logger := zerolog.Ctx(ctx)

	for {
		chunk, err := u.reader.ReadLines(u.chunkSize)
		if err != nil && err != io.ErrClosedPipe {
			logger.Error().Err(err).Msg("failed to read lines")
			return err
		}

		if len(chunk) > 0 {
			isFinal := err == io.ErrClosedPipe
			if err := u.upload(ctx, chunk, isFinal); err != nil {
				logger.Error().Err(err).Msg("failed to upload chunk")
				return err
			}
		}

		if err == io.ErrClosedPipe {
			return nil
		}
	}
}
