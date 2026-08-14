package v2service

// DELETE /v2/spaces/{spaceId}/objects/{objectId} service tests
// (APIV2_OBJECT_DELETE.md §15). The fixture discipline: deleteFixture wires
// the FULL allow shape — live object, store row, matching provenance,
// matching caller — and every refusal case flips exactly ONE input. The
// success case is what makes each refusal meaningful: a fixture that can
// only refuse cannot distinguish "refused because legacy" from "refused
// because the check is broken". The ClientCommands mock is strict, so any
// path that archives when it must not (an error path falling through to the
// RPC) fails on the unexpected call, not silently.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const deleteObjId = "objDel1"

// callerCtx is the request context the auth middleware produces for a key
// named "Claude Desktop" — the RAW name, exactly as the app link records it.
func callerCtx() context.Context {
	return domain.CtxWithIntegrationName(context.Background(), "Claude Desktop")
}

// deleteRead is the live read of an ordinary deletable page.
func deleteRead(sbType model.SmartBlockType) apicore.ObjectRead {
	return apicore.ObjectRead{
		SbType: sbType,
		Snapshot: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String(deleteObjId),
				"name": pbtypes.String("Doomed"),
			}},
			ObjectTypes: []string{"ot-page"},
			Blocks:      []*model.Block{{Id: deleteObjId, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		},
		Heads: []string{"headA"},
	}
}

// newDeleteFixture wires the COMPLETE allow shape; tests then break one
// input each. recordedName parameterizes what provenance recorded — a RAW
// app name (§5: never a slug).
func newDeleteFixture(t *testing.T, accountMatch bool, recordedName string) *v2Fixture {
	fx := newV2Fixture(t)
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(model.SmartBlockType_Page), nil).Maybe()
	fx.provenanceMock.EXPECT().CreatorProvenance(mock.Anything, testSpaceId, deleteObjId).Return(accountMatch, recordedName, nil).Maybe()
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String(deleteObjId),
		bundle.RelationKeyName:           domain.String("Doomed"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
	}})
	return fx
}

func requireNotCreatedByThisKey(t *testing.T, err error) *v2model.Error {
	t.Helper()
	var v2Err *v2model.Error
	require.ErrorAs(t, err, &v2Err)
	assert.Equal(t, http.StatusForbidden, v2Err.Status)
	assert.Equal(t, v2model.CodeNotCreatedByThisKey, v2Err.Code)
	// every ownership refusal carries the §9.5 probe hint
	require.NotEmpty(t, v2Err.Issues)
	assert.Contains(t, v2Err.Issues[0].Hint, "dry_run=true")
	return v2Err
}

