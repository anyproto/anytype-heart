package smartblock

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/objectgc"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// objectGCCallRecorder records calls to ArchiveOrphansOnLinksRemoval and RestoreOrphansOnLinksAdded
// via buffered channels so goroutine timing works in tests.
type objectGCCallRecorder struct {
	removedCh  chan linksRemovalCall
	restoredCh chan linksRestoredCall
}

type linksRemovalCall struct {
	spaceId   string
	contextId string
	links     []string
	skipBin   bool
}

type linksRestoredCall struct {
	spaceId   string
	contextId string
	links     []string
}

func newObjectGCCallRecorder() *objectGCCallRecorder {
	return &objectGCCallRecorder{
		removedCh:  make(chan linksRemovalCall, 4),
		restoredCh: make(chan linksRestoredCall, 4),
	}
}

func (r *objectGCCallRecorder) Init(_ *app.App) error         { return nil }
func (r *objectGCCallRecorder) Name() string                  { return "test-objectgc" }
func (r *objectGCCallRecorder) Run(_ context.Context) error   { return nil }
func (r *objectGCCallRecorder) Close(_ context.Context) error { return nil }
func (r *objectGCCallRecorder) CheckObjectsOnObjectArchived(_, _ string, _ bool) (objectgc.OrphanCandidates, error) {
	return objectgc.OrphanCandidates{}, nil
}
func (r *objectGCCallRecorder) ArchiveOrphansOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, _ []string) (objectgc.OrphanCandidates, error) {
	r.removedCh <- linksRemovalCall{spaceId: spaceId, contextId: contextId, links: removedLinks, skipBin: skipBin}
	return objectgc.OrphanCandidates{}, nil
}
func (r *objectGCCallRecorder) RestoreOrphansOnLinksAdded(spaceId, contextId string, addedLinks []string) ([]string, error) {
	r.restoredCh <- linksRestoredCall{spaceId: spaceId, contextId: contextId, links: addedLinks}
	return nil, nil
}

// TestSmartBlock_ObjectGC_LinksAdded_TriggersRestore verifies that Apply calls
// RestoreOrphansOnLinksAdded when links are added from an empty state (the undo case).
// This exercises the fix for the len(linksBefore) > 0 guard that used to skip the diff.
func TestSmartBlock_ObjectGC_LinksAdded_TriggersRestore(t *testing.T) {
	// given – object with no link blocks initially
	objectId := "root"
	fx := newFixture(objectId, t)
	fx.init(t, []*model.Block{{Id: objectId}})

	recorder := newObjectGCCallRecorder()
	fx.objectGC = recorder

	fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
	fx.eventSender.EXPECT().SendToSession(mock.Anything, mock.Anything).Maybe()

	// when – add a link block pointing to "file1" (simulates undo that re-adds a file block)
	s := fx.NewState()
	s.Add(simple.New(&model.Block{
		Id: "link1",
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{TargetBlockId: "file1"},
		},
	}))
	require.NoError(t, s.InsertTo(objectId, model.Block_Inner, "link1"))
	require.NoError(t, fx.Apply(s))

	// then – RestoreOrphansOnLinksAdded must be called with "file1"
	select {
	case call := <-recorder.restoredCh:
		assert.Equal(t, testSpaceId, call.spaceId)
		assert.Equal(t, objectId, call.contextId)
		assert.Equal(t, []string{"file1"}, call.links)
	case <-time.After(time.Second):
		t.Fatal("timeout: RestoreOrphansOnLinksAdded was not called")
	}
}

// TestSmartBlock_ObjectGC_LinksRemoved_TriggersGC verifies that Apply calls
// ArchiveOrphansOnLinksRemoval when a link block is removed.
func TestSmartBlock_ObjectGC_LinksRemoved_TriggersGC(t *testing.T) {
	// given – object starts empty
	objectId := "root"
	fx := newFixture(objectId, t)
	fx.init(t, []*model.Block{{Id: objectId}})

	recorder := newObjectGCCallRecorder()
	fx.objectGC = recorder

	fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
	fx.eventSender.EXPECT().SendToSession(mock.Anything, mock.Anything).Maybe()

	// First Apply: add link block to establish links in the doc state
	s1 := fx.NewState()
	s1.Add(simple.New(&model.Block{
		Id: "link1",
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{TargetBlockId: "file1"},
		},
	}))
	require.NoError(t, s1.InsertTo(objectId, model.Block_Inner, "link1"))
	require.NoError(t, fx.Apply(s1))

	// Drain the restore call triggered by the first Apply
	select {
	case <-recorder.restoredCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for restore call from first apply")
	}

	// when – remove the link block (simulates user deleting a file block)
	s2 := fx.NewState()
	s2.Unlink("link1")
	require.NoError(t, fx.Apply(s2))

	// then – ArchiveOrphansOnLinksRemoval must be called with "file1"
	select {
	case call := <-recorder.removedCh:
		assert.Equal(t, testSpaceId, call.spaceId)
		assert.Equal(t, objectId, call.contextId)
		assert.Contains(t, call.links, "file1")
	case <-time.After(time.Second):
		t.Fatal("timeout: ArchiveOrphansOnLinksRemoval was not called")
	}
}
