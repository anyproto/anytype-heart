package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestImportFileFromRelation(t *testing.T) {
	ctx := context.Background()
	app := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_NONE)

	fileSub := newTestSubscription(t, app, []domain.RelationKey{bundle.RelationKeyId}, []database.FilterRequest{
		filterEqualsToInteger(bundle.RelationKeyFileIndexingStatus, model.FileIndexingStatus_Indexed),
		filterEqualsToInteger(bundle.RelationKeyResolvedLayout, model.ObjectType_image),
		filterEqualsToString(bundle.RelationKeyName, "Saturn"),
		filterEqualsToString(bundle.RelationKeyFileMimeType, "image/jpeg"),
		filterNotEmpty(bundle.RelationKeyFileId),
	})

	objectSub := newTestSubscription(t, app, []domain.RelationKey{bundle.RelationKeyId, bundle.RelationKeyIconImage}, []database.FilterRequest{
		filterNotEmpty(bundle.RelationKeyIconImage),
	})

	importerService := getService[importer.Importer](app)
	res := importerService.Import(ctx, &importer.ImportRequest{
		RpcObjectImportRequest: &pb.RpcObjectImportRequest{
			SpaceId: app.personalSpaceId(),
			Mode:    pb.RpcObjectImportRequest_IGNORE_ERRORS,
			Type:    model.Import_Pb,
			Params: &pb.RpcObjectImportRequestParamsOfPbParams{
				PbParams: &pb.RpcObjectImportRequestPbParams{
					Path: []string{"./testdata/import/object with file relation/"},
				},
			},
		},
		Origin: objectorigin.Import(model.Import_Pb),
		IsSync: true,
	})
	require.NoError(t, res.Err)

	app.waitEventMessage(t, func(msg *pb.EventMessage) bool {
		if v := msg.GetProcessDone(); v != nil {
			return v.Process.Id == res.ProcessId
		}
		return false
	})

	var fileObjectId string
	fileSub.waitOneObjectDetailsSet(t, app, func(t *testing.T, msg *pb.EventObjectDetailsSet) {
		fileObjectId = pbtypes.GetString(msg.Details, bundle.RelationKeyId.String())
		assertImageAvailableInGateway(t, app, fileObjectId)
	})
	objectSub.waitObjectDetailsSetWithPredicate(t, app, func(t *testing.T, msg *pb.EventObjectDetailsSet) bool {
		list := pbtypes.GetStringList(msg.Details, bundle.RelationKeyIconImage.String())
		if len(list) > 0 {
			return fileObjectId == list[0]
		}
		return false

	})
}

func TestImportFileFromBlock(t *testing.T) {
	testImportObjectWithFileBlock(t, "./testdata/import/object with file block/")
}

func TestImportFileFromMarkdown(t *testing.T) {
	testImportFileFromMarkdown(t, "./testdata/import/markdown with files/")
}

func testImportFileFromMarkdown(t *testing.T, path string) {
	ctx := context.Background()
	app := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_NONE)

	fileSub := newTestSubscription(t, app, []domain.RelationKey{bundle.RelationKeyId}, []database.FilterRequest{
		filterEqualsToInteger(bundle.RelationKeyFileIndexingStatus, model.FileIndexingStatus_Indexed),
		filterEqualsToInteger(bundle.RelationKeyResolvedLayout, model.ObjectType_image),
		filterEqualsToString(bundle.RelationKeyName, "saturn"), // Name comes from file's name
		filterEqualsToString(bundle.RelationKeyFileMimeType, "image/jpeg"),
		filterNotEmpty(bundle.RelationKeyFileId),
	})

	importerService := getService[importer.Importer](app)
	res := importerService.Import(ctx, &importer.ImportRequest{
		RpcObjectImportRequest: &pb.RpcObjectImportRequest{
			SpaceId: app.personalSpaceId(),
			Mode:    pb.RpcObjectImportRequest_IGNORE_ERRORS,
			Type:    model.Import_Markdown,
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
					Path: []string{path},
				},
			},
		},
		Origin: objectorigin.Import(model.Import_Markdown),
		IsSync: true,
	})
	require.NoError(t, res.Err)

	app.waitEventMessage(t, func(msg *pb.EventMessage) bool {
		if v := msg.GetProcessDone(); v != nil {
			return v.Process.Id == res.ProcessId
		}
		return false
	})
	fileSub.waitOneObjectDetailsSet(t, app, func(t *testing.T, msg *pb.EventObjectDetailsSet) {
		fileObjectId := pbtypes.GetString(msg.Details, bundle.RelationKeyId.String())
		assertImageAvailableInGateway(t, app, fileObjectId)
	})
}

func testImportObjectWithFileBlock(t *testing.T, path string) {
	ctx := context.Background()
	app := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_NONE)

	fileSub := newTestSubscription(t, app, []domain.RelationKey{bundle.RelationKeyId}, []database.FilterRequest{
		filterEqualsToInteger(bundle.RelationKeyFileIndexingStatus, model.FileIndexingStatus_Indexed),
		filterEqualsToInteger(bundle.RelationKeyResolvedLayout, model.ObjectType_image),
		filterEqualsToString(bundle.RelationKeyName, "test_image"),
		filterEqualsToString(bundle.RelationKeyFileMimeType, "image/png"),
		filterNotEmpty(bundle.RelationKeyFileId),
	})

	importerService := getService[importer.Importer](app)
	res := importerService.Import(ctx, &importer.ImportRequest{
		RpcObjectImportRequest: &pb.RpcObjectImportRequest{
			SpaceId: app.personalSpaceId(),
			Mode:    pb.RpcObjectImportRequest_IGNORE_ERRORS,
			Type:    model.Import_Pb,
			Params: &pb.RpcObjectImportRequestParamsOfPbParams{
				PbParams: &pb.RpcObjectImportRequestPbParams{
					Path: []string{path},
				},
			},
		},
		Origin: objectorigin.Import(model.Import_Pb),
		IsSync: true,
	})
	require.NoError(t, res.Err)

	app.waitEventMessage(t, func(msg *pb.EventMessage) bool {
		if v := msg.GetProcessDone(); v != nil {
			return v.Process.Id == res.ProcessId
		}
		return false
	})
	fileSub.waitOneObjectDetailsSet(t, app, func(t *testing.T, msg *pb.EventObjectDetailsSet) {
		fileObjectId := pbtypes.GetString(msg.Details, bundle.RelationKeyId.String())
		assertImageAvailableInGateway(t, app, fileObjectId)
	})
}
