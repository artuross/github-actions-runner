package timeline

import (
	"fmt"
	"io"
	"sync"
)

type MultiLogWriter struct {
	writers []io.Writer
	mu      sync.Mutex
}

// NewMultiLogWriter creates a new MultiLogWriter that writes to multiple writers
func NewMultiLogWriter(writer io.Writer, writers ...io.Writer) *MultiLogWriter {
	return &MultiLogWriter{
		writers: append([]io.Writer{writer}, writers...),
	}
}

// Write implements io.Writer and writes to all underlying writers
func (w *MultiLogWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, writer := range w.writers {
		n, err = writer.Write(p)
		if err != nil {
			return n, fmt.Errorf("write failed: %w", err)
		}
	}

	return n, nil
}
