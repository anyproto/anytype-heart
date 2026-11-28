package v2service

// DELETE /v2/spaces/{space_id}/objects/{object_id} and POST …/restore service
// tests (APIV2_OBJECT_DELETE.md §15). The fixture discipline: deleteFixture
// wires the FULL allow shape — live object, store row, matching provenance,
// matching caller — and every refusal case flips exactly ONE input. The
// success case is what makes each refusal meaningful: a fixture that can
// only refuse cannot distinguish "refused because legacy" from "refused
// because the check is broken". The ClientCommands mock is strict, so any
// path that writes when it must not (an error path falling through to the
// RPC) fails on the unexpected call, not silently.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/restriction"
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

// ownProvenance is the provenance of an object this key created and nobody
// else touched — the one shape a permanent delete accepts.
func ownProvenance() apicore.Provenance {
	return apicore.Provenance{AccountMatch: true, IntegrationName: "Claude Desktop"}
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
// input each. prov parameterizes what provenance recorded — a RAW app name
// (§5: never a slug).
func newDeleteFixture(t *testing.T, prov apicore.Provenance) *v2Fixture {
	return newDeleteFixtureOf(t, model.SmartBlockType_Page, prov)
}

func newDeleteFixtureOf(t *testing.T, sbType model.SmartBlockType, prov apicore.Provenance) *v2Fixture {
	return newDeleteFixtureRead(t, deleteRead(sbType), prov)
}

func newDeleteFixtureRead(t *testing.T, read apicore.ObjectRead, prov apicore.Provenance) *v2Fixture {
	fx := newV2Fixture(t)
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(read, nil).Maybe()
	// Bin membership agrees with the read's detail here; the tests about
	// the two disagreeing wire their own
	fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, deleteObjId).Return(isArchivedRead(read), nil).Maybe()
	fx.provenanceMock.EXPECT().CreatorProvenance(mock.Anything, testSpaceId, deleteObjId).Return(prov, nil).Maybe()
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String(deleteObjId),
		bundle.RelationKeyName:           domain.String("Doomed"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
	}})
	return fx
}

// pageReadOf is the live read of a page, in the Bin or not.
func pageReadOf(archived bool) apicore.ObjectRead {
	if archived {
		return archivedRead(model.SmartBlockType_Page)
	}
	return deleteRead(model.SmartBlockType_Page)
}

// archivedRead is deleteRead of an object whose isArchived detail is set —
// what edit restrictions read.
func archivedRead(sbType model.SmartBlockType) apicore.ObjectRead {
	read := deleteRead(sbType)
	read.Snapshot.Details.Fields[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(true)
	return read
}

// markRowArchived sets the INDEXED row's Bin state alone — the lagging
// source the routes must not trust.
func (fx *v2Fixture) markRowArchived(t *testing.T, archived bool) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String(deleteObjId),
		bundle.RelationKeyName:           domain.String("Doomed"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyIsArchived:     domain.Bool(archived),
	}})
}

func (fx *v2Fixture) expectArchive(archived bool) {
	fx.mwMock.On("ObjectSetIsArchived", mock.Anything, &pb.RpcObjectSetIsArchivedRequest{
		ContextId: deleteObjId, IsArchived: archived,
	}).Return(&pb.RpcObjectSetIsArchivedResponse{
		Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL},
	}).Once()
}

func (fx *v2Fixture) expectHardDelete() {
	fx.mwMock.On("ObjectListDelete", mock.Anything, &pb.RpcObjectListDeleteRequest{
		ObjectIds: []string{deleteObjId},
	}).Return(&pb.RpcObjectListDeleteResponse{
		Error: &pb.RpcObjectListDeleteResponseError{Code: pb.RpcObjectListDeleteResponseError_NULL},
	}).Once()
}

// binWarning returns the receipt's one Bin notice.
func binWarning(t *testing.T, result *v2model.CreateResult) v2model.Issue {
	t.Helper()
	require.Len(t, result.Warnings, 1)
	return result.Warnings[0]
}

// requireOps asserts the issue's typed references, in order.
func requireOps(t *testing.T, issue v2model.Issue, want ...v2model.Ref) {
	t.Helper()
	assert.Equal(t, want, issue.SeeAlso)
}

func requireNotCreatedByThisKey(t *testing.T, err error) *v2model.Error {
	t.Helper()
	var v2Err *v2model.Error
	require.ErrorAs(t, err, &v2Err)
	assert.Equal(t, http.StatusForbidden, v2Err.Status)
	assert.Equal(t, v2model.CodeNotCreatedByThisKey, v2Err.Code)
	// every ownership refusal carries the §9.5 probe hint, which names the
	// archive as the alternative; the message keeps the ownership reason
	// alone, so the alternative is said once and for the run it rides on
	require.NotEmpty(t, v2Err.Issues)
	assert.Contains(t, v2Err.Issues[0].Hint, "dry_run=true")
	assert.Contains(t, v2Err.Issues[0].Hint, "without permanent, DELETE moves the object to the Bin")
	assert.NotContains(t, v2Err.Message, "Without permanent")
	return v2Err
}

