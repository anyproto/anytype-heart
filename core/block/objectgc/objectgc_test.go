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
		objectGC: gc,
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

// systemObjectWithRef builds a non-GC-eligible (system layout) object that nevertheless passes the
// CreatedInContextRef gate, so that only the layout can exclude it.
func systemObjectWithRef(id, createdInContext, ref string, backlinks []string) objectstore.TestObject {
	obj := systemObject(id, createdInContext, backlinks)
	obj[bundle.RelationKeyCreatedInContextRef] = domain.String(ref)
	return obj
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

	// then: file has empty CreatedInContextRef (collection-created) — excluded by the ref gate
	require.NoError(t, err)
	assert.Empty(t, ids)
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

	// then: file has empty CreatedInContextRef (collection-created) — excluded by the ref gate
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestCheckObjectsOnObjectArchived_ParentArchived_OtherBacklinksAllDeleted(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, deletedObject("other"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "other"}))

	// when
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: file has empty CreatedInContextRef (collection-created) — excluded by the ref gate
	require.NoError(t, err)
	assert.Empty(t, ids)
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

	// then: file has empty CreatedInContextRef (collection-created) — excluded by the ref gate
	require.NoError(t, err)
	assert.Empty(t, ids)
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

	// then: file has empty CreatedInContextRef (collection-created) — excluded by the ref gate
	require.NoError(t, err)
	assert.Empty(t, ids)
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

	// then: file has empty CreatedInContextRef (collection-created) — excluded by the ref gate
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestCheckObjectsOnObjectArchived_BacklinkerArchived_ParentDeletedInStore(t *testing.T) {
	// given: file's parent is confirmed deleted in the store
	fx := newFixture(t)
	fx.addObject(t, deletedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	ids, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file has empty CreatedInContextRef (collection-created) — excluded by the ref gate
	require.NoError(t, err)
	assert.Empty(t, ids)
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
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", false)

	// then: file restored
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, res.Files)
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
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", []string{"parent"}))

	// when: parent is archived
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: non-file object orphan → candidate (not auto-archived)
	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.ElementsMatch(t, []string{"child"}, res.Candidates)
}

func TestCheckObjectsOnObjectArchived_NonFileObject_ParentArchived_WithActiveBacklink(t *testing.T) {
	// given: child still referenced by another active object
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("other"))
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", []string{"parent", "other"}))

	// when
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: child is not a candidate because "other" is still active
	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.Empty(t, res.Candidates)
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
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", false)

	// then: objects are NOT auto-restored (file-only restore); user restores from bin
	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.Empty(t, res.Candidates)
}

func TestArchiveOrphansOnLinksRemoval_NonFileObject_BecomesCandidate(t *testing.T) {
	// given: a basic object whose link is removed; caller requests skipBin=true
	fx := newFixture(t)
	fx.participantProvider = &mockParticipantProvider{id: "user1"}
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:                  domain.String("child"),
			bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext:    domain.String("parent"),
			bundle.RelationKeyCreatedInContextRef: domain.String("block1"),
			bundle.RelationKeyBacklinks:           domain.StringList([]string{"parent"}),
			bundle.RelationKeyCreator:             domain.String("user1"),
		},
	})

	// when: caller requests skipBin=true (as chat service does for files)
	res, err := fx.ArchiveOrphansOnLinksRemoval(testSpaceId, "parent", []string{"child"}, true, nil)

	// then: non-file object is never archived/deleted — surfaced as a candidate instead
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
	assert.ElementsMatch(t, []string{"child"}, res.Candidates)
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

func TestRestoreOrphansOnLinksAdded_NonFileObject_NotRestored(t *testing.T) {
	// given: basic object is archived, undo re-adds the link
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

	// then: objects are NOT auto-restored (file-only); user restores from bin
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.unarchivedIds)
}

// -- returned-IDs tests for ArchiveOrphansOnLinksRemoval --

func TestArchiveOrphansOnLinksRemoval_ReturnsArchivedIds(t *testing.T) {
	// given: a file with no active backlinks besides the context
	fx := newFixture(t)
	fx.addObject(t, fileObject("file1", "page", []string{"page"}))

	// when
	archivedIds, err := fx.ArchiveOrphansOnLinksRemoval(testSpaceId, "page", []string{"file1"}, false, nil)

	// then: file has empty CreatedInContextRef (collection-created) — excluded by the ref gate
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
	assert.Empty(t, archivedIds)
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

// fileObjectWithRef builds a file with a non-empty CreatedInContextRef — i.e. a file added
// via a block (not via a collection) — eligible for GC since it passes the CreatedInContextRef gate.
func fileObjectWithRef(id, createdInContext, createdInContextRef string, backlinks []string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:                  domain.String(id),
		bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_image)),
		bundle.RelationKeyCreatedInContext:    domain.String(createdInContext),
		bundle.RelationKeyCreatedInContextRef: domain.String(createdInContextRef),
		bundle.RelationKeyBacklinks:           domain.StringList(backlinks),
	}
}

