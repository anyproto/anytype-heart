package txt

import (
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestTXT_GetSnapshots(t *testing.T) {
	t.Run("successful import", func(t *testing.T) {
		// given
		h := &TXT{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := h.GetSnapshots(&pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfTxtParams{
				TxtParams: &pb.RpcObjectImportRequestTxtParams{Path: []string{"testdata/test.txt"}},
			},
			Type: 4,
			Mode: 1,
		}, p, 0)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, sn)
		assert.Len(t, sn.Snapshots, 2)
		assert.Contains(t, sn.Snapshots[0].FileName, "test.txt")
		assert.NotEmpty(t, sn.Snapshots[0].Snapshot.Data.Details.Fields["name"])
		assert.Equal(t, sn.Snapshots[0].Snapshot.Data.Details.Fields["name"], pbtypes.String("test"))

		assert.Contains(t, sn.Snapshots[1].FileName, rootCollectionName)
		assert.NotEmpty(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes)
		assert.Equal(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.URL())

		var (
			found bool
			text  string
		)

		for _, block := range sn.Snapshots[0].Snapshot.Data.GetBlocks() {
			if t, ok := block.Content.(*model.BlockContentOfText); ok {
				found = ok
				text = t.Text.GetText()
			}
		}

		assert.Equal(t, text, "test")
		assert.True(t, found)
	})

	t.Run("failed to import non-txt file", func(t *testing.T) {
		// given
		h := &TXT{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := h.GetSnapshots(&pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfTxtParams{
				TxtParams: &pb.RpcObjectImportRequestTxtParams{Path: []string{"testdata/test"}},
			},
			Type: 4,
			Mode: 1,
		}, p, 0)

		// then
		assert.NotNil(t, err)
		assert.True(t, errors.Is(err.GetResultError(pb.RpcObjectImportRequest_Txt), converter.ErrNoObjectsToImport))
		assert.Nil(t, sn)
	})

	t.Run("snapshots have import date relation", func(t *testing.T) {
		// given
		h := &TXT{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := h.GetSnapshots(&pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfTxtParams{
				TxtParams: &pb.RpcObjectImportRequestTxtParams{Path: []string{"testdata/test.txt"}},
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