func TestDeleteObjectArchives(t *testing.T) {
	restore := v2model.RefRestoreObject(testSpaceId, deleteObjId)
	permanent := v2model.Resend("permanent", "true")

	t.Run("an object this key created: archived, and the receipt offers the permanent resend", func(t *testing.T) {
		// given
		fx := newDeleteFixture(t, ownProvenance())
		fx.expectArchive(true)

		// when
		got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, deleteObjId, got.Id)
		assert.Equal(t, "page", got.Type)
		assert.False(t, got.Permanent)
		notice := binWarning(t, got)
		assert.Equal(t, "moved to the Bin", notice.Message)
		assert.Contains(t, notice.Hint, "permanent=true")
		requireOps(t, notice, restore, permanent)
	})

	t.Run("objects this key did not create are archived too, and the receipt says permanent is closed", func(t *testing.T) {
		// the old own-output gate refused every one of these shapes; archive
		// is reversible, so they now archive, and only the permanent clause
		// of the receipt differs
		cases := map[string]apicore.Provenance{
			"another integration": {AccountMatch: true, IntegrationName: "Linear"},
			"no recorded key":     {AccountMatch: true},
			"another member":      {},
		}
		for name, prov := range cases {
			fx := newDeleteFixture(t, prov)
			fx.expectArchive(true)

			got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, false)

			require.NoError(t, err, name)
			notice := binWarning(t, got)
			assert.Contains(t, notice.Hint, "only objects this key created can be deleted permanently", name)
			requireOps(t, notice, restore)
		}
	})

	t.Run("an own object another member edited: archived, permanent closed for that reason", func(t *testing.T) {
		prov := ownProvenance()
		prov.EditedByOthers = true
		fx := newDeleteFixture(t, prov)
		fx.expectArchive(true)

		got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, false)

		require.NoError(t, err)
		assert.Contains(t, binWarning(t, got).Hint, "changes by another space member")
	})

	t.Run("a provenance read failure does not fail the archive; the receipt drops the permanent clause", func(t *testing.T) {
		// the notice is advice: the archive succeeded, and an infrastructure
		// failure in the permanent verdict must not turn that into a 500.
		// The dry run drops the clause too, and still says the object is not
		// in the Bin yet
		cases := []struct {
			dryRun bool
			want   v2model.Issue
		}{
			{dryRun: false, want: v2model.Issue{Path: "object_id", Message: "moved to the Bin"}.
				Hintf("restore it with %s", restore)},
			{dryRun: true, want: v2model.Issue{Path: "object_id", Message: "a real request moves this object to the Bin"}.
				Hintf("once it is in the Bin, restore it with %s", restore.With("dry_run", "true"))},
		}
		for _, tc := range cases {
			// given
			fx := newV2Fixture(t)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(model.SmartBlockType_Page), nil).Once()
			fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, deleteObjId).Return(false, nil).Maybe()
			fx.provenanceMock.EXPECT().CreatorProvenance(mock.Anything, testSpaceId, deleteObjId).Return(apicore.Provenance{}, errors.New("storage failure")).Once()
			if !tc.dryRun {
				fx.expectArchive(true)
			}

			// when
			got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, tc.dryRun, false)

			// then
			require.NoError(t, err, "dryRun=%v", tc.dryRun)
			assert.Equal(t, tc.want, binWarning(t, got), "dryRun=%v", tc.dryRun)
		}
	})

	t.Run("unknown object is a 404", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "ghost").Return(apicore.ObjectRead{}, treestorage.ErrUnknownTreeId).Once()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, "ghost", false, false)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusNotFound, v2Err.Status)
	})

	t.Run("a tombstoned row is a 410 even though its tree survives", func(t *testing.T) {
		// given: the corpse shape — the live read still answers (derived
		// trees survive deletion) but the row says isDeleted
		fx := newDeleteFixture(t, ownProvenance())
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:        domain.String(deleteObjId),
			bundle.RelationKeyIsDeleted: domain.Bool(true),
		}})
		want := v2model.ObjectDeleted(testSpaceId, deleteObjId)

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, false)

		assert.Equal(t, want, err)
	})

	t.Run("a deleted object is a 410 for a permanent delete too", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).
			Return(apicore.ObjectRead{}, fmt.Errorf("read live object state: %w", spacestorage.ErrTreeStorageAlreadyDeleted)).Once()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

		assert.Equal(t, v2model.ObjectDeleted(testSpaceId, deleteObjId), err)
	})

	t.Run("an older build's type tombstone is not a deleted object either", func(t *testing.T) {
		// given: the layout only in deletedSnapshot, the tree surviving
		fx := newDeleteFixtureOf(t, model.SmartBlockType_STType, ownProvenance())
		fx.addTypeTombstone(t, deleteObjId)

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, false)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusBadRequest, v2Err.Status)
	})

	t.Run("an uninstalled type's row is not a deleted object: the steer still answers", func(t *testing.T) {
		// given: heart sets isDeleted on an uninstalled type and keeps its
		// layout; it is removed, not deleted for good
		fx := newDeleteFixtureOf(t, model.SmartBlockType_STType, ownProvenance())
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String(deleteObjId),
			bundle.RelationKeyIsDeleted:      domain.Bool(true),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}})

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, false)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusBadRequest, v2Err.Status)
		assert.Contains(t, v2Err.Message, "type")
	})

	t.Run("type and property targets are steered to their own routes", func(t *testing.T) {
		// provenance mock deliberately has NO expectation here: the steer
		// must fire BEFORE any provenance read
		cases := []struct {
			sbType model.SmartBlockType
			hint   string
		}{
			{model.SmartBlockType_STType, "/types/"},
			{model.SmartBlockType_STRelation, "/properties/"},
			{model.SmartBlockType_STRelationOption, "options"},
		}
		for _, tc := range cases {
			for _, permanent := range []bool{false, true} {
				fx := newV2Fixture(t)
				fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(tc.sbType), nil).Once()

				_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, permanent)

				var v2Err *v2model.Error
				require.ErrorAs(t, err, &v2Err)
				assert.Equal(t, http.StatusBadRequest, v2Err.Status, tc.sbType.String())
				assert.Equal(t, v2model.CodeValidationFailed, v2Err.Code, tc.sbType.String())
				require.NotEmpty(t, v2Err.Issues, tc.sbType.String())
				assert.Contains(t, v2Err.Issues[0].Hint, tc.hint, tc.sbType.String())
			}
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
			for _, permanent := range []bool{false, true} {
				fx := newV2Fixture(t)
				fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(sbType), nil).Once()

				_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, permanent)

				var v2Err *v2model.Error
				require.ErrorAs(t, err, &v2Err, sbType.String())
				assert.Equal(t, http.StatusForbidden, v2Err.Status, sbType.String())
				assert.Equal(t, v2model.CodeForbidden, v2Err.Code, sbType.String())
				assert.Contains(t, v2Err.Message, "user content only", sbType.String())
			}
		}
	})

	t.Run("user-content types pass the allowlist — files and templates archive", func(t *testing.T) {
		for _, sbType := range []model.SmartBlockType{
			model.SmartBlockType_FileObject,
			model.SmartBlockType_Template,
		} {
			fx := newDeleteFixtureOf(t, sbType, apicore.Provenance{AccountMatch: true, IntegrationName: "Linear"})
			fx.expectArchive(true)

			_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, false)

			require.NoError(t, err, sbType.String())
		}
	})

	t.Run("already archived is an idempotent 200 with a warning", func(t *testing.T) {
		// no ObjectSetIsArchived expectation: the no-op must not re-archive
		fx := newDeleteFixtureRead(t, archivedRead(model.SmartBlockType_Page), ownProvenance())

		got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, false)

		require.NoError(t, err)
		notice := binWarning(t, got)
		assert.Equal(t, "already archived", notice.Message)
		requireOps(t, notice, restore, permanent)
	})

	t.Run("dry run: the verdict and the notice, no archive", func(t *testing.T) {
		// C9's real-not-writing assertion: the strict mw mock proves nothing
		// was written
		fx := newDeleteFixture(t, apicore.Provenance{AccountMatch: true, IntegrationName: "Linear"})

		got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, true, false)

		require.NoError(t, err)
		assert.True(t, got.DryRun)
		assert.Equal(t, "a real request moves this object to the Bin", binWarning(t, got).Message)
	})

	t.Run("the Bin notice speaks for the run it rides on", func(t *testing.T) {
		// a dry run moved nothing, so its notice says what the real request
		// does: the object is not in the Bin to restore from yet, and a
		// resend of a dry run with permanent=true is still a dry run, so it
		// previews the delete for good (the wrapper renders the resend as
		// "this call again", dry_run and all)
		other := apicore.Provenance{AccountMatch: true, IntegrationName: "Linear"}
		// a preview's resend carries dry_run itself: copied as a REST query in
		// place of the request's, a bare permanent=true deletes for good
		previewPermanent := v2model.Resend("permanent", "true").With("dry_run", "true")
		// and so does its restore: following it from a preview restores nothing
		previewRestore := restore.With("dry_run", "true")
		cases := []struct {
			name    string
			read    apicore.ObjectRead
			prov    apicore.Provenance
			dryRun  bool
			archive bool
			want    v2model.Issue
		}{
			{
				name:   "dry run of an own object",
				read:   deleteRead(model.SmartBlockType_Page),
				prov:   ownProvenance(),
				dryRun: true,
				want: v2model.Issue{Path: "object_id", Message: "a real request moves this object to the Bin"}.
					Hintf("once it is in the Bin, restore it with %s; this key created it, so resending with %s previews deleting it for good", previewRestore, previewPermanent),
			},
			{
				name:   "dry run of another key's object",
				read:   deleteRead(model.SmartBlockType_Page),
				prov:   other,
				dryRun: true,
				want: v2model.Issue{Path: "object_id", Message: "a real request moves this object to the Bin"}.
					Hintf("once it is in the Bin, restore it with %s; only objects this key created can be deleted permanently", previewRestore),
			},
			{
				name:   "dry run of an own object already in the Bin",
				read:   archivedRead(model.SmartBlockType_Page),
				prov:   ownProvenance(),
				dryRun: true,
				want: v2model.Issue{Path: "object_id", Message: "already archived"}.
					Hintf("restore it with %s; this key created it, so resending with %s previews deleting it for good", previewRestore, previewPermanent),
			},
			{
				name:    "real delete of an own object keeps its wording",
				read:    deleteRead(model.SmartBlockType_Page),
				prov:    ownProvenance(),
				archive: true,
				want: v2model.Issue{Path: "object_id", Message: "moved to the Bin"}.
					Hintf("restore it with %s; this key created it, so resending with %s deletes it for good", restore, permanent),
			},
			{
				name:    "real delete of another key's object keeps its wording",
				read:    deleteRead(model.SmartBlockType_Page),
				prov:    other,
				archive: true,
				want: v2model.Issue{Path: "object_id", Message: "moved to the Bin"}.
					Hintf("restore it with %s; only objects this key created can be deleted permanently", restore),
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				fx := newDeleteFixtureRead(t, tc.read, tc.prov)
				if tc.archive {
					fx.expectArchive(true)
				}

				// when
				got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, tc.dryRun, false)

				// then
				require.NoError(t, err)
				assert.Equal(t, tc.dryRun, got.DryRun)
				assert.Equal(t, tc.want, binWarning(t, got))
			})
		}
	})

	t.Run("a schema steer keeps the request's mode, so following it from a dry run stays a dry run", func(t *testing.T) {
		// given: the type and property behind the steered ids, so the
		// followed route resolves them
		cases := []struct {
			sbType model.SmartBlockType
			op     string
		}{
			{model.SmartBlockType_STType, v2model.OpDeleteType},
			{model.SmartBlockType_STRelation, v2model.OpDeleteProperty},
		}
		for _, tc := range cases {
			for _, dryRun := range []bool{false, true} {
				fx := newV2Fixture(t)
				fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(tc.sbType), nil).Once()
				want := v2model.NewRef(tc.op, "space_id", testSpaceId)
				if dryRun {
					want = want.With("dry_run", "true")
				}

				// when
				_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, dryRun, false)

				// then
				got := v2Err(t, err)
				require.Len(t, got.Issues, 1)
				assert.Equal(t, []v2model.Ref{want}, got.Issues[0].SeeAlso, "%s, dryRun=%v", tc.sbType, dryRun)
			}
		}

		// and the dry run's reference, followed, writes nothing: the strict
		// mw mock has no archive expectation
		fx := newSchemaDeleteFixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(model.SmartBlockType_STType), nil).Once()
		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, true, false)
		ref := v2Err(t, err).Issues[0].SeeAlso[0]
		fx.expectNotInBin("type-chore")
		followed, err := fx.DeleteType(callerCtx(), testSpaceId, "chore", ref.Query["dry_run"] == "true")
		require.NoError(t, err)
		assert.True(t, followed.DryRun)
	})

	t.Run("a file preview in a one-to-one space says the real request may still refuse", func(t *testing.T) {
		// the real archive checks the file's owner (CanDeleteFile) inside the
		// RPC, which a dry run never reaches; in a one-to-one space only the
		// member who added a file can delete it
		fx := newDeleteFixtureOf(t, model.SmartBlockType_FileObject, apicore.Provenance{AccountMatch: true, IntegrationName: "Linear"})
		fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("spaceView_" + testSpaceId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:  domain.String(testSpaceId),
			bundle.RelationKeySpaceUxType:    domain.Int64(int64(model.SpaceUxType_OneToOne)),
		}})
		want := v2model.Issue{Path: "object_id", Message: "a real request moves this file to the Bin if this account added it: in a one-to-one space only the member who added a file can delete it"}.
			Hintf("once it is in the Bin, restore it with %s; only objects this key created can be deleted permanently", restore.With("dry_run", "true"))

		// when
		got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, true, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, binWarning(t, got))
	})

	t.Run("an archive preview the object's restrictions refuse is refused as the real archive is", func(t *testing.T) {
		// a regular member previewing the archive of another member's page:
		// the member policy forbids deleting it (LockForeignDelete), and the
		// live read carries that verdict, so the preview refuses as the real
		// archive RPC does instead of promising a move
		read := deleteRead(model.SmartBlockType_Page)
		read.DeleteRefused = restriction.ErrRestricted
		fx := newDeleteFixtureRead(t, read, apicore.Provenance{AccountMatch: true, IntegrationName: "Linear"})
		want := restrictionForbidden(deleteObjId, restriction.ErrRestricted)

		// when
		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, true, false)

		// then
		assert.Equal(t, want, v2Err(t, err))
	})

	t.Run("the grant conjunction fires first", func(t *testing.T) {
		// scoped-key ordering (§9.3): space_not_granted / write_not_granted
		// win over everything. No reader and no provenance expectations —
		// reaching either fails the test, which is what pins the ordering
		// rather than just the final code.
		t.Run("space not granted", func(t *testing.T) {
			fx := newV2Fixture(t)
			ctx := util.CtxWithApiGrant(callerCtx(), &util.ApiGrant{Spaces: []string{"someOtherSpace"}, Perms: util.GrantPermsReadWrite})

			_, err := fx.DeleteObject(ctx, testSpaceId, deleteObjId, false, false)

			requireSpaceNotGranted(t, err)
		})
		t.Run("write not granted", func(t *testing.T) {
			fx := newV2Fixture(t)
			ctx := util.CtxWithApiGrant(callerCtx(), &util.ApiGrant{Spaces: []string{testSpaceId}, Perms: util.GrantPermsRead})

			_, err := fx.DeleteObject(ctx, testSpaceId, deleteObjId, false, false)

			var v2Err *v2model.Error
			require.ErrorAs(t, err, &v2Err)
			assert.Equal(t, v2model.CodeWriteNotGranted, v2Err.Code)
		})
	})

	t.Run("the archive RPC's permanent refusals map to a 403, not a 500 — F4", func(t *testing.T) {
		// CanDeleteFile's refusal reads "can't delete other's file" — it does
		// NOT contain "restricted", so a match on that word alone (the first
		// build) let this permanent refusal fall through to the retry-shaped
		// 500 branch. The fixture uses the exact fileobject/service.go text.
		for _, desc := range []string{"restricted", "can't delete other's file"} {
			fx := newDeleteFixture(t, ownProvenance())
			fx.mwMock.On("ObjectSetIsArchived", mock.Anything, mock.Anything).Return(&pb.RpcObjectSetIsArchivedResponse{
				Error: &pb.RpcObjectSetIsArchivedResponseError{
					Code:        pb.RpcObjectSetIsArchivedResponseError_UNKNOWN_ERROR,
					Description: desc,
				},
			}).Once()

			_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, false)

			var v2Err *v2model.Error
			require.ErrorAs(t, err, &v2Err, desc)
			assert.Equal(t, http.StatusForbidden, v2Err.Status, desc)
			assert.Equal(t, v2model.CodeForbidden, v2Err.Code, desc)
			assert.Contains(t, v2Err.Message, "do not retry", desc)
		}
	})

	t.Run("an archive-RPC failure surfaces as an internal error", func(t *testing.T) {
		fx := newDeleteFixture(t, ownProvenance())
		fx.mwMock.On("ObjectSetIsArchived", mock.Anything, mock.Anything).Return(&pb.RpcObjectSetIsArchivedResponse{
			Error: &pb.RpcObjectSetIsArchivedResponseError{
				Code:        pb.RpcObjectSetIsArchivedResponseError_UNKNOWN_ERROR,
				Description: "boom",
			},
		}).Once()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, false)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "archive "+deleteObjId)
	})
}

