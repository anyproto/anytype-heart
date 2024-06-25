package ftsearch

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

func Test_AutoBatcher(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "")
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	docsCount, err := ft.DocCount()
	require.NoError(t, err)
	require.Equal(t, 0, int(docsCount))

	batcher := ft.NewAutoBatcher(30, 100000)
	for i := 0; i < 32; i++ {
		err = batcher.UpdateDoc(
			SearchDoc{
				Id:    domain.NewObjectPathWithBlock("o", fmt.Sprintf("%d", i)).String(),
				Title: "one",
				Text:  "two",
			})
		require.NoError(t, err)
	}
	docsCount, err = ft.DocCount()
	require.Equal(t, 30, int(docsCount))

	err = batcher.Finish()
	docsCount, err = ft.DocCount()
	require.Equal(t, 32, int(docsCount))

	for i := 0; i < 32; i++ {
		err = batcher.DeleteDoc(domain.NewObjectPathWithBlock("o", fmt.Sprintf("%d", i)).String())
		require.NoError(t, err)
	}

	docsCount, err = ft.DocCount()
	require.Equal(t, 32, int(docsCount))
	batcher.Finish()
	docsCount, err = ft.DocCount()
	require.Equal(t, 0, int(docsCount))
}
