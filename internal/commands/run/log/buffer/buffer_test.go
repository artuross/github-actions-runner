package buffer_test

import (
	"io"
	"testing"

	"github.com/artuross/github-actions-runner/internal/commands/run/log/buffer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuffer_Length(t *testing.T) {
	buf := buffer.NewBuffer()

	const initialLength = 0
	assert.Equal(t, initialLength, buf.Length())

	const str1 = "0123456789"
	const strLen1 = 10
	n, err := buf.Write([]byte(str1))
	require.NoError(t, err)
	require.Equal(t, strLen1, n)

	assert.Equal(t, strLen1, buf.Length())

	const str2 = "abcdefgh"
	const strLen2 = 8
	n, err = buf.Write([]byte(str2))
	require.NoError(t, err)
	require.Equal(t, strLen2, n)

	assert.Equal(t, strLen1+strLen2, buf.Length())
}

func TestBuffer_ReadAt(t *testing.T) {
	t.Run("negative offset", func(t *testing.T) {
		buf := buffer.NewBuffer()

		const offset = -1
		n, err := buf.ReadAt(nil, int64(offset))
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
		assert.Zero(t, n)
	})

	t.Run("offset over length", func(t *testing.T) {
		buf := buffer.NewBuffer()

		const str = "0123456789"
		const strLen = 10
		n, err := buf.Write([]byte(str))
		require.NoError(t, err)
		require.Equal(t, strLen, n)

		const offset = strLen + 1
		n, err = buf.ReadAt(nil, int64(offset))
		require.ErrorIs(t, err, io.EOF)
		assert.Zero(t, n)
	})

	t.Run("ok", func(t *testing.T) {
		buf := buffer.NewBuffer()

		const str = "0123456789"
		const strLen = 10

		n, err := buf.Write([]byte(str))
		require.NoError(t, err)
		require.Equal(t, strLen, n)

		const offset1 = int64(0)
		dest1 := make([]byte, 5)
		destLen1 := 5
		n, err = buf.ReadAt(dest1, offset1)
		assert.NoError(t, err)
		assert.Equal(t, destLen1, n)
		assert.Equal(t, []byte{'0', '1', '2', '3', '4'}, dest1)

		const offset2 = int64(4)
		dest2 := make([]byte, 4)
		destLen2 := 4
		n, err = buf.ReadAt(dest2, (offset2))
		assert.NoError(t, err)
		assert.Equal(t, destLen2, n)
		assert.Equal(t, []byte{'4', '5', '6', '7'}, dest2)

		const offset3 = 7
		dest3 := make([]byte, 10)
		destLen3 := 3 // 10 - 7
		n, err = buf.ReadAt(dest3, int64(offset3))
		assert.ErrorIs(t, err, io.EOF)
		assert.Equal(t, destLen3, n)
		assert.Equal(t, []byte{'7', '8', '9', 0, 0, 0, 0, 0, 0, 0}, dest3)
	})
}
