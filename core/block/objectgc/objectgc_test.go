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

func (m *mockArchiver) SetListIsArchived(_ session.Context, _ context.Context, objectIds []string, isArchived bool) error {
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
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_ParentArchived_WithActiveBacklink(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("other"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "other"}))

	// when
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_ParentArchived_OtherBacklinksAllArchived(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, archivedObject("other"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "other"}))

	// when
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_ParentArchived_OtherBacklinksAllDeleted(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, deletedObject("other"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "other"}))

	// when
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentStillActive(t *testing.T) {
	// given: file's parent is active, objectId is just a backlinker
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "backlinker", true)

	// then: file must not be touched — parent is still active (safety gate)
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentArchived_NoOtherBacklinks(t *testing.T) {
	// given: file's parent is already archived, backlinker is the last reference
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "backlinker", true)

	// then: file should be archived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentArchived_OtherActiveBacklink(t *testing.T) {
	// given: file's parent is archived, but another object still actively links to the file
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, regularObject("activeRef"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker", "activeRef"}))

	// when: backlinker is archived
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "backlinker", true)

	// then: file kept because activeRef is still active
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentArchived_OtherBacklinksAllArchived(t *testing.T) {
	// given: file's parent and all other backlinks are archived
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, archivedObject("archivedRef"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker", "archivedRef"}))

	// when: backlinker is archived
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "backlinker", true)

	// then: file should be archived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentMissingFromStore(t *testing.T) {
	// given: file's parent is not in the store at all (fully deleted), backlinker is the last reference
	fx := newFixture(t)
	fx.addObject(t, regularObject("backlinker"))
	// "parent" is intentionally not added to the store
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "backlinker", true)

	// then: file archived — missing parent treated as deleted (safety gate passes)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
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
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "page", true)

	// then: icon must NOT be archived — only objects with explicit createdInContext are GC'd
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_ObjectIsFile_EarlyReturn(t *testing.T) {
	// given: the archived object is itself a file
	fx := newFixture(t)
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("fileContext"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_image)),
		},
	})

	// when
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "fileContext", true)

	// then: returns immediately, no GC triggered
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

// -- links restored (undo) tests --

func TestCheckObjectsOnLinksRestored_RestoresArchivedFile(t *testing.T) {
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
	err := fx.CheckObjectsOnLinksRestored(nil, testSpaceId, "page", []string{"file1"})

	// then: file is unarchived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.unarchivedIds)
}

func TestCheckObjectsOnLinksRestored_IgnoresFileFromDifferentContext(t *testing.T) {
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
	err := fx.CheckObjectsOnLinksRestored(nil, testSpaceId, "page", []string{"file1"})

	// then: file is not touched — wrong context
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.unarchivedIds)
}

func TestCheckObjectsOnLinksRestored_IgnoresAlreadyActiveFile(t *testing.T) {
	// given: file is not archived (was never GC'd or was already restored)
	fx := newFixture(t)
	fx.addObject(t, fileObject("file1", "page", []string{"page"}))

	// when
	err := fx.CheckObjectsOnLinksRestored(nil, testSpaceId, "page", []string{"file1"})

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
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "parent", false)

	// then: file restored
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.unarchivedIds)
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
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "parent", false)

	// then: file kept archived because it has other backlinks
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.unarchivedIds)
}

// -- non-file object GC tests --

func TestCheckObjectsOnObjectArchived_NonFileObject_ParentArchived_NoOtherBacklinks(t *testing.T) {
	// given: a basic (non-file) object was created inside parent
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObject("child", "parent", []string{"parent"}))

	// when: parent is archived
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "parent", true)

	// then: child is archived too
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_NonFileObject_ParentArchived_WithActiveBacklink(t *testing.T) {
	// given: child still referenced by another active object
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("other"))
	fx.addObject(t, basicObject("child", "parent", []string{"parent", "other"}))

	// when
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "parent", true)

	// then: child is kept because "other" is still active
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
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
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "parent", false)

	// then: child is restored
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, fx.archiver.unarchivedIds)
}

func TestCheckObjectsOnLinksRemoval_NonFileObject_SkipBinForcedFalse(t *testing.T) {
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
	err := fx.CheckObjectsOnLinksRemoval(nil, testSpaceId, "parent", []string{"child"}, true, nil)

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
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "parent", true)

	// then: system object is NOT touched
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