// basicObjectWithRef builds a non-file object with a non-empty CreatedInContextRef — i.e. an
// object created via a block (eligible for orphan candidate collection).
func basicObjectWithRef(id, createdInContext, createdInContextRef string, backlinks []string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:                  domain.String(id),
		bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_basic)),
		bundle.RelationKeyCreatedInContext:    domain.String(createdInContext),
		bundle.RelationKeyCreatedInContextRef: domain.String(createdInContextRef),
		bundle.RelationKeyBacklinks:           domain.StringList(backlinks),
	}
}

func TestArchiveOrphansOnLinksRemoval_FileWithRef_Archived(t *testing.T) {
	// given: a file with non-empty CreatedInContextRef (block-attached file)
	fx := newFixture(t)
	fx.addObject(t, fileObjectWithRef("file1", "page", "block1", []string{"page"}))

	// when
	res, err := fx.ArchiveOrphansOnLinksRemoval(testSpaceId, "page", []string{"file1"}, false, nil)

	// then: file is archived — non-empty ref means it is NOT a collection-created file
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
	assert.ElementsMatch(t, []string{"file1"}, res.Files)
}

func TestCheckObjectsOnObjectArchived_FileWithRef_ParentArchived_Archived(t *testing.T) {
	// given: parent archived, file has a non-empty CreatedInContextRef and no other backlinks
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, fileObjectWithRef("file1", "parent", "block1", []string{"parent"}))

	// when
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: level-1 file with non-empty ref is auto-archived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, res.Files)
	assert.Empty(t, res.Candidates)
}

// -- returned-IDs tests for CheckObjectsOnObjectArchived --

func TestCheckObjectsOnObjectArchived_NoRefFile_NotCollected(t *testing.T) {
	// given: parent archived, file has no other backlinks but an empty CreatedInContextRef
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent"}))

	// when
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: file has empty CreatedInContextRef (collection-created) — excluded by the ref gate
	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.Empty(t, res.Candidates)
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
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", false)

	// then: file restored and returned
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, res.Files)
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

func TestCheckObjectsOnObjectArchived_TwoLevelTree_AllObjectsAreCandidates(t *testing.T) {
	// given
	// parent (being archived)
	//   └→ child  (createdInContext=parent, backlinks=[parent])
	//        └→ grandchild (createdInContext=child, backlinks=[child])
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", []string{"parent"}))
	fx.addObject(t, basicObjectWithRef("grandchild", "child", "block1", []string{"child"}))

	// when
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: BFS descends through objects; both levels surfaced as candidates
	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.ElementsMatch(t, []string{"child", "grandchild"}, res.Candidates)
}

func TestCheckObjectsOnObjectArchived_CycleAB_TerminatesAndCollectsB(t *testing.T) {
	// given: A.createdInContext=B, B.createdInContext=A — cycle
	// Archiving A: BFS visits A (seed), finds B (child of A), visited={A,B}.
	// When processing B, finds A as its child — but A is already in visited → skip.
	fx := newFixture(t)
	fx.addObject(t, basicObjectWithRef("A", "B", "block1", []string{"B"}))
	fx.addObject(t, basicObjectWithRef("B", "A", "block1", []string{"A"}))

	// when — must complete without hanging
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "A", true)

	// then: B is collected as a candidate; traversal terminates
	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.ElementsMatch(t, []string{"B"}, res.Candidates)
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
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", []string{"parent"}))
	fx.addObject(t, basicObjectWithRef("grandchild", "child", "block1", []string{"child", "external"}))
	fx.addObject(t, basicObjectWithRef("greatgrandchild", "grandchild", "block1", []string{"grandchild"}))

	// when
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: grandchild has an external active backlink → it and its subtree are excluded
	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.ElementsMatch(t, []string{"child"}, res.Candidates)
}

func TestCheckObjectsOnObjectArchived_SiblingCrossReference_BothCandidates(t *testing.T) {
	// given: two siblings that reference each other as backlinks.
	// The pending set must treat in-batch siblings as inactive so both are collected.
	// parent
	//   └→ child1 (backlinks=[parent, child2])
	//   └→ child2 (backlinks=[parent, child1])
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObjectWithRef("child1", "parent", "block1", []string{"parent", "child2"}))
	fx.addObject(t, basicObjectWithRef("child2", "parent", "block1", []string{"parent", "child1"}))

	// when
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: both siblings are candidates
	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.ElementsMatch(t, []string{"child1", "child2"}, res.Candidates)
}

