package html

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestHTML_GetSnapshots(t *testing.T) {
	h := &HTML{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := h.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfHtmlParams{
			HtmlParams: &pb.RpcObjectImportRequestHtmlParams{Path: []string{"testdata/test.html", "testdata/test"}},
		},
		Type: 3,
		Mode: 1,
	}, p)

	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 1)
	assert.Contains(t, sn.Snapshots[0].FileName, "test.html")
	assert.NotEmpty(t, sn.Snapshots[0].Snapshot.Details.Fields["name"])
	assert.Equal(t, sn.Snapshots[0].Snapshot.Details.Fields["name"], pbtypes.String("test"))

	assert.Nil(t, err)
}
