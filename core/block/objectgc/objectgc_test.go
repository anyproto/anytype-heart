package objectgc

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testSpaceId = "test-space"

// -- fixture --

type fixture struct {
	*objectGC
	store    *objectstore.StoreFixture
	archiver *mockArchiver
}

func newFixture(t *testing.T) *fixture {
	store := objectstore.NewStoreFixture(t)
	archiver := &mockArchiver{}
	gc := &objectGC{
		objectStore:         store,
		objectArchiver:      archiver,
		backlinksWatcher:    &noopFlusher{},
		componentCtx:        context.Background(),
		participantProvider: &mockParticipantProvider{},
	}
	return &fixture{
		objectGC:   gc,
		store:    store,
		archiver: archiver,
	}
}

func (f *fixture) addObject(t *testing.T, obj objectstore.TestObject) {
	f.store.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})
}

func fileObject(id, createdInContext string, backlinks []string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:               domain.String(id),
		bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_image)),
		bundle.RelationKeyCreatedInContext: domain.String(createdInContext),
		bundle.RelationKeyBacklinks:        domain.StringList(backlinks),
	}
}

func regularObject(id string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
	}
}

func archivedObject(id string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyIsArchived:     domain.Bool(true),
	}
}

func deletedObject(id string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyIsDeleted:      domain.Bool(true),
	}
}

func basicObject(id, createdInContext string, backlinks []string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:               domain.String(id),
		bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyCreatedInContext: domain.String(createdInContext),
		bundle.RelationKeyBacklinks:        domain.StringList(backlinks),
	}
}

func systemObject(id, createdInContext string, backlinks []string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:               domain.String(id),
		bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_participant)),
		bundle.RelationKeyCreatedInContext: domain.String(createdInContext),
		bundle.RelationKeyBacklinks:        domain.StringList(backlinks),
	}
}

// -- mocks --

type mockParticipantProvider struct{ id string }

func (m *mockParticipantProvider) MyParticipantId(_ string) string { return m.id }

type mockArchiver struct {
	archivedIds   []string
	unarchivedIds []string
}

func (m *mockArchiver) SetListIsArchivedNoGC(_ context.Context, objectIds []string, isArchived bool) error {
	if isArchived {
		m.archivedIds = append(m.archivedIds, objectIds...)
	} else {
		m.unarchivedIds = append(m.unarchivedIds, objectIds...)
	}
	return nil
}

type noopFlusher struct{}

func (n *noopFlusher) Name() string          { return "noopFlusher" }
func (n *noopFlusher) Init(_ *app.App) error { return nil }
func (n *noopFlusher) FlushUpdates()         {}

// -- archive direction tests --

func TestCheckObjectsOnObjectArchived_ParentArchived_NoOtherBacklinks(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent"}))

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, ids)
}

func TestCheckObjectsOnObjectArchived_ParentArchived_WithActiveBacklink(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("other"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "other"}))

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestCheckObjectsOnObjectArchived_ParentArchived_OtherBacklinksAllArchived(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, archivedObject("other"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "other"}))

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, ids)
}

