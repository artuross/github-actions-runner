package buffer

import (
	"io"
	"sync"

	"github.com/artuross/github-actions-runner/internal/commands/run/log/windowedbuffer"
)

var (
	_ io.ReaderAt = (*Buffer)(nil)
	_ io.Writer   = (*Buffer)(nil)
)

type Buffer struct {
	mu       sync.Mutex
	buffer   []byte
	position int
}

func NewBuffer() *Buffer {
	return &Buffer{
		mu:       sync.Mutex{},
		buffer:   make([]byte, 0),
		position: 0,
	}
}

func (b *Buffer) Length() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.position
}

func (b *Buffer) ReadAt(p []byte, off int64) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if off < 0 {
		return 0, io.ErrUnexpectedEOF
	}

	if off >= int64(len(b.buffer)) {
		return 0, io.EOF
	}

	n := copy(p, b.buffer[off:])
	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

func (b *Buffer) WindowedBuffer() *windowedbuffer.WindowedBuffer {
	return windowedbuffer.NewBuffer(b, b.Length())
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buffer = append(b.buffer, p...)
	b.position += len(p)

	return len(p), nil
}
