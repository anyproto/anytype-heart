package filegc

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testSpaceId = "test-space"

// -- fixture --

type fixture struct {
	*fileGC
	store    *objectstore.StoreFixture
	archiver *mockArchiver
}

func newFixture(t *testing.T) *fixture {
	store := objectstore.NewStoreFixture(t)
	archiver := &mockArchiver{}
	gc := &fileGC{
		objectStore:      store,
		objectArchiver:   archiver,
		backlinksWatcher: &noopFlusher{},
		componentCtx:     context.Background(),
	}
	return &fixture{
		fileGC:   gc,
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

// -- mocks --

type mockArchiver struct {
	archivedIds   []string
	unarchivedIds []string
}

func (m *mockArchiver) SetListIsArchived(_ context.Context, objectIds []string, isArchived bool) error {
	if isArchived {
		m.archivedIds = append(m.archivedIds, objectIds...)
	} else {
		m.unarchivedIds = append(m.unarchivedIds, objectIds...)
	}
	return nil
}

type noopFlusher struct{}

func (n *noopFlusher) Name() string              { return "noopFlusher" }
func (n *noopFlusher) Init(_ *app.App) error     { return nil }
func (n *noopFlusher) FlushUpdates()             {}

// -- archive direction tests --

func TestCheckFilesOnObjectArchived_ParentArchived_NoOtherBacklinks(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent"}))

	// when
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckFilesOnObjectArchived_ParentArchived_WithActiveBacklink(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("other"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "other"}))

	// when
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

func TestCheckFilesOnObjectArchived_ParentArchived_OtherBacklinksAllArchived(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, archivedObject("other"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "other"}))

	// when
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckFilesOnObjectArchived_ParentArchived_OtherBacklinksAllDeleted(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, deletedObject("other"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "other"}))

	// when
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "parent", true)

	// then
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckFilesOnObjectArchived_BacklinkerArchived_ParentStillActive(t *testing.T) {
	// given: file's parent is active, objectId is just a backlinker
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file must not be touched — parent is still active (safety gate)
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

func TestCheckFilesOnObjectArchived_BacklinkerArchived_ParentArchived_NoOtherBacklinks(t *testing.T) {
	// given: file's parent is already archived, backlinker is the last reference
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file should be archived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckFilesOnObjectArchived_BacklinkerArchived_ParentArchived_OtherActiveBacklink(t *testing.T) {
	// given: file's parent is archived, but another object still actively links to the file
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, regularObject("activeRef"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker", "activeRef"}))

	// when: backlinker is archived
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file kept because activeRef is still active
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

func TestCheckFilesOnObjectArchived_BacklinkerArchived_ParentArchived_OtherBacklinksAllArchived(t *testing.T) {
	// given: file's parent and all other backlinks are archived
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("backlinker"))
	fx.addObject(t, archivedObject("archivedRef"))
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker", "archivedRef"}))

	// when: backlinker is archived
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file should be archived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckFilesOnObjectArchived_BacklinkerArchived_ParentMissingFromStore(t *testing.T) {
	// given: file's parent is not in the store at all (fully deleted), backlinker is the last reference
	fx := newFixture(t)
	fx.addObject(t, regularObject("backlinker"))
	// "parent" is intentionally not added to the store
	fx.addObject(t, fileObject("file1", "parent", []string{"parent", "backlinker"}))

	// when: backlinker is archived
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "backlinker", true)

	// then: file archived — missing parent treated as deleted (safety gate passes)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.archivedIds)
}

func TestCheckFilesOnObjectArchived_ObjectIsFile_EarlyReturn(t *testing.T) {
	// given: the archived object is itself a file
	fx := newFixture(t)
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("fileContext"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_image)),
		},
	})

	// when
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "fileContext", true)

	// then: returns immediately, no GC triggered
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.archivedIds)
}

// -- links restored (undo) tests --

func TestCheckFilesOnLinksRestored_RestoresArchivedFile(t *testing.T) {
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
	err := fx.CheckFilesOnLinksRestored(testSpaceId, "page", []string{"file1"})

	// then: file is unarchived
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.unarchivedIds)
}

func TestCheckFilesOnLinksRestored_IgnoresFileFromDifferentContext(t *testing.T) {
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
	err := fx.CheckFilesOnLinksRestored(testSpaceId, "page", []string{"file1"})

	// then: file is not touched — wrong context
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.unarchivedIds)
}

func TestCheckFilesOnLinksRestored_IgnoresAlreadyActiveFile(t *testing.T) {
	// given: file is not archived (was never GC'd or was already restored)
	fx := newFixture(t)
	fx.addObject(t, fileObject("file1", "page", []string{"page"}))

	// when
	err := fx.CheckFilesOnLinksRestored(testSpaceId, "page", []string{"file1"})

	// then: nothing to do
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.unarchivedIds)
}

// -- unarchive direction tests --

func TestCheckFilesOnObjectArchived_Unarchive_NoOtherBacklinks(t *testing.T) {
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
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "parent", false)

	// then: file restored
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"file1"}, fx.archiver.unarchivedIds)
}

func TestCheckFilesOnObjectArchived_Unarchive_HasOtherBacklinks(t *testing.T) {
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
	err := fx.CheckFilesOnObjectArchived(testSpaceId, "parent", false)

	// then: file kept archived because it has other backlinks
	require.NoError(t, err)
	assert.Empty(t, fx.archiver.unarchivedIds)
}