func TestCheckObjectsOnObjectArchived_ParentArchived_OtherBacklinksAllDeleted(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, deletedObject("other"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "other"}))

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, ids)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentStillActive(t *testing.T) {
	// given: file's parent is active, objectId is just a backlinker
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file must not be touched — parent is still active (safety gate)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentArchived_NoOtherBacklinks(t *testing.T) {
	// given: file's parent is already archived, backlinker is the last reference
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file should be archived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, ids)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentArchived_OtherActiveBacklink(t *testing.T) {
	// given: file's parent is archived, but another object still actively links to the file
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, regularObject("activeRef"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker", "activeRef"}))

	// when: backlinker is archived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file kept because activeRef is still active
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentArchived_OtherBacklinksAllArchived(t *testing.T) {
	// given: file's parent and all other backlinks are archived
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, archivedObject("archivedRef"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker", "archivedRef"}))

	// when: backlinker is archived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file should be archived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, ids)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentMissingFromStore(t *testing.T) {
	// given: file's parent is not in the store at all (sync gap), backlinker is the last reference
	fx := newFixture(t)
	fx.addObject(t, regularObject("backlinker"))
	// "parent" is intentionally not added to the store — simulates sync lag
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file NOT archived — parent is not confirmed inactive in the store (sync-consistency check)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentArchivedInStore(t *testing.T) {
	// given: file's parent is confirmed archived in the store
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file archived — parent confirmed inactive
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, ids)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentDeletedInStore(t *testing.T) {
	// given: file's parent is confirmed deleted in the store
	fx := newFixture(t)
	fx.addObject(t, deletedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file archived — parent confirmed inactive (deleted)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, ids)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_NoCreatedInContext_NotArchived(t *testing.T) {
	// given: file has no createdInContext (e.g. imported without context tracking),
	// and its only backlink is the object being archived (e.g. usecase icon image)
	fx := newFixture(t)
	fx.addObject(t, regularObject("page"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("icon"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_image)),
			bundle.RelationKeyBacklinks:      domain.StringList([]string{"page"}),
			// no createdInContext — mimics usecase-imported icons
		},
	})

	// when: page is archived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "page", true)

	// then: icon must NOT be archived — only objects with explicit createdInContext are GC'd
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestCheckObjectsOnObjectArchived_ImageObject_NoChildrenFound(t *testing.T) {
	// given: the archived object is itself an image with no children created in its context
	fx := newFixture(t)
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("fileContext"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_image)),
		},
	})

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "fileContext", true)

	// then: no children have createdInContext == "fileContext", so nothing is archived
	require.NoError(t, err)
	assert.Empty(t, ids)
}

// -- links restored (undo) tests --

func TestRestoreOrphansOnLinksAdded_RestoresArchivedFile(t *testing.T) {
	// given: file was GC'd (archived) when its link was deleted; undo re-adds the link
	fx := newFixture(t)
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("file1"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_image)),
			bundle.RelationKeyCreatedInContext: domain.String("page"),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when: undo re-adds the file link
	_, err := fx.RestoreOrphansOnLinksAdded(testSpaceId, "page", []string{"file1"})

	// then: file is unarchived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.unarchivedIds)
}

func TestRestoreOrphansOnLinksAdded_IgnoresFileFromDifferentContext(t *testing.T) {
	// given: file belongs to a different context
	fx := newFixture(t)
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("file1"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_image)),
			bundle.RelationKeyCreatedInContext: domain.String("other-page"),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when: link restored to a different page
	_, err := fx.RestoreOrphansOnLinksAdded(testSpaceId, "page", []string{"file1"})

	// then: file is not touched — wrong context
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.unarchivedIds)
}

func TestRestoreOrphansOnLinksAdded_IgnoresAlreadyActiveFile(t *testing.T) {
	// given: file is not archived (was never GC'd or was already restored)
	fx := newFixture(t)
	fx.addObject(t, fileObject("file1", "page", []string{"page"}))

	// when
	_, err := fx.RestoreOrphansOnLinksAdded(testSpaceId, "page", []string{"file1"})

	// then: nothing to do
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.unarchivedIds)
}

// -- unarchive direction tests --

func TestCheckObjectsOnObjectArchived_Unarchive_NoOtherBacklinks(t *testing.T) {
	// given: file was archived alongside its parent, now parent is being restored
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("file1"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_image)),
			bundle.RelationKeyCreatedInContext: domain.String("parent"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"parent"}),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when: parent unarchived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", false)

	// then: file restored
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, ids)
}

func TestCheckObjectsOnObjectArchived_Unarchive_HasOtherBacklinks(t *testing.T) {
	// given: file has another backlink besides the parent
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("file1"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_image)),
			bundle.RelationKeyCreatedInContext: domain.String("parent"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"parent", "otherRef"}),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when: parent unarchived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", false)

	// then: file kept archived because it has other backlinks
	require.NoError(t, err)
	assert.Empty(t, ids)
}

