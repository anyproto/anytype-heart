package bufferpool

import (
	"io"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPool(t *testing.T) {
	pool := NewPool()
	require.NotNil(t, pool, "NewPool should not return nil")
}

func TestBuffer_Write(t *testing.T) {
	pool := NewPool()
	buf := pool.Get()
	data := []byte("Hello, World!")
	n, err := buf.Write(data)
	require.NoError(t, err, "Write should not return an error")
	assert.Equal(t, len(data), n, "Write should return the number of bytes written")

	err = buf.Close()
	require.NoError(t, err, "Close should not return an error")
}

func TestBuffer_Close(t *testing.T) {
	pool := NewPool()
	buf := pool.Get()

	err := buf.Close()
	require.NoError(t, err, "Close should not return an error")
	n, err := buf.Write([]byte("Hello, World!"))
	assert.ErrorIs(t, err, io.EOF, "Read after Close should return an error")
	require.Zero(t, n, "Write after Close should not write any bytes")
}

func TestBuffer_GetReadSeekCloser(t *testing.T) {
	debug.SetGCPercent(-1)
	pool := NewPool()
	buf := pool.Get()

	data := []byte("Hello, World!")
	_, err := buf.Write(data)
	require.NoError(t, err, "Write should not return an error")

	rsc, err := buf.GetReadSeekCloser()
	require.NoError(t, err, "GetReadSeekCloser should not return an error")
	assert.NotNil(t, rsc, "GetReadSeekCloser should not return nil")

	readData := make([]byte, len(data))
	readData2 := make([]byte, len(data))

	n, err := rsc.Read(readData)
	require.NoError(t, err, "Read should not return an error")
	assert.Equal(t, len(data), n, "Read should return the number of bytes read")
	assert.Equal(t, data, readData, "Read data should match written data")

	n2, err := rsc.Seek(0, io.SeekStart)
	require.NoError(t, err, "Seek should not return an error")
	assert.Equal(t, int64(0), n2, "Seek should return the new offset")

	_, err = rsc.Read(readData2)
	require.NoError(t, err, "Read after seek should not return an error")
	assert.Equal(t, data, readData2, "Read data after seek should match written data")

	err = rsc.Close()
	require.NoError(t, err, "Close should not return an error")

	_, err = rsc.Read(readData)
	assert.Error(t, err, "Read after Close should return an error")

	err = rsc.Close()
	require.NoError(t, err, "Close after Close should not return an error")
	debug.SetGCPercent(100)
}
