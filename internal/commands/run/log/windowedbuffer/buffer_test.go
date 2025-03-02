package windowedbuffer_test

import (
	"io"
	"testing"

	"github.com/artuross/github-actions-runner/internal/commands/run/log/buffer"
	"github.com/artuross/github-actions-runner/internal/commands/run/log/windowedbuffer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowedBuffer_Close(t *testing.T) {
	bb := buffer.NewBuffer()
	n, err := bb.Write([]byte("0123456789"))
	require.NoError(t, err)
	require.Equal(t, 10, n)

	buf := windowedbuffer.NewBuffer(bb, 0)

	err = buf.Close()
	assert.NoError(t, err)

	err = buf.Close()
	assert.NoError(t, err)
}

func TestWindowedBuffer_Length(t *testing.T) {
	bb := buffer.NewBuffer()
	buf := windowedbuffer.NewBuffer(bb, 0)

	assert.Equal(t, 0, buf.Length())

	n, err := bb.Write([]byte("0123456789"))
	require.NoError(t, err)
	require.Equal(t, 10, n)

	assert.Equal(t, 10, buf.Length())

	n, err = buf.Write([]byte("0123456789"))
	require.NoError(t, err)
	require.Equal(t, 10, n)

	assert.Equal(t, 20, buf.Length())

	err = buf.Close()
	assert.NoError(t, err)

	assert.Equal(t, 20, buf.Length())

	n, err = bb.Write([]byte("0123456789"))
	require.NoError(t, err)
	require.Equal(t, 10, n)

	assert.Equal(t, 20, buf.Length())
}

func TestWindowedBuffer_ReadAt(t *testing.T) {
	t.Run("open buffer", func(t *testing.T) {
		bb := buffer.NewBuffer()
		n, err := bb.Write([]byte("0123456789"))
		require.NoError(t, err)
		require.Equal(t, 10, n)

		buf := windowedbuffer.NewBuffer(bb, 1)

		p1 := make([]byte, 4)
		n, err = buf.ReadAt(p1, 0)
		assert.NoError(t, err)
		assert.Equal(t, 4, n)
		assert.Equal(t, "1234", string(p1))

		p2 := make([]byte, 5)
		n, err = buf.ReadAt(p2, 4)
		assert.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, "56789", string(p2))

		p3 := make([]byte, 5)
		n, err = buf.ReadAt(p3, 9)
		assert.ErrorIs(t, err, io.EOF)
		assert.Zero(t, n)
		assert.Equal(t, []byte{0, 0, 0, 0, 0}, p3)

		n, err = bb.Write([]byte("01234"))
		require.NoError(t, err)
		require.Equal(t, 5, n)

		p4 := make([]byte, 6)
		n, err = buf.ReadAt(p4, 9)
		assert.ErrorIs(t, err, io.EOF)
		assert.Equal(t, 5, n)
		assert.Equal(t, []byte{'0', '1', '2', '3', '4', 0}, p4)
	})

	t.Run("closed buf", func(t *testing.T) {
		bb := buffer.NewBuffer()
		n, err := bb.Write([]byte("0123456789"))
		require.NoError(t, err)
		require.Equal(t, 10, n)

		buf := windowedbuffer.NewBuffer(bb, 1)

		err = buf.Close()
		require.NoError(t, err)

		n, err = bb.Write([]byte("abcd"))
		require.NoError(t, err)
		require.Equal(t, 4, n)

		p1 := make([]byte, 3)
		n, err = buf.ReadAt(p1, 0)
		assert.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, []byte{'1', '2', '3'}, p1)

		p2 := make([]byte, 6)
		n, err = buf.ReadAt(p2, 3)
		assert.ErrorIs(t, err, io.ErrClosedPipe)
		assert.Equal(t, 6, n)
		assert.Equal(t, []byte{'4', '5', '6', '7', '8', '9'}, p2)

		p3 := make([]byte, 3)
		n, err = buf.ReadAt(p3, 8)
		assert.ErrorIs(t, err, io.ErrClosedPipe)
		assert.Equal(t, 1, n)
		assert.Equal(t, []byte{'9', 0, 0}, p3)

		p4 := make([]byte, 3)
		n, err = buf.ReadAt(p4, 10)
		assert.ErrorIs(t, err, io.ErrClosedPipe)
		assert.Equal(t, 0, n)
		assert.Equal(t, []byte{0, 0, 0}, p4)
	})
}