// -- non-file object GC tests --

func TestCheckObjectsOnObjectArchived_NonFileObject_ParentArchived_NoOtherBacklinks(t *testing.T) {
	// given: a basic (non-file) object was created inside parent
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObject("child", "parent", []string{"parent"}))

	// when: parent is archived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: child is archived too
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, ids)
}

func TestCheckObjectsOnObjectArchived_NonFileObject_ParentArchived_WithActiveBacklink(t *testing.T) {
	// given: child still referenced by another active object
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("other"))
	fx.addObject(t, basicObject("child", "parent", []string{"parent", "other"}))

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: child is kept because "other" is still active
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestCheckObjectsOnObjectArchived_NonFileObject_Unarchive(t *testing.T) {
	// given: child was archived alongside parent
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("child"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext: domain.String("parent"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"parent"}),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when: parent is unarchived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", false)

	// then: child is restored
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, ids)
}

func TestArchiveOrphansOnLinksRemoval_NonFileObject_SkipBinForcedFalse(t *testing.T) {
	// given: a basic object whose link is removed; caller requests skipBin=true
	fx := newFixture(t)
	fx.participantProvider = &mockParticipantProvider{id: "user1"}
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("child"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext: domain.String("parent"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"parent"}),
			bundle.RelationKeyCreator:          domain.String("user1"),
		},
	})

	// when: caller requests skipBin=true (as chat service does for files)
	_, err := fx.ArchiveOrphansOnLinksRemoval(testSpaceId, "parent", []string{"child"}, true, nil)

	// then: child is archived, NOT permanently deleted — skipBin overridden to false for non-files
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_SystemLayoutObject_NotGCd(t *testing.T) {
	// given: an object with a system layout (participant) has createdInContext set
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		systemObject("sysobj", "parent", []string{"parent"}),
	})

	// when: parent is archived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: system object is NOT touched
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestRestoreOrphansOnLinksAdded_NonFileObject_Restored(t *testing.T) {
	// given: basic object was GC'd (archived), undo re-adds the link
	fx := newFixture(t)
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("child"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext: domain.String("page"),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when: link re-added via undo
	_, err := fx.RestoreOrphansOnLinksAdded(testSpaceId, "page", []string{"child"})

	// then: child is unarchived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, fx.archiver.unarchivedIds)
}

// -- returned-IDs tests for ArchiveOrphansOnLinksRemoval --

func TestArchiveOrphansOnLinksRemoval_ReturnsArchivedIds(t *testing.T) {
	// given: a file with no active backlinks besides the context
	fx := newFixture(t)
	fx.addObject(t, fileObject("file1", "page", []string{"page"}))

	// when
	archivedIds, err := fx.ArchiveOrphansOnLinksRemoval(testSpaceId, "page", []string{"file1"}, false, nil)

	// then: file archived and returned
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
	assert.ElementsMatch(t, []string{"file1"}, archivedIds)
}

func TestArchiveOrphansOnLinksRemoval_ActiveBacklink_ReturnsEmpty(t *testing.T) {
	// given: file has an active backlink — won't be archived
	fx := newFixture(t)
	fx.addObject(t, regularObject("other"))
	fx.addObject(t, fileObject("file1", "page", []string{"page", "other"}))

	// when
	archivedIds, err := fx.ArchiveOrphansOnLinksRemoval(testSpaceId, "page", []string{"file1"}, false, nil)

	// then: nothing archived, empty slice returned
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
	assert.Empty(t, archivedIds)
}

// -- returned-IDs tests for CheckObjectsOnObjectArchived --

func TestCheckObjectsOnObjectArchived_ReturnsArchivedIds(t *testing.T) {
	// given: parent archived, file has no other backlinks
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent"}))

	// when
	archivedIds, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: file archived and returned
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, archivedIds)
}

func TestCheckObjectsOnObjectArchived_Unarchive_ReturnsRestoredIds(t *testing.T) {
	// given: parent unarchived, file was archived
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("file1"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_image)),
			bundle.RelationKeyCreatedInContext: domain.String("parent"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"parent"}),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when: unarchive direction
	restoredIds, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", false)

	// then: file restored and returned
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, restoredIds)
}

