package core

import (
	"context"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAnytype_FileByHash(t *testing.T) {
	s := getRunningService(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	nf, err := s.FileAddWithBytes(ctx, []byte("123"), "file.txt")
	require.NoError(t, err)
	require.Len(t, nf.Hash(), 59)

	f, err := s.FileByHash(ctx, nf.Hash())
	require.NoError(t, err)
	require.Equal(t, nf.Hash(), f.Hash())

	fr, err := f.Reader()
	require.NoError(t, err)

	fb, err := ioutil.ReadAll(fr)
	require.NoError(t, err)
	require.Equal(t, fb, []byte("123"))

	require.NotNil(t, f.Meta())
	require.Equal(t, "file.txt", f.Meta().Name)
	require.Equal(t, int64(3), f.Meta().Size)
}