func TestDeleteObjectPermanently(t *testing.T) {
	t.Run("the allow shape deletes for good, archived first or not", func(t *testing.T) {
		// given: created by this account via this key, edited by nobody
		// else — the ONE shape that may be destroyed. This case is what
		// gives every refusal below its meaning.
		for _, archived := range []bool{false, true} {
			read := deleteRead(model.SmartBlockType_Page)
			if archived {
				read = archivedRead(model.SmartBlockType_Page)
			}
			fx := newDeleteFixtureRead(t, read, ownProvenance())
			fx.expectHardDelete()
			want := &v2model.CreateResult{Id: deleteObjId, Type: "page", Permanent: true}

			// when
			got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

			// then
			require.NoError(t, err, "archived=%v", archived)
			assert.Equal(t, want, got, "archived=%v", archived)
		}
	})

	t.Run("no recorded key refuses — the legacy/app/import shape", func(t *testing.T) {
		// same fixture as the allow case, ONLY the recorded name removed —
		// so this refusal is attributable to the missing record, not to a
		// broken check (the fixture rule)
		fx := newDeleteFixture(t, apicore.Provenance{AccountMatch: true})

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

		v2Err := requireNotCreatedByThisKey(t, err)
		assert.Contains(t, v2Err.Message, "no API key is recorded")
	})

	t.Run("a different integration's object refuses, naming both apps", func(t *testing.T) {
		fx := newDeleteFixture(t, apicore.Provenance{AccountMatch: true, IntegrationName: "Linear"})

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

		v2Err := requireNotCreatedByThisKey(t, err)
		assert.Contains(t, v2Err.Message, `"Linear"`)
		assert.Contains(t, v2Err.Message, `"Claude Desktop"`)
	})

	t.Run("names that normalize identically are DIFFERENT principals — the F2 regression", func(t *testing.T) {
		// given: recorded "Claude Desktop", caller "Claude/Desktop". The old
		// slug normalization collapsed both to claude-desktop. A revert to
		// normalized comparison deletes (the strict mock fails on the
		// unexpected RPC) instead of refusing.
		fx := newDeleteFixture(t, ownProvenance())
		ctx := domain.CtxWithIntegrationName(context.Background(), "Claude/Desktop")

		_, err := fx.DeleteObject(ctx, testSpaceId, deleteObjId, false, true)

		v2Err := requireNotCreatedByThisKey(t, err)
		assert.Contains(t, v2Err.Message, `"Claude Desktop"`)
		assert.Contains(t, v2Err.Message, `"Claude/Desktop"`)
		assert.Contains(t, v2Err.Message, "compared exactly")
	})

	t.Run("a non-Latin app name can delete its own output — the F3 regression", func(t *testing.T) {
		// the old normalization slugged every Cyrillic/CJK/emoji name to "",
		// so such a key's objects were undeletable by their own creator
		fx := newDeleteFixture(t, apicore.Provenance{AccountMatch: true, IntegrationName: "日本語アプリ"})
		fx.expectHardDelete()
		ctx := domain.CtxWithIntegrationName(context.Background(), "日本語アプリ")

		got, err := fx.DeleteObject(ctx, testSpaceId, deleteObjId, false, true)

		require.NoError(t, err)
		assert.True(t, got.Permanent)
	})

	t.Run("another member's object refuses", func(t *testing.T) {
		fx := newDeleteFixture(t, apicore.Provenance{})

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

		v2Err := requireNotCreatedByThisKey(t, err)
		assert.Contains(t, v2Err.Message, "another space member")
	})

	t.Run("a nameless caller can never delete permanently, even its own output", func(t *testing.T) {
		// recorded provenance exists; the CALLER has no app name (§5 empty
		// AppName) — flip is on the caller side only
		fx := newDeleteFixture(t, ownProvenance())

		_, err := fx.DeleteObject(context.Background(), testSpaceId, deleteObjId, false, true)

		v2Err := requireNotCreatedByThisKey(t, err)
		assert.Contains(t, v2Err.Message, "no recorded app name")
		assert.Contains(t, v2Err.Message, `"Claude Desktop"`)
	})

	t.Run("an own object another member edited refuses — their changes would go with it", func(t *testing.T) {
		prov := ownProvenance()
		prov.EditedByOthers = true
		fx := newDeleteFixture(t, prov)

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusForbidden, v2Err.Status)
		assert.Equal(t, v2model.CodeForbidden, v2Err.Code)
		assert.Contains(t, v2Err.Message, "no changes by another space member")
	})

	t.Run("an own object with a discussion refuses — its replies would be orphaned", func(t *testing.T) {
		// the discussion is a tree of its own: the co-author scan never sees
		// another member's reply in it
		read := deleteRead(model.SmartBlockType_Page)
		read.Snapshot.Details.Fields[bundle.RelationKeyDiscussionId.String()] = pbtypes.String("disc1")
		fx := newDeleteFixtureRead(t, read, ownProvenance())

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusForbidden, v2Err.Status)
		assert.Contains(t, v2Err.Message, "without a discussion")
	})

	t.Run("chats refuse before any provenance read", func(t *testing.T) {
		// no provenance expectation: chats never carry a stamp, and building
		// their history to learn that is the cost being avoided
		for _, sbType := range []model.SmartBlockType{model.SmartBlockType_ChatDerivedObject, model.SmartBlockType_DiscussionObject} {
			fx := newV2Fixture(t)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(sbType), nil).Once()
			fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, deleteObjId).Return(false, nil).Once()

			_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

			var v2Err *v2model.Error
			require.ErrorAs(t, err, &v2Err, sbType.String())
			assert.Equal(t, http.StatusForbidden, v2Err.Status, sbType.String())
			assert.Contains(t, v2Err.Message, "chats and discussions", sbType.String())
		}
	})

	t.Run("a provenance read failure refuses and never deletes", func(t *testing.T) {
		// fail-closed on the ERROR path: the strict mw mock has no
		// ObjectListDelete expectation
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(model.SmartBlockType_Page), nil).Once()
		fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, deleteObjId).Return(false, nil).Maybe()
		fx.provenanceMock.EXPECT().CreatorProvenance(mock.Anything, testSpaceId, deleteObjId).Return(apicore.Provenance{}, errors.New("storage failure")).Once()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

		require.Error(t, err)
		var v2Err *v2model.Error
		assert.False(t, errors.As(err, &v2Err), "an infrastructure failure is a 500, not a shaped refusal")
	})

	t.Run("a nil provenance dependency refuses and never deletes", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.Service = NewService(fx.mwMock, fx.readerMock, fx.creatorMock, fx.mutatorMock, nil, fx.widgetsMock, nil, nil, nil, nil, nil, nil, fx.objectStore, objectstore.TestTechSpaceId, testAccountId)
		fx.registerSpace(t, testSpaceId)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(model.SmartBlockType_Page), nil).Once()
		fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, deleteObjId).Return(false, nil).Maybe()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not configured")
	})

	t.Run("a file other objects still use refuses, naming them; deleted users do not count", func(t *testing.T) {
		// the dedup hazard: identical uploads share one file object, so a key
		// can "own" a file the person embedded elsewhere. Archived users
		// count — restoring one must not bring back a broken embed.
		fx := newDeleteFixtureOf(t, model.SmartBlockType_FileObject, ownProvenance())
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:    domain.String("embedder"),
				bundle.RelationKeyLinks: domain.StringList([]string{"other", deleteObjId}),
			},
			{
				bundle.RelationKeyId:         domain.String("binnedCover"),
				bundle.RelationKeyCoverId:    domain.String(deleteObjId),
				bundle.RelationKeyIsArchived: domain.Bool(true),
			},
			{
				bundle.RelationKeyId:        domain.String("deletedIcon"),
				bundle.RelationKeyIconImage: domain.String(deleteObjId),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
			},
		})

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusConflict, v2Err.Status)
		assert.Equal(t, v2model.CodeFileInUse, v2Err.Code)
		assert.Contains(t, v2Err.Message, "embedder")
		assert.Contains(t, v2Err.Message, "binnedCover")
		assert.NotContains(t, v2Err.Message, "deletedIcon")
	})

	t.Run("file properties are uses — a custom one, the icon, and a hidden object's icon — review finding 1/6", func(t *testing.T) {
		// links carry object-format relation values only, so a file held in
		// a file property never reached the links probe; and the workspace's
		// space icon is a hidden object's iconImage, a real use
		const fileProp = "6a76aa11c0ffee0000000001"
		cases := map[string]objectstore.TestObject{
			"custom file property": {
				bundle.RelationKeyId:         domain.String("holder"),
				domain.RelationKey(fileProp): domain.StringList([]string{"other", deleteObjId}),
			},
			"icon": {
				bundle.RelationKeyId:        domain.String("holder"),
				bundle.RelationKeyIconImage: domain.String(deleteObjId),
			},
			"hidden object's icon": {
				bundle.RelationKeyId:        domain.String("holder"),
				bundle.RelationKeyIconImage: domain.String(deleteObjId),
				bundle.RelationKeyIsHidden:  domain.Bool(true),
			},
		}
		for name, holder := range cases {
			fx := newDeleteFixtureOf(t, model.SmartBlockType_FileObject, ownProvenance())
			fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
				{
					bundle.RelationKeyId:             domain.String("rel-attachment"),
					bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
					bundle.RelationKeyRelationKey:    domain.String(fileProp),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_file)),
				},
				holder,
			})

			_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

			var v2Err *v2model.Error
			require.ErrorAs(t, err, &v2Err, name)
			assert.Equal(t, v2model.CodeFileInUse, v2Err.Code, name)
			assert.Contains(t, v2Err.Message, "holder", name)
		}
	})

	t.Run("a file nothing uses deletes; the Bin linking it is not a use — found live", func(t *testing.T) {
		// the Bin collection "links" everything in it, so an archived file
		// read as used by the Bin and could never be deleted
		fx := newDeleteFixtureOf(t, model.SmartBlockType_FileObject, ownProvenance())
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:        domain.String("deletedEmbedder"),
				bundle.RelationKeyLinks:     domain.StringList([]string{deleteObjId}),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
			},
			{
				bundle.RelationKeyId:       domain.String("theBin"),
				bundle.RelationKeyLinks:    domain.StringList([]string{deleteObjId}),
				bundle.RelationKeyIsHidden: domain.Bool(true),
			},
		})
		fx.expectHardDelete()

		got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

		require.NoError(t, err)
		assert.True(t, got.Permanent)
	})

	t.Run("dry run allowed: the verdict, no delete", func(t *testing.T) {
		fx := newDeleteFixture(t, ownProvenance())
		want := &v2model.CreateResult{Id: deleteObjId, Type: "page", DryRun: true, Permanent: true}

		got, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, true, true)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("an ownership refusal says the alternative once, for the run and the Bin state", func(t *testing.T) {
		// the message keeps the ownership reason; the hint says what dropping
		// permanent does: on a dry run it previews the archive, and for an
		// object already in the Bin it does nothing at all
		const message = `permanent deletion is limited to objects this API key created: this object was created via "Linear", not via this key ("Claude Desktop"); app names are compared exactly`
		refusal := func(issue v2model.Issue) *v2model.Error {
			return v2model.NotCreatedByThisKey(message, issue)
		}
		issue := v2model.Issue{Path: "object_id", Message: "this key may permanently delete only objects it created"}
		cases := []struct {
			name     string
			archived bool
			dryRun   bool
			want     *v2model.Error
		}{
			{
				name: "a real request",
				want: refusal(issue.Hintf("probe without writing by resending with %s; a read of the object shows who created it; without permanent, DELETE moves the object to the Bin",
					v2model.Resend("dry_run", "true").With("permanent", "true"))),
			},
			{
				name:   "a dry run",
				dryRun: true,
				want:   refusal(issue.WithHint(v2model.Plain("a read of the object shows who created it; without permanent, the dry run previews moving the object to the Bin"))),
			},
			{
				name:     "a real request for an object in the Bin",
				archived: true,
				want: refusal(issue.Hintf("probe without writing by resending with %s; a read of the object shows who created it; the object is already in the Bin",
					v2model.Resend("dry_run", "true").With("permanent", "true"))),
			},
			{
				name:     "a dry run for an object in the Bin",
				archived: true,
				dryRun:   true,
				want:     refusal(issue.WithHint(v2model.Plain("a read of the object shows who created it; the object is already in the Bin"))),
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				read := deleteRead(model.SmartBlockType_Page)
				if tc.archived {
					read = archivedRead(model.SmartBlockType_Page)
				}
				fx := newDeleteFixtureRead(t, read, apicore.Provenance{AccountMatch: true, IntegrationName: "Linear"})

				// when
				_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, tc.dryRun, true)

				// then
				assert.Equal(t, tc.want, v2Err(t, err))
			})
		}
	})

	t.Run("a refusal's archive alternative speaks for the run and the Bin state", func(t *testing.T) {
		// on a dry run, resending without permanent previews the archive; it
		// moves nothing, so the repair says so; and an object already in the
		// Bin has no archive left to offer
		editedProv := ownProvenance()
		editedProv.EditedByOthers = true
		discussed := func(read apicore.ObjectRead) apicore.ObjectRead {
			read.Snapshot.Details.Fields[bundle.RelationKeyDiscussionId.String()] = pbtypes.String("disc1")
			return read
		}
		cases := []struct {
			name     string
			fixture  func(t *testing.T, read apicore.ObjectRead) *v2Fixture
			read     func(archived bool) apicore.ObjectRead
			real     string
			dryRun   string
			archived string
		}{
			{
				name: "another member changed it",
				fixture: func(t *testing.T, read apicore.ObjectRead) *v2Fixture {
					return newDeleteFixtureRead(t, read, editedProv)
				},
				read:     func(archived bool) apicore.ObjectRead { return pageReadOf(archived) },
				real:     "without permanent, DELETE moves the object to the Bin, where it can be restored",
				dryRun:   "without permanent, the dry run previews moving the object to the Bin, where it can be restored",
				archived: "the object is already in the Bin, where it can be restored",
			},
			{
				name: "it has a discussion",
				fixture: func(t *testing.T, read apicore.ObjectRead) *v2Fixture {
					return newDeleteFixtureRead(t, read, ownProvenance())
				},
				read:     func(archived bool) apicore.ObjectRead { return discussed(pageReadOf(archived)) },
				real:     "without permanent, DELETE moves the object to the Bin, where it can be restored",
				dryRun:   "without permanent, the dry run previews moving the object to the Bin, where it can be restored",
				archived: "the object is already in the Bin, where it can be restored",
			},
			{
				name: "it is a chat",
				fixture: func(t *testing.T, read apicore.ObjectRead) *v2Fixture {
					return newDeleteFixtureRead(t, read, ownProvenance())
				},
				read: func(archived bool) apicore.ObjectRead {
					if archived {
						return archivedRead(model.SmartBlockType_ChatDerivedObject)
					}
					return deleteRead(model.SmartBlockType_ChatDerivedObject)
				},
				real:     "without permanent, DELETE moves it to the Bin",
				dryRun:   "without permanent, the dry run previews moving it to the Bin",
				archived: "it is already in the Bin",
			},
			{
				name: "other objects use the file",
				fixture: func(t *testing.T, read apicore.ObjectRead) *v2Fixture {
					fx := newDeleteFixtureRead(t, read, ownProvenance())
					fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
						bundle.RelationKeyId:    domain.String("embedder"),
						bundle.RelationKeyLinks: domain.StringList([]string{deleteObjId}),
					}})
					return fx
				},
				read: func(archived bool) apicore.ObjectRead {
					if archived {
						return archivedRead(model.SmartBlockType_FileObject)
					}
					return deleteRead(model.SmartBlockType_FileObject)
				},
				real:     "detach the file from those objects (a user in the Bin counts too), then resend this request; without permanent, DELETE moves the file to the Bin",
				dryRun:   "detach the file from those objects (a user in the Bin counts too), then resend this request; without permanent, the dry run previews moving the file to the Bin",
				archived: "detach the file from those objects (a user in the Bin counts too), then resend this request; the file is already in the Bin",
			},
		}
		for _, tc := range cases {
			for _, mode := range []struct {
				dryRun, archived bool
				want             string
			}{
				{want: tc.real},
				{dryRun: true, want: tc.dryRun},
				{archived: true, want: tc.archived},
				{dryRun: true, archived: true, want: tc.archived},
			} {
				// given
				fx := tc.fixture(t, tc.read(mode.archived))

				// when
				_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, mode.dryRun, true)

				// then
				got := v2Err(t, err)
				require.Len(t, got.Issues, 1, "%s, dryRun=%v, archived=%v", tc.name, mode.dryRun, mode.archived)
				assert.Equal(t, mode.want, got.Issues[0].Hint, "%s, dryRun=%v, archived=%v", tc.name, mode.dryRun, mode.archived)
				assert.NotContains(t, got.Message, "Without permanent", "the alternative is the hint's alone")
			}
		}
	})

	t.Run("the ownership probe's query reaches the same refusal", func(t *testing.T) {
		// given: copied in place of the request's query, the probe must test
		// the permanent delete it refused, not preview an archive
		fx := newDeleteFixture(t, apicore.Provenance{AccountMatch: true, IntegrationName: "Linear"})
		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)
		refused := requireNotCreatedByThisKey(t, err)
		probe := refused.Issues[0].SeeAlso[0]

		// when
		_, err = fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, probe.Query["dry_run"] == "true", probe.Query["permanent"] == "true")

		// then
		again := v2Err(t, err)
		assert.Equal(t, refused.Code, again.Code)
		assert.Equal(t, refused.Message, again.Message)
	})

	t.Run("a refusal's archive alternative says when the archive is refused too, or holds only for the file's owner", func(t *testing.T) {
		issue := v2model.Issue{Path: "object_id", Message: "this key may permanently delete only objects it created"}
		probe := v2model.Resend("dry_run", "true").With("permanent", "true")
		t.Run("restrictions forbid deleting it", func(t *testing.T) {
			// given
			read := deleteRead(model.SmartBlockType_Page)
			read.DeleteRefused = restriction.ErrRestricted
			fx := newDeleteFixtureRead(t, read, apicore.Provenance{AccountMatch: true, IntegrationName: "Linear"})
			want := issue.Hintf("probe without writing by resending with %s; a read of the object shows who created it; the object cannot be moved to the Bin either: its restrictions forbid deleting it", probe)

			// when
			_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

			// then
			assert.Equal(t, []v2model.Issue{want}, v2Err(t, err).Issues)
		})
		t.Run("a file in a one-to-one space", func(t *testing.T) {
			// given: only the member who added a file can delete it there,
			// which the archive RPC checks and a read cannot
			fx := newDeleteFixtureOf(t, model.SmartBlockType_FileObject, apicore.Provenance{AccountMatch: true, IntegrationName: "Linear"})
			fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String("spaceView_" + testSpaceId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
				bundle.RelationKeyTargetSpaceId:  domain.String(testSpaceId),
				bundle.RelationKeySpaceUxType:    domain.Int64(int64(model.SpaceUxType_OneToOne)),
			}})
			want := issue.Hintf("probe without writing by resending with %s; a read of the object shows who created it; without permanent, DELETE moves the object to the Bin if this account added it", probe)

			// when
			_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

			// then
			assert.Equal(t, []v2model.Issue{want}, v2Err(t, err).Issues)
		})
	})

	t.Run("the delete RPC's permanent refusals map to a 403", func(t *testing.T) {
		for _, desc := range []string{"restricted", "can't delete other's file"} {
			fx := newDeleteFixture(t, ownProvenance())
			fx.mwMock.On("ObjectListDelete", mock.Anything, mock.Anything).Return(&pb.RpcObjectListDeleteResponse{
				Error: &pb.RpcObjectListDeleteResponseError{
					Code:        pb.RpcObjectListDeleteResponseError_UNKNOWN_ERROR,
					Description: desc,
				},
			}).Once()

			_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

			var v2Err *v2model.Error
			require.ErrorAs(t, err, &v2Err, desc)
			assert.Equal(t, http.StatusForbidden, v2Err.Status, desc)
			assert.Contains(t, v2Err.Message, "do not retry", desc)
		}
	})

	t.Run("a delete-RPC failure surfaces as an internal error", func(t *testing.T) {
		fx := newDeleteFixture(t, ownProvenance())
		fx.mwMock.On("ObjectListDelete", mock.Anything, mock.Anything).Return(&pb.RpcObjectListDeleteResponse{
			Error: &pb.RpcObjectListDeleteResponseError{
				Code:        pb.RpcObjectListDeleteResponseError_UNKNOWN_ERROR,
				Description: "boom",
			},
		}).Once()

		_, err := fx.DeleteObject(callerCtx(), testSpaceId, deleteObjId, false, true)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete "+deleteObjId+" permanently")
	})
}