func TestRestoreOrphansOnLinksAdded_ReturnsRestoredIds(t *testing.T) {
	// given: file was GC'd and is now being restored via undo
	fx := newFixture(t)
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("file1"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_image)),
			bundle.RelationKeyCreatedInContext: domain.String("page"),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when
	restoredIds, err := fx.RestoreOrphansOnLinksAdded(testSpaceId, "page", []string{"file1"})

	// then: file unarchived and returned
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.unarchivedIds)
	assert.ElementsMatch(t, []string{"file1"}, restoredIds)
}

func TestFilterExplicitIds_RemovesFromAutoArchive(t *testing.T) {
	sctx := session.NewContext(session.WithSession("test-token"))
	sctx.SetMessages("block1", []*pb.EventMessage{{
		Value: &pb.EventMessageValueOfObjectAutoArchive{
			ObjectAutoArchive: &pb.EventObjectAutoArchive{ObjectIds: []string{"a", "b", "c"}},
		},
	}})

	FilterExplicitIds(sctx, []string{"b", "c"})

	msgs := sctx.GetMessages()
	require.Len(t, msgs, 1)
	msg := msgs[0].Value.(*pb.EventMessageValueOfObjectAutoArchive)
	assert.ElementsMatch(t, []string{"a"}, msg.ObjectAutoArchive.ObjectIds)
}

func TestFilterExplicitIds_RemovesFromAutoRestore(t *testing.T) {
	sctx := session.NewContext(session.WithSession("test-token"))
	sctx.SetMessages("block1", []*pb.EventMessage{{
		Value: &pb.EventMessageValueOfObjectAutoRestore{
			ObjectAutoRestore: &pb.EventObjectAutoRestore{ObjectIds: []string{"a", "b", "c"}},
		},
	}})

	FilterExplicitIds(sctx, []string{"a", "b", "c"})

	// all IDs removed — message dropped entirely
	assert.Empty(t, sctx.GetMessages())
}

func TestFilterExplicitIds_PreservesUnrelatedMessages(t *testing.T) {
	sctx := session.NewContext(session.WithSession("test-token"))
	sctx.SetMessages("block1", []*pb.EventMessage{
		{Value: &pb.EventMessageValueOfObjectAutoArchive{
			ObjectAutoArchive: &pb.EventObjectAutoArchive{ObjectIds: []string{"a", "b"}},
		}},
		{Value: &pb.EventMessageValueOfObjectAutoRestore{
			ObjectAutoRestore: &pb.EventObjectAutoRestore{ObjectIds: []string{"c", "d"}},
		}},
	})

	FilterExplicitIds(sctx, []string{"b", "d"})

	msgs := sctx.GetMessages()
	require.Len(t, msgs, 2)
	archiveMsg := msgs[0].Value.(*pb.EventMessageValueOfObjectAutoArchive)
	assert.ElementsMatch(t, []string{"a"}, archiveMsg.ObjectAutoArchive.ObjectIds)
	restoreMsg := msgs[1].Value.(*pb.EventMessageValueOfObjectAutoRestore)
	assert.ElementsMatch(t, []string{"c"}, restoreMsg.ObjectAutoRestore.ObjectIds)
}

// -- BFS collect tests --

func TestCheckObjectsOnObjectArchived_TwoLevelTree_BothLevelsArchived(t *testing.T) {
	// given
	// parent (being archived)
	//   └→ child  (createdInContext=parent, backlinks=[parent])
	//        └→ grandchild (createdInContext=child, backlinks=[child])
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObject("child", "parent", []string{"parent"}))
	fx.addObject(t, basicObject("grandchild", "child", []string{"child"}))

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child", "grandchild"}, ids)
}

