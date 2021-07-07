package debugtree

import (
	"context"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFilename = "./testdata/at.dbg.bafybahawhs6m7zuohuhjg3h4cmbji57d4pnc464giqtroyms45pc6ax4.20210707.142017.29.zip"

func TestOpen(t *testing.T) {
	dt, err := Open(testFilename)
	require.NoError(t, err)
	defer dt.Close()

	t.Run("id", func(t *testing.T) {
		assert.Equal(t, "bafybahawhs6m7zuohuhjg3h4cmbji57d4pnc464giqtroyms45pc6ax4", dt.ID())
	})

	t.Run("type", func(t *testing.T) {
		assert.Equal(t, smartblock.SmartBlockTypePage, dt.Type())
	})

	t.Run("logs", func(t *testing.T) {
		sl, err := dt.GetLogs()
		require.NoError(t, err)
		assert.NotEmpty(t, sl)
	})

	t.Run("record", func(t *testing.T) {
		rec, err := dt.GetRecord(context.TODO(), "bafyreicegzanmqdbjgwukrxbp52fzfrw27h6dx2xvhvkze3kdc2qogemvq")
		require.NoError(t, err)
		assert.NotEmpty(t, rec)
	})
}