func TestCheckObjectsOnLinksRestored_NonFileObject_Restored(t *testing.T) {
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
	err := fx.CheckObjectsOnLinksRestored(nil, testSpaceId, "page", []string{"child"})

	// then: child is unarchived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, fx.archiver.unarchivedIds)
}

// -- event emission tests for CheckObjectsOnLinksRemoval --

func TestCheckObjectsOnLinksRemoval_WithSession_EmitsEvent(t *testing.T) {
	// given: a file that will be archived (no active backlinks, sctx provided)
	fx := newFixture(t)
	fx.addObject(t, fileObject("file1", "page", []string{"page"}))
	sctx := session.NewContext(session.WithSession("test-token"))

	// when
	err := fx.CheckObjectsOnLinksRemoval(sctx, testSpaceId, "page", []string{"file1"}, false, nil)

	// then: file archived AND event accumulated in session context
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
	msgs := sctx.GetMessages()
	require.Len(t, msgs, 1)
	msg := msgs[0].Value.(*pb.EventMessageValueOfObjectAutoArchive)
	assert.ElementsMatch(t, []string{"file1"}, msg.ObjectAutoArchive.ObjectIds)
}

func TestCheckObjectsOnLinksRemoval_NilSession_NoEvent(t *testing.T) {
	// given: same setup but no session
	fx := newFixture(t)
	fx.addObject(t, fileObject("file1", "page", []string{"page"}))

	// when
	err := fx.CheckObjectsOnLinksRemoval(nil, testSpaceId, "page", []string{"file1"}, false, nil)

	// then: file archived, nil sctx — no panic, nothing to assert on session
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckObjectsOnLinksRemoval_WithSession_NothingArchived_NoEvent(t *testing.T) {
	// given: file has an active backlink — won't be archived
	fx := newFixture(t)
	fx.addObject(t, regularObject("other"))
	fx.addObject(t, fileObject("file1", "page", []string{"page", "other"}))
	sctx := session.NewContext(session.WithSession("test-token"))

	// when
	err := fx.CheckObjectsOnLinksRemoval(sctx, testSpaceId, "page", []string{"file1"}, false, nil)

	// then: nothing archived, no event in session
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
	assert.Empty(t, sctx.GetMessages())
}

// -- event emission tests for CheckObjectsOnObjectArchived --

func TestCheckObjectsOnObjectArchived_WithSession_EmitsEvent(t *testing.T) {
	// given: parent archived, file has no other backlinks
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent"}))
	sctx := session.NewContext(session.WithSession("test-token"))

	// when
	err := fx.CheckObjectsOnObjectArchived(sctx, testSpaceId, "parent", true)

	// then: file archived AND event accumulated in session context
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
	msgs := sctx.GetMessages()
	require.Len(t, msgs, 1)
	msg := msgs[0].Value.(*pb.EventMessageValueOfObjectAutoArchive)
	assert.ElementsMatch(t, []string{"file1"}, msg.ObjectAutoArchive.ObjectIds)
}

func TestCheckObjectsOnObjectArchived_NilSession_NoEvent(t *testing.T) {
	// given: parent archived, file has no other backlinks, but no session
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent"}))

	// when
	err := fx.CheckObjectsOnObjectArchived(nil, testSpaceId, "parent", true)

	// then: file archived, nil sctx — no panic
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckObjectsOnObjectArchived_Unarchive_NoEvent(t *testing.T) {
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
	sctx := session.NewContext(session.WithSession("test-token"))

	// when: unarchive direction
	err := fx.CheckObjectsOnObjectArchived(sctx, testSpaceId, "parent", false)

	// then: file restored, NO auto-archive event (only archive direction emits)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.unarchivedIds)
	assert.Empty(t, sctx.GetMessages())
}

func TestAccumulateAutoArchiveEvent_MergesAcrossMultipleCalls(t *testing.T) {
	// given: a fresh session context
	sctx := session.NewContext(session.WithSession("test-token"))

	// when: two separate archival operations accumulate into the same session
	accumulateAutoArchiveEvent(sctx, []string{"file1", "file2"}, "block1")
	accumulateAutoArchiveEvent(sctx, []string{"file2", "file3"}, "block1")

	// then: exactly one event with deduplicated union of all IDs
	msgs := sctx.GetMessages()
	require.Len(t, msgs, 1)
	msg := msgs[0].Value.(*pb.EventMessageValueOfObjectAutoArchive)
	assert.ElementsMatch(t, []string{"file1", "file2", "file3"}, msg.ObjectAutoArchive.ObjectIds)
}