func TestRestoreObject(t *testing.T) {
	// No provenance expectation anywhere here: restore has the archive's
	// reach, so reaching the provenance read fails the test (the strict
	// mock panics on the unexpected call).
	newRestoreFixture := func(t *testing.T, sbType model.SmartBlockType) *v2Fixture {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(archivedRead(sbType), nil).Maybe()
		fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, deleteObjId).Return(true, nil).Maybe()
		return fx
	}

	t.Run("an archived object is restored, whoever created it, and answers once editable", func(t *testing.T) {
		// given: the read before the restore is archived; the read after it
		// has the detail cleared, which is what the restore waits for
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(archivedRead(model.SmartBlockType_Page), nil).Once()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(model.SmartBlockType_Page), nil).Once()
		fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, deleteObjId).Return(true, nil).Once()
		fx.expectArchive(false)
		want := &v2model.CreateResult{Id: deleteObjId, Type: "page"}

		// when
		got, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("a detail that never clears: restored, with a warning", func(t *testing.T) {
		// the reconcile goroutine has not caught up within the wait
		old := restoreSettleTimeout
		restoreSettleTimeout = 50 * time.Millisecond
		t.Cleanup(func() { restoreSettleTimeout = old })
		fx := newRestoreFixture(t, model.SmartBlockType_Page)
		fx.expectArchive(false)

		got, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, false)

		require.NoError(t, err)
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, "edits may be refused")
	})

	t.Run("the Bin state is the Bin's membership, not the lagging detail or row — found live", func(t *testing.T) {
		// a restore right after an archive: the RPC has returned and the Bin
		// holds the object, but its detail and row are written later by the
		// Bin's reconcile goroutine. Read off either, the restore said "not
		// archived" and did nothing.
		fx := newV2Fixture(t)
		// twice: the restore reads it, then waits on the (already clear) detail
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(model.SmartBlockType_Page), nil).Twice()
		fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, deleteObjId).Return(true, nil).Once()
		fx.markRowArchived(t, false)
		fx.expectArchive(false)

		got, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, false)

		require.NoError(t, err)
		assert.Empty(t, got.Warnings)
	})

	t.Run("out of the Bin while the detail still says archived: not archived", func(t *testing.T) {
		// the other direction of the same lag: a restore just returned
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(archivedRead(model.SmartBlockType_Page), nil).Once()
		fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, deleteObjId).Return(false, nil).Once()

		got, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, false)

		require.NoError(t, err)
		assert.Equal(t, []v2model.Issue{{Path: "object_id", Message: "not archived"}}, got.Warnings)
	})

	t.Run("a Bin read failure is an error, never a guess", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(archivedRead(model.SmartBlockType_Page), nil).Once()
		fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, deleteObjId).Return(false, errors.New("bin unavailable")).Once()

		_, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, false)

		require.Error(t, err)
		var v2Err *v2model.Error
		assert.False(t, errors.As(err, &v2Err))
	})

	t.Run("an object that is not archived is an idempotent 200 with a warning", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(model.SmartBlockType_Page), nil).Once()
		fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, deleteObjId).Return(false, nil).Once()
		want := &v2model.CreateResult{Id: deleteObjId, Type: "page",
			Warnings: []v2model.Issue{{Path: "object_id", Message: "not archived"}}}

		got, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, false)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("dry run: the verdict, no write", func(t *testing.T) {
		fx := newRestoreFixture(t, model.SmartBlockType_Page)

		got, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, true)

		require.NoError(t, err)
		assert.Equal(t, &v2model.CreateResult{Id: deleteObjId, Type: "page", DryRun: true}, got)
	})

	t.Run("types, properties and tag options are restored in the app", func(t *testing.T) {
		for _, sbType := range []model.SmartBlockType{
			model.SmartBlockType_STType,
			model.SmartBlockType_STRelation,
			model.SmartBlockType_STRelationOption,
		} {
			fx := newRestoreFixture(t, sbType)

			_, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, false)

			var v2Err *v2model.Error
			require.ErrorAs(t, err, &v2Err, sbType.String())
			assert.Equal(t, http.StatusBadRequest, v2Err.Status, sbType.String())
			require.NotEmpty(t, v2Err.Issues, sbType.String())
			assert.Contains(t, v2Err.Issues[0].Hint, "Anytype app", sbType.String())
		}
	})

	t.Run("system objects refuse", func(t *testing.T) {
		fx := newRestoreFixture(t, model.SmartBlockType_Workspace)

		_, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, false)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusForbidden, v2Err.Status)
	})

	t.Run("write not granted refuses before the read", func(t *testing.T) {
		fx := newV2Fixture(t)
		ctx := util.CtxWithApiGrant(callerCtx(), &util.ApiGrant{Spaces: []string{testSpaceId}, Perms: util.GrantPermsRead})

		_, err := fx.RestoreObject(ctx, testSpaceId, deleteObjId, false)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, v2model.CodeWriteNotGranted, v2Err.Code)
	})

	t.Run("a permanently deleted object says so", func(t *testing.T) {
		// the tree is gone: the read adapter answers ErrTreeStorageAlreadyDeleted
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).
			Return(apicore.ObjectRead{}, fmt.Errorf("read live object state: %w", spacestorage.ErrTreeStorageAlreadyDeleted)).Once()

		_, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, false)

		assert.Equal(t, v2model.ObjectDeleted(testSpaceId, deleteObjId), err)
	})

	t.Run("a tombstoned row says so too", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(archivedRead(model.SmartBlockType_Page), nil).Once()
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:        domain.String(deleteObjId),
			bundle.RelationKeyIsDeleted: domain.Bool(true),
		}})

		_, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, false)

		assert.Equal(t, v2model.ObjectDeleted(testSpaceId, deleteObjId), err)
	})
}

