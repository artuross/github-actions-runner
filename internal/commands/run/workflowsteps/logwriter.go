package workflowsteps

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/artuross/github-actions-runner/internal/repository/resultsreceiver"
	"github.com/rs/zerolog"
)

type LogWriter struct {
	ctx              context.Context
	workflowRunID    string
	workflowJobRunID string
	stepID           string
	client           *resultsreceiver.Repository
	buffer           *bytes.Buffer
	lineBuffer       *bytes.Buffer // New buffer for incomplete lines
	uploads          chan []byte
	done             chan struct{}
	wg               sync.WaitGroup
	mu               sync.Mutex
}

const maxBufferSize = 2 * 1024 * 1024 // 2MB

func NewLogWriter(ctx context.Context, client *resultsreceiver.Repository, workflowRunID, workflowJobRunID, stepID string) *LogWriter {
	w := &LogWriter{
		ctx:              ctx,
		workflowRunID:    workflowRunID,
		workflowJobRunID: workflowJobRunID,
		stepID:           stepID,
		client:           client,
		buffer:           bytes.NewBuffer(make([]byte, 0, maxBufferSize)),
		lineBuffer:       bytes.NewBuffer(nil), // Initialize line buffer
		uploads:          make(chan []byte, 10),
		done:             make(chan struct{}),
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
	fmt.Println("closing")

	// Signal worker to stop
	close(w.done)

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
					resp, err := w.client.GetStepLogSignedBlobURL(w.ctx, w.workflowRunID, w.workflowJobRunID, w.stepID)
					if err != nil {
						zerolog.Ctx(w.ctx).Error().Err(err).Msg("failed to get signed blob URL")
						continue
					}

					fmt.Println(resp.UploadURL)

					req, _ := http.NewRequest(http.MethodPut, resp.UploadURL, bytes.NewReader(chunk))
					req.Header.Set("Content-Type", "application/octet-stream")
					req.Header.Set("Content-Length", strconv.Itoa(len(chunk)))
					req.Header.Set("x-ms-blob-type", "BlockBlob")
					req.Header.Set("x-ms-blob-content-type", "text/plain")

					_, _ = http.DefaultClient.Do(req)

					_ = w.client.CreateStepLogsMetadata(w.ctx, w.workflowRunID, w.workflowJobRunID, w.stepID, 10, time.Now())

					_ = chunk

				default:
					return
				}
			}

		case chunk := <-w.uploads:
			fmt.Println("append not supported yet")
			_ = chunk
		}
	}
}