func TestCheckObjectsOnObjectArchived_CycleAB_TerminatesAndArchivesB(t *testing.T) {
	// given: A.createdInContext=B, B.createdInContext=A — cycle
	// Archiving A: BFS visits A (seed), finds B (child of A), visited={A,B}.
	// When processing B, finds A as its child — but A is already in visited → skip.
	// Result: B is archived, no infinite loop.
	fx := newFixture(t)
	fx.addObject(t, basicObject("A", "B", []string{"B"}))
	fx.addObject(t, basicObject("B", "A", []string{"A"}))

	// when — must complete without hanging
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "A", true)

	// then
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"B"}, ids)
}

func TestCheckObjectsOnObjectArchived_GrandchildWithExternalBacklink_SubtreeExcluded(t *testing.T) {
	// given: grandchild has an external active backlink; its subtree must be excluded too.
	// parent
	//   └→ child (createdInContext=parent)
	//        └→ grandchild (backlinks=[child, external])  ← external active backlink
	//             └→ greatgrandchild (createdInContext=grandchild)
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("external"))
	fx.addObject(t, basicObject("child", "parent", []string{"parent"}))
	fx.addObject(t, basicObject("grandchild", "child", []string{"child", "external"}))
	fx.addObject(t, basicObject("greatgrandchild", "grandchild", []string{"grandchild"}))

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	// child is archived; grandchild and greatgrandchild are kept (external backlink on grandchild)
	assert.ElementsMatch(t, []string{"child"}, ids)
}

func TestCheckObjectsOnObjectArchived_SiblingCrossReference_BothArchived(t *testing.T) {
	// given: two siblings that reference each other as backlinks.
	// Without the pending set, each sibling would see the other as an "active" backlink
	// and both would be incorrectly excluded.
	// parent
	//   └→ child1 (backlinks=[parent, child2])
	//   └→ child2 (backlinks=[parent, child1])
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObject("child1", "parent", []string{"parent", "child2"}))
	fx.addObject(t, basicObject("child2", "parent", []string{"parent", "child1"}))

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: both siblings are archived because their only backlinks are each other (in pending set)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child1", "child2"}, ids)
}

func TestCheckObjectsOnObjectArchived_Unarchive_MultiLevelRestored(t *testing.T) {
	// given
	// parent (unarchived)
	//   └→ child (archived, createdInContext=parent, backlinks=[parent])
	//        └→ grandchild (archived, createdInContext=child, backlinks=[child])
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("child"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext: domain.String("parent"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"parent"}),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
		{
			bundle.RelationKeyId:               domain.String("grandchild"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext: domain.String("child"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"child"}),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", false)

	// then: both child and grandchild are restored
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child", "grandchild"}, ids)
}

func TestCheckObjectsOnObjectArchived_SiblingExcludedCascade_DependentSiblingAlsoExcluded(t *testing.T) {
	// Regression test for cascade exclusion bug:
	// When sibling X is evicted (has external active backlink), sibling Y whose only
	// backlink is X must also be excluded — not archived.
	//
	// parent (being archived)
	//   └→ X (backlinks=[parent, external])  ← external active backlink → excluded
	//   └→ Y (backlinks=[X])                 ← X is not being archived → Y must also be excluded
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("external"))
	fx.addObject(t, basicObject("X", "parent", []string{"parent", "external"}))
	fx.addObject(t, basicObject("Y", "parent", []string{"X"}))

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: neither X nor Y is archived
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestCheckObjectsOnObjectArchived_Unarchive_ArchivedSiblingCrossReference_BothRestored(t *testing.T) {
	// Regression test for restore-path sibling bug:
	// Two archived siblings that mutually reference each other should both be restored
	// when their parent is unarchived, not block each other.
	//
	// parent (unarchived)
	//   └→ child1 (archived, backlinks=[parent, child2])
	//   └→ child2 (archived, backlinks=[parent, child1])
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("child1"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext: domain.String("parent"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"parent", "child2"}),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
		{
			bundle.RelationKeyId:               domain.String("child2"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext: domain.String("parent"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"parent", "child1"}),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", false)

	// then: both siblings are restored
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child1", "child2"}, ids)
}
