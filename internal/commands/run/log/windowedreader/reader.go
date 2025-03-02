package windowedreader

import (
	"bytes"
	"io"
	"sync"
	"time"
)

var (
	_ io.Closer = (*WindowedReader)(nil)
	_ io.Reader = (*WindowedReader)(nil)
)

type WindowedReader struct {
	mu     sync.Mutex
	wb     io.ReaderAt
	offset int64
	closed bool
}

func New(wb io.ReaderAt) *WindowedReader {
	return &WindowedReader{
		mu:     sync.Mutex{},
		wb:     wb,
		offset: 0,
		closed: false,
	}
}

func (r *WindowedReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.closed = true

	return nil
}

func (r *WindowedReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// read from the current offset and advance
	n, err := r.wb.ReadAt(p, r.offset)
	if err == nil {
		r.offset += int64(n)
	}

	return n, err
}

func (r *WindowedReader) ReadLines(maxLength int) ([]byte, error) {
	result := make([]byte, maxLength)
	totalBytes := 0

	r.mu.Lock()
	offset := r.offset
	r.mu.Unlock()

	for {
		n, err := r.wb.ReadAt(result[totalBytes:], int64(totalBytes)+offset)
		if err != nil && err != io.EOF && err != io.ErrClosedPipe {
			totalBytes += n

			r.mu.Lock()
			r.offset += int64(totalBytes)
			r.mu.Unlock()

			return result[:totalBytes], err
		}

		if err == io.ErrClosedPipe && n == 0 {
			r.mu.Lock()
			r.offset += int64(totalBytes)
			r.mu.Unlock()

			return result[:totalBytes], err
		}

		if n == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for n > 0 {
			idx := bytes.IndexByte(result[totalBytes:totalBytes+n], '\n')
			if idx == -1 {
				time.Sleep(100 * time.Millisecond)
				break
			}

			realIndex := totalBytes + idx + 1
			if realIndex > maxLength && totalBytes > 0 {
				r.mu.Lock()
				r.offset += int64(totalBytes)
				r.mu.Unlock()

				return result[:totalBytes], nil
			}

			n -= idx + 1
			totalBytes = realIndex
		}

		if totalBytes == maxLength {
			return result, nil
		}
	}
}
