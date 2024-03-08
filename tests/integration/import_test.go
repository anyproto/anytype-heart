package integration

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestImportFileFromRelation(t *testing.T) {
	ctx := context.Background()
	app := createAccountAndStartApp(t, pb.RpcObjectImportUseCaseRequest_NONE)

	fileSub := newTestSubscription(t, app, []domain.RelationKey{bundle.RelationKeyId}, []*model.BlockContentDataviewFilter{
		filterEqualsToInteger(bundle.RelationKeyFileIndexingStatus, model.FileIndexingStatus_Indexed),
		filterEqualsToInteger(bundle.RelationKeyLayout, model.ObjectType_image),
		filterEqualsToString(bundle.RelationKeyName, "Saturn"),
		filterEqualsToString(bundle.RelationKeyFileMimeType, "image/jpeg"),
		filterNotEmpty(bundle.RelationKeyFileId),
	})

	objectSub := newTestSubscription(t, app, []domain.RelationKey{bundle.RelationKeyId, bundle.RelationKeyIconImage}, []*model.BlockContentDataviewFilter{
		filterNotEmpty(bundle.RelationKeyIconImage),
	})

	importerService := getService[importer.Importer](app)
	_, processId, err := importerService.Import(ctx, &pb.RpcObjectImportRequest{
		SpaceId: app.personalSpaceId(),
		Mode:    pb.RpcObjectImportRequest_IGNORE_ERRORS,
		Type:    model.Import_Pb,
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{
			PbParams: &pb.RpcObjectImportRequestPbParams{
				Path: []string{"./testdata/import/object with file relation/"},
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

	fileSub := newTestSubscription(t, app, []domain.RelationKey{bundle.RelationKeyId}, []*model.BlockContentDataviewFilter{
		filterEqualsToInteger(bundle.RelationKeyFileIndexingStatus, model.FileIndexingStatus_Indexed),
		filterEqualsToInteger(bundle.RelationKeyLayout, model.ObjectType_image),
		filterEqualsToString(bundle.RelationKeyName, "saturn"), // Name comes from file's name
		filterEqualsToString(bundle.RelationKeyFileMimeType, "image/jpeg"),
		filterNotEmpty(bundle.RelationKeyFileId),
	})

	importerService := getService[importer.Importer](app)
	_, processId, err := importerService.Import(ctx, &pb.RpcObjectImportRequest{
		SpaceId: app.personalSpaceId(),
		Mode:    pb.RpcObjectImportRequest_IGNORE_ERRORS,
		Type:    model.Import_Markdown,
		Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
			MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
				Path: []string{path},
			},
		},
	}, objectorigin.Import(model.Import_Markdown), nil)
	require.NoError(t, err)

	app.waitEventMessage(t, func(msg *pb.EventMessage) bool {
		if v := msg.GetProcessDone(); v != nil {
			return v.Process.Id == processId
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

	fileSub := newTestSubscription(t, app, []domain.RelationKey{bundle.RelationKeyId}, []*model.BlockContentDataviewFilter{
		filterEqualsToInteger(bundle.RelationKeyFileIndexingStatus, model.FileIndexingStatus_Indexed),
		filterEqualsToInteger(bundle.RelationKeyLayout, model.ObjectType_image),
		filterEqualsToString(bundle.RelationKeyName, "test_image"),
		filterEqualsToString(bundle.RelationKeyFileMimeType, "image/png"),
		filterNotEmpty(bundle.RelationKeyFileId),
	})

	importerService := getService[importer.Importer](app)
	_, processId, err := importerService.Import(ctx, &pb.RpcObjectImportRequest{
		SpaceId: app.personalSpaceId(),
		Mode:    pb.RpcObjectImportRequest_IGNORE_ERRORS,
		Type:    model.Import_Pb,
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{
			PbParams: &pb.RpcObjectImportRequestPbParams{
				Path: []string{path},
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
	fileSub.waitOneObjectDetailsSet(t, app, func(t *testing.T, msg *pb.EventObjectDetailsSet) {
		fileObjectId := pbtypes.GetString(msg.Details, bundle.RelationKeyId.String())
		assertImageAvailableInGateway(t, app, fileObjectId)
	})
}

func assertImageAvailableInGateway(t *testing.T, app *testApplication, fileObjectId string) {
	gw := getService[gateway.Gateway](app)
	host := gw.Addr()
	resp, err := http.Get("http://" + host + "/image/" + fileObjectId)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t, len(raw) > 0)
}