func TestPatchArchivedObject(t *testing.T) {
	t.Run("an edit of an object in the Bin is a 409 naming the restore", func(t *testing.T) {
		// given: archived objects carry every edit restriction, so before
		// this the edit met a 403 calling itself permanent. No mutator
		// expectation: the refusal comes before the lock.
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(archivedRead(model.SmartBlockType_Page), nil).Once()
		// the row lags: it still says live, and must not be what decides
		fx.markRowArchived(t, false)

		// when
		_, err := fx.PatchObject(callerCtx(), testSpaceId, deleteObjId,
			patchBody(`{"op":"set_properties","set":{"name":"Renamed"}}`), "", false, false)

		// then
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, http.StatusConflict, v2Err.Status)
		assert.Equal(t, v2model.CodeObjectArchived, v2Err.Code)
		require.Len(t, v2Err.Issues, 1)
		requireOps(t, v2Err.Issues[0], v2model.RefRestoreObject(testSpaceId, deleteObjId))

		// and a preview's: following it from a dry run restores nothing
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(archivedRead(model.SmartBlockType_Page), nil).Once()
		_, err = fx.PatchObject(callerCtx(), testSpaceId, deleteObjId,
			patchBody(`{"op":"set_properties","set":{"name":"Renamed"}}`), "", true, false)
		require.ErrorAs(t, err, &v2Err)
		require.Len(t, v2Err.Issues, 1)
		requireOps(t, v2Err.Issues[0], v2model.RefRestoreObject(testSpaceId, deleteObjId).With("dry_run", "true"))
	})

	t.Run("an archived type's object names the app, not a restore that refuses it", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(archivedRead(model.SmartBlockType_STType), nil).Once()

		_, err := fx.PatchObject(callerCtx(), testSpaceId, deleteObjId,
			patchBody(`{"op":"set_properties","set":{"name":"Renamed"}}`), "", false, false)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, v2model.CodeObjectArchived, v2Err.Code)
		require.Len(t, v2Err.Issues, 1)
		assert.Empty(t, v2Err.Issues[0].SeeAlso)
		assert.Contains(t, v2Err.Issues[0].Hint, "Anytype app")
	})
}

