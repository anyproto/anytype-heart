package txt

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTXT_GetSnapshots(t *testing.T) {
	h := &TXT{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := h.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfTxtParams{
			TxtParams: &pb.RpcObjectImportRequestTxtParams{Path: []string{"testdata/test.txt", "testdata/test"}},
		},
		Type: 4,
		Mode: 1,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 1)
	assert.Contains(t, sn.Snapshots[0].FileName, "test.txt")
	assert.NotEmpty(t, sn.Snapshots[0].Snapshot.Details.Fields["name"])
	assert.Equal(t, sn.Snapshots[0].Snapshot.Details.Fields["name"], pbtypes.String("test"))

	var (
		found bool
		text  string
	)

	for _, block := range sn.Snapshots[0].Snapshot.GetBlocks() {
		if t, ok := block.Content.(*model.BlockContentOfText); ok {
			found = ok
			text = t.Text.GetText()
		}
	}

	assert.Equal(t, text, "test")
	assert.True(t, found)
}
