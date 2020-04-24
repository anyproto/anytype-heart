package core

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-library/files"
	"github.com/stretchr/testify/require"
)

func TestAnytype_ImageByHash(t *testing.T) {
	s := getRunningService(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	fd, err := os.Open("../mill/testdata/image.jpeg")
	require.NoError(t, err)

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
	require.Equal(t, int64(68648), flargest.Meta().Size)
}

func TestAnytype_ImageByHashUnencrypted(t *testing.T) {
	s := getRunningService(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	fd, err := os.Open("../mill/testdata/image.jpeg")
	require.NoError(t, err)

	nf, err := s.ImageAdd(ctx, files.WithReader(fd), files.WithName("image.jpeg"), files.WithPlaintext(true))
	require.NoError(t, err)
	require.Len(t, nf.Hash(), 59)

	f, err := s.ImageByHash(ctx, nf.Hash())
	require.NoError(t, err)
	for _, variant := range f.(*image).variantsByWidth {
		require.Equal(t, "", variant.Key)
	}

	flargest, err := f.GetFileForLargestWidth(ctx)
	require.NoError(t, err)

	require.Equal(t, "", flargest.(*file).info.Key)

	flargestr, err := flargest.Reader()
	require.NoError(t, err)

	fb, err := ioutil.ReadAll(flargestr)
	require.NoError(t, err)
	require.True(t, len(fb) > 100)

	require.NotNil(t, flargest.Meta())
	require.Equal(t, "image.jpeg", flargest.Meta().Name)
	require.Equal(t, int64(68648), flargest.Meta().Size)
}

func TestAnytype_ImageFileKeysRestore(t *testing.T) {
	s := getRunningService(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	fd, err := os.Open("../mill/testdata/image.png")
	require.NoError(t, err)

	nf, err := s.ImageAddWithReader(ctx, fd, "image.jpeg")
	require.NoError(t, err)
	require.Len(t, nf.Hash(), 59)

	keysExpectedJson, _ := json.Marshal(s.(*Anytype).files.KeysCache[nf.Hash()])
	s.(*Anytype).files.KeysCache = make(map[string]map[string]string)

	keysActual, err := s.(*Anytype).files.FileRestoreKeys(context.Background(), nf.Hash())
	require.NoError(t, err)

	keysActualJson, _ := json.Marshal(keysActual)
	require.Equal(t, keysExpectedJson, keysActualJson)
}
