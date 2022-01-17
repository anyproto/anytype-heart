package core

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestImages(t *testing.T) {
	_, mw, close := start(t, nil)
	defer close()
	t.Run("image_offload_unsplash", func(t *testing.T) {
		coreService := mw.app.MustComponent(core.CName).(core.Service)

		i, err := coreService.ImageUnsplashDownload(context.Background(), "may0ysY9OrU")
		require.NoError(t, err)

		ipfs := mw.app.MustComponent(ipfs.CName).(ipfs.IPFS)
		fileCid, err := cid.Parse(i.Hash())
		require.NoError(t, err)

		hasFile, err := ipfs.HasBlock(fileCid)
		require.NoError(t, err)
		require.True(t, hasFile)

		_, err = coreService.FileOffload(i.Hash())
		require.NoError(t, err)

		hasFile, err = ipfs.HasBlock(fileCid)
		require.NoError(t, err)
		require.False(t, hasFile)

		_, err = coreService.FileOffload(i.Hash())
		require.NoError(t, err)
	})

	t.Run("image_search_unsplash", func(t *testing.T) {
		coreService := mw.app.MustComponent(core.CName).(core.Service)

		i, err := coreService.ImageUnsplashSearch(context.Background(), 3)
		require.NoError(t, err)

		require.True(t, len(i) > 0)
	})

}
