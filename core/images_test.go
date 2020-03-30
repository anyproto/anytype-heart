package core

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAnytype_ImageByHash(t *testing.T) {
	s := getRunningService(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	fd, err := os.Open("mill/testdata/image.jpeg")

	nf, err := s.ImageAddWithReader(ctx, fd, "image.jpeg")
	require.NoError(t, err)
	require.Len(t, nf.Hash(), 59)

	f, err := s.ImageByHash(ctx, nf.Hash())
	require.NoError(t, err)
	require.Equal(t, nf.Hash(), f.Hash())

	flargest, err := f.GetFileForLargestWidth(ctx)
	require.NoError(t, err)

	flargestr, err := flargest.Reader()
	require.NoError(t, err)

	fb, err := ioutil.ReadAll(flargestr)
	require.NoError(t, err)
	require.True(t, len(fb) > 100)

	require.NotNil(t, flargest.Meta())
	require.Equal(t, "image.jpeg", flargest.Meta().Name)
	require.Equal(t, int64(63098), flargest.Meta().Size)
}
