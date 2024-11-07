package txt

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/test"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestTXT_GetSnapshots(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h := &TXT{}
		p := process.NewNoOp()
		sn, err := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfTxtParams{
				TxtParams: &pb.RpcObjectImportRequestTxtParams{Path: []string{filepath.Join("testdata", "test.txt"), filepath.Join("testdata", "test")}},
			},
			Type: 4,
			Mode: 1,
		}, p)

		assert.NotNil(t, err)
		assert.True(t, errors.Is(err.GetResultError(model.Import_Txt), common.ErrFileImportNoObjectsInDirectory))
		assert.NotNil(t, sn)
		assert.Len(t, sn.Snapshots, 2)
		assert.Contains(t, sn.Snapshots[0].FileName, "test.txt")
		assert.NotEmpty(t, sn.Snapshots[0].Snapshot.Data.Details.Fields["name"])
		assert.Equal(t, sn.Snapshots[0].Snapshot.Data.Details.Fields["name"], pbtypes.String("test"))

		assert.Contains(t, sn.Snapshots[1].FileName, rootCollectionName)
		assert.NotEmpty(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes)
		assert.Equal(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.String())

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
	t.Run("no objects in dir", func(t *testing.T) {
		// given
		dir := t.TempDir()
		h := &TXT{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		_, err := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfTxtParams{
				TxtParams: &pb.RpcObjectImportRequestTxtParams{Path: []string{dir}},
			},
			Type: 4,
			Mode: 1,
		}, p)
		// then
		assert.NotNil(t, err)
		assert.True(t, errors.Is(err.GetResultError(model.Import_Pb), common.ErrFileImportNoObjectsInDirectory))
	})
	t.Run("no objects in archive", func(t *testing.T) {
		// given
		dir := t.TempDir()
		zipPath := filepath.Join(dir, "empty.zip")
		err := test.CreateEmptyZip(t, zipPath)
		assert.Nil(t, err)

		h := &TXT{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		_, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfTxtParams{
				TxtParams: &pb.RpcObjectImportRequestTxtParams{Path: []string{zipPath}},
			},
			Type: 4,
			Mode: 1,
		}, p)
		// then
		assert.NotNil(t, ce)
		assert.True(t, errors.Is(ce.GetResultError(model.Import_Pb), common.ErrFileImportNoObjectsInZipArchive))
	})
}
