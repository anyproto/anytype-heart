//go:build integration

package tests

import (
	"os"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	migrationMnemonicKey = "migration_mnemonic"
	migrationAccounIDKey = "migration_account_id"
)

func newImportSession(t *testing.T, port string) *testSession {
	var s testSession

	c, err := newClient(port)
	require.NoError(t, err)
	s.ClientCommandsClient = c

	mnemonic, err := readStringFromCache(migrationMnemonicKey)
	require.NoError(t, err)
	t.Log("your mnemonic:", mnemonic)

	cctx := s.newCallCtx(t)
	_ = call(cctx, s.WalletRecover, &pb.RpcWalletRecoverRequest{
		Mnemonic: mnemonic,
		RootPath: rootPath,
	})

	cctx, s.eventReceiver = s.openClientSession(t, mnemonic)

	accountID, err := readStringFromCache(migrationAccounIDKey)
	require.NoError(t, err)
	t.Log("your account ID:", accountID)

	return &s
}

func fetchObjects(t *testing.T, s *testSession, ids []string) map[string]*model.ObjectView {
	cctx := s.newCallCtx(t)

	res := map[string]*model.ObjectView{}
	for _, id := range ids {
		resp := call(cctx, s.ObjectShow, &pb.RpcObjectShowRequest{
			ObjectId: id,
		})

		res[id] = resp.ObjectView
	}
	return res
}

func createAndExportAccount(t *testing.T) (string, map[string]*model.ObjectView) {
	exportPort := os.Getenv("ANYTYPE_OLD_TEST_GRPC_PORT")
	if exportPort == "" {
		t.Fatal("you must specify ANYTYPE_OLD_TEST_GRPC_PORT env variable")
	}

	exportSession := newTestSession(t, exportPort, migrationMnemonicKey, migrationAccounIDKey)
	cctx := exportSession.newCallCtx(t)

	resp := call(cctx, exportSession.ObjectSearch, &pb.RpcObjectSearchRequest{
		Keys: []string{bundle.RelationKeyId.String()},
	})

	oldObjectIDs := lo.Map(resp.Records, func(r *types.Struct, _ int) string {
		return r.Fields[bundle.RelationKeyId.String()].GetStringValue()
	})
	oldObjects := fetchObjects(t, exportSession, oldObjectIDs)

	exportResp := call(cctx, exportSession.ObjectListExport, &pb.RpcObjectListExportRequest{
		Path:            "/var/anytype_old/",
		Format:          pb.RpcObjectListExport_Protobuf,
		Zip:             true,
		IncludeArchived: true,
		IncludeFiles:    true,
		IncludeNested:   true,
	})

	call(cctx, exportSession.AccountStop, &pb.RpcAccountStopRequest{
		RemoveData: false,
	})

	return exportResp.Path, oldObjects
}

func TestMigration(t *testing.T) {
	_ = os.RemoveAll(cacheFilename(migrationMnemonicKey))
	_ = os.RemoveAll(cacheFilename(migrationAccounIDKey))

	exportPath, oldObjects := createAndExportAccount(t)

	importSession := newImportSession(t, os.Getenv("ANYTYPE_TEST_GRPC_PORT"))
	cctx := importSession.newCallCtx(t)

	call(cctx, importSession.AccountRecoverFromLegacyExport, &pb.RpcAccountRecoverFromLegacyExportRequest{
		Path:     exportPath,
		RootPath: rootPath + "_new",
	})

	call(cctx, importSession.ObjectImport, &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{
			PbParams: &pb.RpcObjectImportRequestPbParams{
				Path: []string{exportPath},
			},
		},
		Type: pb.RpcObjectImportRequest_Pb,
	})

	time.Sleep(1 * time.Minute)

	resp := call(cctx, importSession.ObjectSearch, &pb.RpcObjectSearchRequest{
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyOldAnytypeID.String()},
	})

	filtered := lo.Filter(resp.Records, func(r *types.Struct, _ int) bool {
		return r.Fields[bundle.RelationKeyOldAnytypeID.String()].GetStringValue() != ""
	})
	newIDtoOldID := lo.SliceToMap(filtered, func(r *types.Struct) (string, string) {
		return r.Fields[bundle.RelationKeyId.String()].GetStringValue(), r.Fields[bundle.RelationKeyOldAnytypeID.String()].GetStringValue()
	})

	newObjectIDs := lo.Map(filtered, func(r *types.Struct, _ int) string {
		return r.Fields[bundle.RelationKeyId.String()].GetStringValue()
	})
	newObjects := fetchObjects(t, importSession, newObjectIDs)

	for id, newObject := range newObjects {
		t.Run("details for "+id, func(t *testing.T) {
			oldID := newIDtoOldID[id]
			oldObject := oldObjects[oldID]

			oldDetails := normalizeDetails(oldObject.Details[0].Details)
			newDetails := normalizeDetails(substituteLinksInDetails(newObject.Details[0].Details, newIDtoOldID))
			assertDetails(t, oldDetails, newDetails)

			blockbuilder.AssertPagesEqualWithLinks(t, oldObject.Blocks, newObject.Blocks, newIDtoOldID)
		})
	}
}

func assertDetails(t *testing.T, wantDetails *types.Struct, gotDetails *types.Struct) {
	for key, want := range wantDetails.Fields {
		got := gotDetails.Fields[key]
		assert.Equal(t, want, got, key)
	}
}

func substituteLinksInDetails(d *types.Struct, idsMap map[string]string) *types.Struct {
	for k := range d.Fields {
		if id := pbtypes.GetString(d, k); id != "" {
			if newID, ok := idsMap[id]; ok {
				d.Fields[k] = pbtypes.String(newID)
			}
		} else if ids := pbtypes.GetStringList(d, k); len(ids) > 0 {
			newIDs := lo.Map(ids, func(newID string, _ int) string {
				if oldID, ok := idsMap[newID]; ok {
					return oldID
				} else {
					return newID
				}
			})
			d.Fields[k] = pbtypes.StringList(newIDs)
		}
	}

	return d
}

func normalizeDetails(d *types.Struct) *types.Struct {
	delete(d.Fields, bundle.RelationKeyId.String())
	delete(d.Fields, bundle.RelationKeyWorkspaceId.String())
	delete(d.Fields, bundle.RelationKeyLastModifiedBy.String())
	delete(d.Fields, bundle.RelationKeyLastModifiedDate.String())
	delete(d.Fields, bundle.RelationKeyCreatedDate.String())
	delete(d.Fields, bundle.RelationKeyLinks.String())
	delete(d.Fields, bundle.RelationKeyOldAnytypeID.String())
	delete(d.Fields, bundle.RelationKeyCreator.String())
	delete(d.Fields, bundle.RelationKeySource.String())
	return d
}
