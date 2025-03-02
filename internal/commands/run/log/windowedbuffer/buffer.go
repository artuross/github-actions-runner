package windowedbuffer

import (
	"io"
	"sync"

	"github.com/artuross/github-actions-runner/internal/commands/run/log/windowedreader"
)

var (
	_ io.Closer   = (*WindowedBuffer)(nil)
	_ io.ReaderAt = (*WindowedBuffer)(nil)
	_ io.Writer   = (*WindowedBuffer)(nil)
)

type buffer interface {
	io.Writer
	io.ReaderAt
	Length() int
}

type Reader interface {
	io.Closer
	ReadLines(maxLength int) ([]byte, error)
}

type WindowedBuffer struct {
	mu            sync.Mutex
	buffer        buffer
	startPosition int
	endPosition   int
}

func NewBuffer(buffer buffer, startPosition int) *WindowedBuffer {
	return &WindowedBuffer{
		mu:            sync.Mutex{},
		buffer:        buffer,
		startPosition: startPosition,
		endPosition:   -1,
	}
}

func (w *WindowedBuffer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.endPosition > -1 {
		return nil
	}

	w.endPosition = w.buffer.Length()

	return nil
}

func (w *WindowedBuffer) Length() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.endPosition > -1 {
		return w.endPosition - w.startPosition
	}

	return w.buffer.Length() - w.startPosition
}

func (w *WindowedBuffer) ReadAt(p []byte, offset int64) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if offset < 0 {
		return 0, io.ErrUnexpectedEOF
	}

	if w.endPosition == -1 {
		return w.buffer.ReadAt(p, int64(w.startPosition)+offset)
	}

	maxToEnd := w.endPosition - w.startPosition - int(offset)
	maxToRead := min(maxToEnd, len(p))
	if maxToRead <= 0 {
		return 0, io.ErrClosedPipe
	}

	n, err := w.buffer.ReadAt(p[:maxToRead], int64(w.startPosition)+offset)
	if err != nil {
		return n, err
	}

	if n == maxToEnd {
		return n, io.ErrClosedPipe
	}

	return n, nil
}

func (w *WindowedBuffer) Reader() Reader {
	return windowedreader.New(w)
}

func (b *WindowedBuffer) WindowedBuffer() *WindowedBuffer {
	return NewBuffer(b, b.buffer.Length())
}

func (w *WindowedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.endPosition > -1 {
		return 0, io.ErrClosedPipe
	}

	return w.buffer.Write(p)
}