func TestDeleteObject(t *testing.T) {
	t.Run("the allow shape archives", func(t *testing.T) {
		// given: created by this account via this key — the ONE shape that
		// may archive. This case is what gives every refusal below its
		// meaning: the same fixture with one input flipped must refuse.
		fx := newDeleteFixture(t, true, "Claude Desktop")
		fx.mwMock.On("ObjectSetIsArchived", mock.Anything, &pb.RpcObjectSetIsArchivedRequest{
			ContextId: deleteObjId, IsArchived: true,
		}).Return(&pb.RpcObjectSetIsArchivedResponse{
			Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL},
		}).Once()
		want := &v2model.CreateResult{Id: deleteObjId, Type: "page"}

		// when
		got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("unknown object is a 404", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "ghost").Return(apicore.ObjectRead{}, treestorage.ErrUnknownTreeId).Once()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, "ghost", false)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusNotFound, v2Err.Status)
	})

	t.Run("a tombstoned row is a 404 even though its tree survives", func(t *testing.T) {
		// given: the corpse shape — the live read still answers (derived
		// trees survive deletion) but the row says isDeleted
		fx := newDeleteFixture(t, true, "Claude Desktop")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:        domain.String(deleteObjId),
			bundle.RelationKeyIsDeleted: domain.Bool(true),
		}})

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusNotFound, v2Err.Status)
	})

	t.Run("type and property targets are steered to their own routes", func(t *testing.T) {
		// provenance mock deliberately has NO expectation here: the steer
		// must fire BEFORE the provenance read, or a type created via this
		// key would archive through the wrong route
		cases := []struct {
			sbType model.SmartBlockType
			hint   string
		}{
			{model.SmartBlockType_STType, "/types/"},
			{model.SmartBlockType_STRelation, "/properties/"},
			{model.SmartBlockType_STRelationOption, "options"},
		}
		for _, tc := range cases {
			fx := newV2Fixture(t)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(tc.sbType), nil).Once()

			_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

			var v2Err *v2model.Error
			require.ErrorAs(t, err, &v2Err)
			assert.Equal(t, http.StatusBadRequest, v2Err.Status, tc.sbType.String())
			assert.Equal(t, v2model.CodeValidationFailed, v2Err.Code, tc.sbType.String())
			require.NotEmpty(t, v2Err.Issues, tc.sbType.String())
			assert.Contains(t, v2Err.Issues[0].Hint, tc.hint, tc.sbType.String())
		}
	})

	t.Run("system objects refuse whatever provenance would say — the F1 allowlist", func(t *testing.T) {
		// The claim "derived trees can never pass the root clause" was false:
		// derivePersonalPayload signs derived roots with the account identity
		// (personal space; and UseAccountSignature = every FileObject), so
		// provenance CAN answer accountMatch=true for system objects. These
		// cases are deliberately NON-steered types — a test using only the
		// steered trio would pass with no allowlist at all. No provenance
		// expectation is set: reaching the read fails the test, which pins
		// that the allowlist short-circuits BEFORE provenance; and under an
		// allowlist revert the strict mocks fail on the unexpected calls.
		for _, sbType := range []model.SmartBlockType{
			model.SmartBlockType_Workspace,
			model.SmartBlockType_Archive,
			model.SmartBlockType_Home,
			model.SmartBlockType_Widget,
			model.SmartBlockType_SpaceView,
			model.SmartBlockType_Participant,
			model.SmartBlockType_ProfilePage,
			model.SmartBlockType_Date,
			model.SmartBlockType_ChatObjectDeprecated,
		} {
			fx := newV2Fixture(t)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(sbType), nil).Once()

			_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

			var v2Err *v2model.Error
			require.ErrorAs(t, err, &v2Err, sbType.String())
			assert.Equal(t, http.StatusForbidden, v2Err.Status, sbType.String())
			assert.Equal(t, v2model.CodeForbidden, v2Err.Code, sbType.String())
			assert.Contains(t, v2Err.Message, "user content only", sbType.String())
		}
	})

	t.Run("user-content types pass the allowlist — files and templates stay deletable", func(t *testing.T) {
		// the other direction: an allowlist that quietly excluded FileObject
		// (the very shape F1 found passing the root clause — account-signed
		// derived roots) would break legitimate own-file deletion. Full
		// provenance match → archive succeeds.
		for _, sbType := range []model.SmartBlockType{
			model.SmartBlockType_FileObject,
			model.SmartBlockType_Template,
		} {
			fx := newV2Fixture(t)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(sbType), nil).Once()
			fx.provenanceMock.EXPECT().CreatorProvenance(mock.Anything, testSpaceId, deleteObjId).Return(true, "Claude Desktop", nil).Once()
			fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String(deleteObjId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			}})
			fx.mwMock.On("ObjectSetIsArchived", mock.Anything, mock.Anything).Return(&pb.RpcObjectSetIsArchivedResponse{
				Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL},
			}).Once()

			_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

			require.NoError(t, err, sbType.String())
		}
	})

	t.Run("no recorded key refuses — the legacy/app/import shape", func(t *testing.T) {
		// same fixture as the allow case, ONLY the recorded name removed —
		// so this refusal is attributable to the missing record, not to a
		// broken check (the fixture rule)
		fx := newDeleteFixture(t, true, "")

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

		v2Err := requireNotCreatedByThisKey(t, err)
		assert.Contains(t, v2Err.Message, "no API key is recorded")
		assert.Contains(t, v2Err.Message, "archive it in the Anytype app")
	})

	t.Run("a different integration's object refuses, naming both apps", func(t *testing.T) {
		fx := newDeleteFixture(t, true, "Linear")

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

		v2Err := requireNotCreatedByThisKey(t, err)
		assert.Contains(t, v2Err.Message, `"Linear"`)
		assert.Contains(t, v2Err.Message, `"Claude Desktop"`)
	})

	t.Run("names that normalize identically are DIFFERENT principals — the F2 regression", func(t *testing.T) {
		// given: recorded "Claude Desktop", caller "Claude/Desktop". The old
		// slug normalization collapsed both to claude-desktop, so a key
		// paired under the visibly different name could archive this object
		// end-to-end — a consent dialog showed the user one string while the
		// system treated it as another principal. This fixture is the pair
		// that proves exact match: two names ONE slug apart. A revert to
		// normalized comparison archives (the strict mock fails on the
		// unexpected RPC) instead of refusing.
		fx := newDeleteFixture(t, true, "Claude Desktop")
		ctx := domain.CtxWithIntegrationName(context.Background(), "Claude/Desktop")

		_, err := fx.DeleteObject(ctx, testSpaceId, deleteObjId, false)

		v2Err := requireNotCreatedByThisKey(t, err)
		assert.Contains(t, v2Err.Message, `"Claude Desktop"`)
		assert.Contains(t, v2Err.Message, `"Claude/Desktop"`)
		assert.Contains(t, v2Err.Message, "compared exactly")
	})

	t.Run("a non-Latin app name can delete its own output — the F3 regression", func(t *testing.T) {
		// given: recorded and caller both "日本語アプリ". The old normalization
		// slugged every Cyrillic/CJK/emoji name to "", so such a key's
		// objects were permanently unprovenanced and undeletable by their
		// own creator, with no signal at pairing time. An all-ASCII fixture
		// could never catch this; a revert that normalizes the caller name
		// turns it empty and this DELETE refuses as "nameless".
		fx := newDeleteFixture(t, true, "日本語アプリ")
		fx.mwMock.On("ObjectSetIsArchived", mock.Anything, &pb.RpcObjectSetIsArchivedRequest{
			ContextId: deleteObjId, IsArchived: true,
		}).Return(&pb.RpcObjectSetIsArchivedResponse{
			Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL},
		}).Once()
		ctx := domain.CtxWithIntegrationName(context.Background(), "日本語アプリ")

		got, err := fx.DeleteObject(ctx, testSpaceId, deleteObjId, false)

		require.NoError(t, err)
		assert.Equal(t, &v2model.CreateResult{Id: deleteObjId, Type: "page"}, got)
	})

	t.Run("another member's object refuses", func(t *testing.T) {
		fx := newDeleteFixture(t, false, "")

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

		v2Err := requireNotCreatedByThisKey(t, err)
		assert.Contains(t, v2Err.Message, "another space member")
	})

	t.Run("a nameless caller can never delete, even its own output", func(t *testing.T) {
		// recorded provenance exists; the CALLER has no app name (§5 empty
		// AppName) — flip is on the caller side only
		fx := newDeleteFixture(t, true, "Claude Desktop")

		_, err := fx.DeleteObject(context.Background(), testSpaceId, deleteObjId, false)

		v2Err := requireNotCreatedByThisKey(t, err)
		assert.Contains(t, v2Err.Message, "no recorded app name")
		assert.Contains(t, v2Err.Message, `"Claude Desktop"`)
	})

	t.Run("a provenance read failure refuses and never archives", func(t *testing.T) {
		// fail-closed on the ERROR path: the strict mw mock has no
		// ObjectSetIsArchived expectation, so falling through to the archive
		// fails the test on the unexpected call
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(model.SmartBlockType_Page), nil).Once()
		fx.provenanceMock.EXPECT().CreatorProvenance(mock.Anything, testSpaceId, deleteObjId).Return(false, "", errors.New("storage failure")).Once()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

		require.Error(t, err)
		var v2Err *v2model.Error
		assert.False(t, errors.As(err, &v2Err), "an infrastructure failure is a 500, not a shaped refusal")
	})

	t.Run("a nil provenance dependency refuses and never archives", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.V2Service = NewV2Service(fx.mwMock, fx.readerMock, fx.creatorMock, fx.mutatorMock, nil, fx.objectStore, objectstore.TestTechSpaceId, testAccountId)
		fx.registerSpace(t, testSpaceId)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(model.SmartBlockType_Page), nil).Once()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not configured")
	})

	t.Run("already archived is an idempotent 200 with a warning", func(t *testing.T) {
		// no ObjectSetIsArchived expectation: the no-op must not re-archive
		fx := newDeleteFixture(t, true, "Claude Desktop")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:         domain.String(deleteObjId),
			bundle.RelationKeyIsArchived: domain.Bool(true),
		}})
		want := &v2model.CreateResult{
			Id: deleteObjId, Type: "page",
			Warnings: []v2model.Issue{{Message: "already archived"}},
		}

		got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("dry run allowed: full verdict, no archive", func(t *testing.T) {
		// C9's real-not-writing assertion: allowed verdict, and the strict
		// mw mock proves nothing was written
		fx := newDeleteFixture(t, true, "Claude Desktop")
		want := &v2model.CreateResult{Id: deleteObjId, Type: "page", DryRun: true}

		got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, true)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("dry run refused: the same 403 as the real call", func(t *testing.T) {
		fx := newDeleteFixture(t, true, "Linear")

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, true)

		requireNotCreatedByThisKey(t, err)
	})

	t.Run("the grant conjunction fires before provenance", func(t *testing.T) {
		// scoped-key ordering (§9.3): space_not_granted / write_not_granted
		// win over the ownership check. No reader and no provenance
		// expectations — reaching either fails the test, which is what pins
		// the ordering rather than just the final code.
		t.Run("space not granted", func(t *testing.T) {
			fx := newV2Fixture(t)
			ctx := util.CtxWithApiGrant(callerCtx(), &util.ApiGrant{Spaces: []string{"someOtherSpace"}, Perms: util.GrantPermsReadWrite})

			_, err := fx.DeleteObject(ctx, testSpaceId, deleteObjId, false)

			requireSpaceNotGranted(t, err)
		})
		t.Run("write not granted", func(t *testing.T) {
			fx := newV2Fixture(t)
			ctx := util.CtxWithApiGrant(callerCtx(), &util.ApiGrant{Spaces: []string{testSpaceId}, Perms: util.GrantPermsRead})

			_, err := fx.DeleteObject(ctx, testSpaceId, deleteObjId, false)

			var v2Err *v2model.Error
			require.ErrorAs(t, err, &v2Err)
			assert.Equal(t, v2model.CodeWriteNotGranted, v2Err.Code)
		})
	})

	t.Run("an archive-RPC restriction refusal maps to a permanent 403", func(t *testing.T) {
		fx := newDeleteFixture(t, true, "Claude Desktop")
		fx.mwMock.On("ObjectSetIsArchived", mock.Anything, mock.Anything).Return(&pb.RpcObjectSetIsArchivedResponse{
			Error: &pb.RpcObjectSetIsArchivedResponseError{
				Code:        pb.RpcObjectSetIsArchivedResponseError_UNKNOWN_ERROR,
				Description: "restricted",
			},
		}).Once()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusForbidden, v2Err.Status)
		assert.Equal(t, v2model.CodeForbidden, v2Err.Code)
		assert.Contains(t, v2Err.Message, "do not retry")
	})

	t.Run("the file-ownership refusal is a permanent 403, not a 500 — F4", func(t *testing.T) {
		// CanDeleteFile's refusal reads "can't delete other's file" — it does
		// NOT contain "restricted", so a match on that word alone (the first
		// build) let this permanent refusal fall through to the retry-shaped
		// 500 branch. The fixture uses the exact fileobject/service.go text;
		// it fails if either textual match is narrowed back.
		fx := newDeleteFixture(t, true, "Claude Desktop")
		fx.mwMock.On("ObjectSetIsArchived", mock.Anything, mock.Anything).Return(&pb.RpcObjectSetIsArchivedResponse{
			Error: &pb.RpcObjectSetIsArchivedResponseError{
				Code:        pb.RpcObjectSetIsArchivedResponseError_UNKNOWN_ERROR,
				Description: "can't delete other's file",
			},
		}).Once()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusForbidden, v2Err.Status)
		assert.Equal(t, v2model.CodeForbidden, v2Err.Code)
		assert.Contains(t, v2Err.Message, "do not retry")
	})

	t.Run("an archive-RPC failure surfaces as an internal error", func(t *testing.T) {
		fx := newDeleteFixture(t, true, "Claude Desktop")
		fx.mwMock.On("ObjectSetIsArchived", mock.Anything, mock.Anything).Return(&pb.RpcObjectSetIsArchivedResponse{
			Error: &pb.RpcObjectSetIsArchivedResponseError{
				Code:        pb.RpcObjectSetIsArchivedResponseError_UNKNOWN_ERROR,
				Description: "boom",
			},
		}).Once()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "archive object")
	})
}
