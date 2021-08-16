package files

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)


func TestWriteReaderIntoFileIgnoreSameExistingFile(t *testing.T) {
	t.Run("expect same path", func(t *testing.T) {
		b := []byte{0x00, 0x10, 0x44}
		tmp, err := os.CreateTemp("", "*.txt")
		tmpPath := tmp.Name()
		require.NoError(t, err)
		tmp.Write(b)
		tmp.Close()

		path, err := WriteReaderIntoFileReuseSameExistingFile(tmpPath, bytes.NewReader(b))
		require.NoError(t, err)
		require.Equal(t, tmpPath, path)
	})

	t.Run("expect suffix", func(t *testing.T) {
		b := []byte{0x00, 0x10, 0x44}
		tmp, err := os.CreateTemp("", "*.txt")
		tmpPath := tmp.Name()
		require.NoError(t, err)
		tmp.Write(b)
		tmp.Close()

		b2 := []byte{0x00, 0x10, 0x47}
		path, err := WriteReaderIntoFileReuseSameExistingFile(tmpPath, bytes.NewReader(b2))
		require.NoError(t, err)
		require.NotEqual(t, tmpPath, path)
		require.Equal(t, filepath.Dir(tmpPath), filepath.Dir(path))
		require.Equal(t, filepath.Ext(tmpPath), filepath.Ext(path))
	})

}
