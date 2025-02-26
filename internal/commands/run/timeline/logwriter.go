package timeline

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/artuross/github-actions-runner/internal/repository/ghactions"
)

type LogWriter struct {
	ctx        context.Context
	planID     string
	recordID   ID
	client     *ghactions.Repository
	controller *Controller
	buffer     *bytes.Buffer
	lineBuffer *bytes.Buffer // New buffer for incomplete lines
	uploads    chan []byte
	done       chan struct{}
	wg         sync.WaitGroup
	mu         sync.Mutex
}

const maxBufferSize = 2 * 1024 * 1024 // 2MB

func NewLogWriter(ctx context.Context, client *ghactions.Repository, controller *Controller, planID string, recordID ID) *LogWriter {
	w := &LogWriter{
		ctx:        ctx,
		planID:     planID,
		recordID:   recordID,
		client:     client,
		controller: controller,
		buffer:     bytes.NewBuffer(make([]byte, 0, maxBufferSize)),
		lineBuffer: bytes.NewBuffer(nil), // Initialize line buffer
		uploads:    make(chan []byte, 10),
		done:       make(chan struct{}),
	}

	w.wg.Add(1)
	go w.uploadWorker()

	return w
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// First write to lineBuffer
	n, err = w.lineBuffer.Write(p)
	if err != nil {
		return n, err
	}

	// Process complete lines
	for {
		line, err := w.lineBuffer.ReadBytes('\n')
		if err != nil {
			// No complete line found, put data back in lineBuffer
			w.lineBuffer.Write(line)
			break
		}

		_, err = w.buffer.Write(line)
		if err != nil {
			return n, err
		}

		// Check if main buffer exceeds max size
		if w.buffer.Len() >= maxBufferSize {
			// Copy buffer contents
			chunk := make([]byte, w.buffer.Len())
			copy(chunk, w.buffer.Bytes())

			// Reset buffer
			w.buffer.Reset()

			// Queue upload
			select {
			case w.uploads <- chunk:
			default:
				return n, fmt.Errorf("upload queue is full")
			}
		}
	}

	return n, nil
}

func (w *LogWriter) Close() error {
	w.mu.Lock()

	// Handle any remaining data in lineBuffer
	if w.lineBuffer.Len() > 0 {
		// If there's an incomplete line at the end, add a newline
		w.buffer.Write(w.lineBuffer.Bytes())
		w.buffer.WriteByte('\n')
		w.lineBuffer.Reset()
	}

	// Send remaining buffer if not empty
	if w.buffer.Len() > 0 {
		chunk := make([]byte, w.buffer.Len())
		copy(chunk, w.buffer.Bytes())
		w.uploads <- chunk
	}
	w.mu.Unlock()

	// Signal worker to stop
	close(w.done)

	// Wait for uploads to complete
	w.wg.Wait()

	return nil
}

// uploadWorker and other methods remain the same
func (w *LogWriter) uploadWorker() {
	defer w.wg.Done()

	for {
		select {
		case <-w.done:
			// drain channel
			for {
				select {
				case chunk := <-w.uploads:
					chunkID, err := w.uploadChunk(chunk)
					if err != nil {
						fmt.Printf("Error uploading chunk: %v\n", err)
					}

					w.controller.RecordLogUploaded(w.recordID, chunkID)

				default:
					return
				}
			}

		case chunk := <-w.uploads:
			// TODO: fix error handling
			chunkID, err := w.uploadChunk(chunk)
			if err != nil {
				fmt.Printf("Error uploading chunk: %v\n", err)
			}

			w.controller.RecordLogUploaded(w.recordID, chunkID)
		}
	}
}

func (w *LogWriter) uploadChunk(chunk []byte) (int64, error) {
	chunkID, err := w.getUploadID(w.ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get upload path: %w", err)
	}

	chunkID, err = w.uploadLogs(w.ctx, chunkID, chunk)
	if err != nil {
		return 0, fmt.Errorf("failed to upload chunk: %w", err)
	}

	return chunkID, nil
}

func (w *LogWriter) getUploadID(ctx context.Context) (int64, error) {
	metadata, err := w.client.PostLogMetadata(ctx, w.planID, string(w.recordID))
	if err != nil {
		return 0, fmt.Errorf("fetch log metadata: %w", err)
	}

	return metadata.ID, nil
}

func (w *LogWriter) uploadLogs(ctx context.Context, path int64, chunk []byte) (int64, error) {
	metadata, err := w.client.PostLogs(ctx, w.planID, path, chunk)
	if err != nil {
		return 0, fmt.Errorf("upload logs: %w", err)
	}

	return metadata.ID, nil
}
