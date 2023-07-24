package html

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	cv "github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestHTML_GetSnapshots(t *testing.T) {
	h := &HTML{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfHtmlParams{
			HtmlParams: &pb.RpcObjectImportRequestHtmlParams{Path: []string{"testdata/test.html", "testdata/test"}},
		},
		Type: pb.RpcObjectImportRequest_Txt,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 2)
	assert.Contains(t, sn.Snapshots[0].FileName, "test.html")
	assert.NotEmpty(t, sn.Snapshots[0].Snapshot.Data.Details.Fields["name"])
	assert.Equal(t, sn.Snapshots[0].Snapshot.Data.Details.Fields["name"], pbtypes.String("test"))

	assert.Contains(t, sn.Snapshots[1].FileName, rootCollectionName)
	assert.NotEmpty(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes)
	assert.Equal(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.URL())

	assert.NotNil(t, err)
	assert.Equal(t, err["testdata/test"], cv.ErrNoObjectsToImport)
}
