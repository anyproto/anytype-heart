package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	importv2adapter "github.com/anyproto/anytype-heart/core/block/importv2/adapter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestImportV2Markdown drives a real markdown import through the v2 engine
// end-to-end: real app, real space, real file upload, async run with the
// EventImportFinish completion signal.
func TestImportV2Markdown(t *testing.T) {
	app := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_NONE)

	fileSub := newTestSubscription(t, app, []domain.RelationKey{bundle.RelationKeyId}, []database.FilterRequest{
		filterEqualsToInteger(bundle.RelationKeyFileIndexingStatus, model.FileIndexingStatus_Indexed),
		filterEqualsToString(bundle.RelationKeyName, "saturn"),
		filterEqualsToString(bundle.RelationKeyFileMimeType, "image/jpeg"),
		filterNotEmpty(bundle.RelationKeyFileId),
	})

	getService[*config.Config](app).ImportV2Markdown = true
	v2 := getService[importv2adapter.Importer](app)
	require.True(t, v2.Handles(model.Import_Markdown), "flag must route markdown to v2")
	require.True(t, v2.Handles(model.Import_Obsidian))
	require.False(t, v2.Handles(model.Import_Pb), "other formats stay on v1")

	// when — async import, exactly as the gRPC handler invokes it
	v2.Import(&pb.RpcObjectImportRequest{
		SpaceId: app.personalSpaceId(),
		Mode:    pb.RpcObjectImportRequest_IGNORE_ERRORS,
		Type:    model.Import_Markdown,
		Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
			MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
				Path: []string{"./testdata/import/markdown with files/"},
			},
		},
	})

	// then — completion event carries the root collection and object count
	var rootCollectionId string
	app.waitEventMessage(t, func(msg *pb.EventMessage) bool {
		if v := msg.GetImportFinish(); v != nil {
			rootCollectionId = v.RootCollectionID
			assert.Positive(t, v.ObjectsCount)
			assert.Equal(t, model.Import_Markdown, v.ImportType)
			return true
		}
		return false
	})
	assert.NotEmpty(t, rootCollectionId)

	// and the referenced image went through a real upload + indexing
	fileSub.waitOneObjectDetailsSet(t, app, func(t *testing.T, msg *pb.EventObjectDetailsSet) {
		fileObjectId := msg.Details.GetFields()[bundle.RelationKeyId.String()].GetStringValue()
		assertImageAvailableInGateway(t, app, fileObjectId)
	})
}
