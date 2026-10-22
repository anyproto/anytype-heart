package objectgc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

// These tests pin the contract established when autoArchiveOrphanFiles was disabled: on every
// context-driven path the GC reports orphans and mutates nothing. The invariant that matters is
// "the archiver was never called" — asserting an empty res.Files alone would still pass if a
// caller-independent archive slipped back in.
//
// Flipping autoArchiveOrphanFiles back to true is expected to fail these tests. That is the point.

// creatorFileWithRef builds a GC-eligible orphan file attributed to a specific creator.
func creatorFileWithRef(id, createdInContext, ref, creator string, backlinks []string) objectstore.TestObject {
	obj := fileObjectWithRef(id, createdInContext, ref, backlinks)
	obj[bundle.RelationKeyCreator] = domain.String(creator)
	return obj
}

func TestCheckObjectsOnObjectArchived_Level1File_IsCandidateNotAutoArchived(t *testing.T) {
	// A level-1 orphan file used to be auto-archived. It must now be surfaced for confirmation,
	// exactly like the level-2 files and the objects already were.
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, fileObjectWithRef("f1", "parent", "block1", []string{"parent"}))

	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	require.NoError(t, err)
	assert.Empty(t, res.Files, "nothing may be auto-archived")
	assert.ElementsMatch(t, []string{"f1"}, res.Candidates)
	assert.Empty(t, fx.archiver.archivedIds)
	assert.Empty(t, fx.deleter.deletedIds)
}

func TestCheckObjectsOnObjectArchived_Unarchive_DoesNotRestoreFiles(t *testing.T) {
	// Unarchiving a parent used to pull its archived child files back out of the bin.
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	archivedFile := fileObject("f1", "parent", []string{"parent"})
	archivedFile[bundle.RelationKeyIsArchived] = domain.Bool(true)
	fx.addObject(t, archivedFile)

	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", false)

	require.NoError(t, err)
	assert.Empty(t, res.Files, "nothing may be auto-restored")
	assert.Empty(t, res.Candidates)
	assert.Empty(t, fx.archiver.unarchivedIds)
}

func TestArchiveOrphansOnLinksRemoval_File_IsCandidateNotArchived(t *testing.T) {
	// Removing a link from a page: the orphaned file is reported, not archived.
	fx := newFixture(t)
	fx.addObject(t, fileObjectWithRef("f1", "parent", "block1", []string{"parent"}))

	res, err := fx.ArchiveOrphansOnLinksRemoval(testSpaceId, "parent", []string{"f1"}, false, nil)

	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.ElementsMatch(t, []string{"f1"}, res.Candidates)
	assert.Empty(t, fx.archiver.archivedIds)
	assert.Empty(t, fx.deleter.deletedIds)
}

func TestRestoreOrphansOnLinksAdded_DoesNotRestore(t *testing.T) {
	// Re-adding a link (undo) used to unarchive the file that link pointed at.
	fx := newFixture(t)
	archivedFile := fileObject("f1", "page", []string{"page"})
	archivedFile[bundle.RelationKeyIsArchived] = domain.Bool(true)
	fx.addObject(t, archivedFile)

	restored, err := fx.RestoreOrphansOnLinksAdded(testSpaceId, "page", []string{"f1"})

	require.NoError(t, err)
	assert.Empty(t, restored)
	assert.Empty(t, fx.archiver.unarchivedIds)
}

func TestArchiveOrphansOnLinksRemoval_SkipBin_DeletesOwnFile(t *testing.T) {
	// The chat path is unaffected by the switch: it still permanently deletes.
	fx := newFixture(t)
	fx.addObject(t, creatorFileWithRef("f1", "chat", "msg1", "user1", []string{"chat"}))

	res, err := fx.ArchiveOrphansOnLinksRemoval(testSpaceId, "chat", []string{"f1"}, true, nil)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1"}, fx.deleter.deletedIds)
	assert.Empty(t, res.Files)
	assert.Empty(t, fx.archiver.archivedIds)
}

func TestArchiveOrphansOnLinksRemoval_SkipBin_DeletesOtherParticipantsFile(t *testing.T) {
	// Previously a file uploaded by someone else fell back to archive. Only admins moderate chat,
	// so a moderated message now takes its attachments with it regardless of who uploaded them.
	fx := newFixture(t)
	fx.addObject(t, creatorFileWithRef("f1", "chat", "msg1", "user2", []string{"chat"}))

	res, err := fx.ArchiveOrphansOnLinksRemoval(testSpaceId, "chat", []string{"f1"}, true, nil)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1"}, fx.deleter.deletedIds, "another participant's file is deleted too")
	assert.Empty(t, res.Files, "never falls back to archiving")
	assert.Empty(t, fx.archiver.archivedIds)
}

// A file nested under an orphan object (level >= 2) was already a candidate; it stays one.
func TestCheckObjectsOnObjectArchived_DeepFile_StillCandidate(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", []string{"parent"}))
	fx.addObject(t, fileObjectWithRef("f1", "child", "block1", []string{"child"}))

	res, err := fx.CheckObjectsOnObjectArchived(testSpaceId, "parent", true)

	require.NoError(t, err)
	assert.Empty(t, res.Files)
	assert.ElementsMatch(t, []string{"child", "f1"}, res.Candidates)
	assert.Empty(t, fx.archiver.archivedIds)
}