func TestCheckObjectsOnObjectArchived_Unarchive_FileOnlyRestored(t *testing.T) {
	// given
	// parent (unarchived)
	//   ├→ file1 (archived file, createdInContext=parent)  → restored
	//   └→ obj   (archived object, createdInContext=parent) → NOT restored
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
		{
			bundle.RelationKeyId:               domain.String("obj"),
			bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext: domain.String("parent"),
			bundle.RelationKeyBacklinks:        domain.StringList([]string{"parent"}),
			bundle.RelationKeyIsArchived:       domain.Bool(true),
		},
	})

	// when
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", false)

	// then: only the file is restored; the object stays archived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, res.Files)
	assert.Empty(t, res.Candidates)
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
	fx.addObject(t, basicObjectWithRef("X", "parent", "block1", []string{"parent", "external"}))
	fx.addObject(t, basicObjectWithRef("Y", "parent", "block1", []string{"X"}))

	// when
	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// then: neither X nor Y is collected
	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.Empty(t, res.Candidates)
}

func TestCheckObjectsOnObjectArchived_MixedTree_Level1FileArchived_RestCandidates(t *testing.T) {
	// parent
	//   ├─ f1   (file, level 1)        → Files
	//   ├─ obj  (object, level 1)      → Candidates
	//   │    └─ f2 (file, level 2)     → Candidates
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, fileObjectWithRef("f1", "parent", "block1", []string{"parent"}))
	fx.addObject(t, basicObjectWithRef("obj", "parent", "block1", []string{"parent"}))
	fx.addObject(t, fileObjectWithRef("f2", "obj", "block1", []string{"obj"}))

	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	// only the level-1 file is auto-archived; the object and the deeper file are candidates
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1"}, res.Files)
	assert.ElementsMatch(t, []string{"obj", "f2"}, res.Candidates)
}

func TestFilterExplicitIds_RemovesFromCleanupSuggestion(t *testing.T) {
	sctx := session.NewContext(session.WithSession("test-token"))
	sctx.SetMessages("block1", []*pb.EventMessage{{
		Value: &pb.EventMessageValueOfObjectCleanupSuggestion{
			ObjectCleanupSuggestion: &pb.EventObjectCleanupSuggestion{
				ObjectIds: []string{"a", "b", "c"},
				ContextId: "ctx",
				Trigger:   pb.EventObjectCleanupSuggestion_archive,
			},
		},
	}})

	FilterExplicitIds(sctx, []string{"b"})

	msgs := sctx.GetMessages()
	require.Len(t, msgs, 1)
	msg := msgs[0].Value.(*pb.EventMessageValueOfObjectCleanupSuggestion)
	assert.ElementsMatch(t, []string{"a", "c"}, msg.ObjectCleanupSuggestion.ObjectIds)
	assert.Equal(t, "ctx", msg.ObjectCleanupSuggestion.ContextId)
}

// -- createdInContextIgnored gate --

// ignoredBasicObject builds a candidate object that the user has ignored.
func ignoredBasicObject(id, createdInContext, ref string, backlinks []string) objectstore.TestObject {
	obj := basicObjectWithRef(id, createdInContext, ref, backlinks)
	obj[bundle.RelationKeyCreatedInContextIgnored] = domain.Bool(true)
	return obj
}

func TestCheckObjectsOnObjectArchived_IgnoredObject_NotACandidate(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, ignoredBasicObject("child", "parent", "block1", []string{"parent"}))

	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.Empty(t, res.Candidates)
}

func TestCheckObjectsOnObjectArchived_IgnoredLevel1File_NotAutoArchived(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	ignoredFile := fileObjectWithRef("f1", "parent", "block1", []string{"parent"})
	ignoredFile[bundle.RelationKeyCreatedInContextIgnored] = domain.Bool(true)
	fx.addObject(t, ignoredFile)

	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.Empty(t, res.Candidates)
}

func TestArchiveOrphansOnLinksRemoval_IgnoredObject_NotACandidate(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, ignoredBasicObject("child", "parent", "block1", []string{"parent"}))

	res, err := fx.ArchiveOrphansOnLinksRemoval(testSpaceId, "parent", []string{"child"}, false, nil)

	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.Empty(t, res.Candidates)
}