func TestDeletedTemplate(t *testing.T) {
	// deleting a template uninstalls it instead of removing its tree
	// (core/block deleteDerivedObject): the head mark stays clear and the row
	// says isUninstalled + isDeleted, so the row is what tells
	newFixture := func(t *testing.T) *v2Fixture {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, deleteObjId).Return(deleteRead(model.SmartBlockType_Template), nil).Maybe()
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String(deleteObjId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyIsUninstalled:  domain.Bool(true),
			bundle.RelationKeyIsDeleted:      domain.Bool(true),
		}})
		return fx
	}
	want := v2model.ObjectDeleted(testSpaceId, deleteObjId)

	t.Run("a read is a 410, as the permanent delete's receipt promised", func(t *testing.T) {
		fx := newFixture(t)

		_, _, err := fx.GetObject(callerCtx(), testSpaceId, deleteObjId, ObjectQuery{})

		assert.Equal(t, want, err)
	})

	t.Run("an edit is a 410", func(t *testing.T) {
		fx := newFixture(t)

		_, err := fx.PatchObject(callerCtx(), testSpaceId, deleteObjId, patchBody(`{"op":"set_properties","set":{"name":"x"}}`), "", false, true)

		assert.Equal(t, want, err)
	})

	t.Run("a restore is a 410", func(t *testing.T) {
		fx := newFixture(t)

		_, err := fx.RestoreObject(callerCtx(), testSpaceId, deleteObjId, false)

		assert.Equal(t, want, err)
	})
}
