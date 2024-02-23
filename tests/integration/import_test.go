package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestImportFiles(t *testing.T) {
	t.Run("import from version with Files as Objects", func(t *testing.T) {
		ctx := context.Background()
		app := createAccountAndStartApp(t)

		subscriptionId := "files"
		subscriptionService := getService[subscription.Service](app)
		_, err := subscriptionService.Search(pb.RpcObjectSearchSubscribeRequest{
			SubId: subscriptionId,
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyFileId.String(),
			},
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyFileIndexingStatus.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.FileIndexingStatus_Indexed)),
				},
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_image)),
				},
				{
					RelationKey: bundle.RelationKeyName.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("test_image"),
				},
				{
					RelationKey: bundle.RelationKeyFileMimeType.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String("image/png"),
				},
				{
					RelationKey: bundle.RelationKeyFileId.String(),
					Condition:   model.BlockContentDataviewFilter_NotEmpty,
				},
			},
		})
		require.NoError(t, err)

		importerService := getService[importer.Importer](app)
		_, processId, err := importerService.Import(ctx, &pb.RpcObjectImportRequest{
			SpaceId: app.personalSpaceId(),
			Mode:    pb.RpcObjectImportRequest_IGNORE_ERRORS,
			Type:    model.Import_Pb,
			Params: &pb.RpcObjectImportRequestParamsOfPbParams{
				PbParams: &pb.RpcObjectImportRequestPbParams{
					Path: []string{"./testdata/import/object with file block/"},
				},
			},
		}, objectorigin.Import(model.Import_Pb), nil)
		require.NoError(t, err)

		app.waitEventMessage(t, func(msg *pb.EventMessage) bool {
			if v := msg.GetProcessDone(); v != nil {
				return v.Process.Id == processId
			}
			return false
		})
		app.waitEventMessage(t, func(msg *pb.EventMessage) bool {
			if v := msg.GetObjectDetailsSet(); v != nil {
				if slices.Contains(v.SubIds, subscriptionId) {
					return true
				}
			}
			return false
		})
	})
}
