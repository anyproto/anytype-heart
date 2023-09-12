package markdown

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestMarkdown_GetSnapshots(t *testing.T) {
	t.Run("snapshots have import date relation", func(t *testing.T) {
		// given
		h := &Markdown{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := h.GetSnapshots(&pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{"testdata/test.md"}},
			},
			Type: 4,
			Mode: 1,
		}, p, 1)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, sn)

		for _, snapshot := range sn.Snapshots {
			if snapshot.SbType == sb.SmartBlockTypeSubObject ||
				lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.URL()) {
				continue
			}
			assert.Contains(t, snapshot.Snapshot.Data.Details.Fields, bundle.RelationKeyImportDate.String())
			assert.Equal(t, int64(1), pbtypes.GetInt64(snapshot.Snapshot.Data.Details, bundle.RelationKeyImportDate.String()))
		}
	})
}
