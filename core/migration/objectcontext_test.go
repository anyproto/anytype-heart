package migration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/block/detailservice/mock_detailservice"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

// blockIdAt returns a BSON ObjectId created at ts; n tells apart ids created in the same second
func blockIdAt(ts int64, n int) string {
	return fmt.Sprintf("%08x%016x", ts, n)
}

// objectCreatedAt is a creation date inside the object migration scope
var objectCreatedAt = objectContextCutoff - 30*24*60*60

func TestService_findBestContext(t *testing.T) {
	s := &service{}
	file := contextTarget{id: "file1", isFile: true, createdAt: 9999999999}
	sources := map[string]int64{"page1": 0, "page2": 0, "chat1": 0}

	t.Run("empty links and no chat returns nil", func(t *testing.T) {
		result := s.findBestContext(file, nil, nil, sources)
		assert.Nil(t, result)
	})

	t.Run("block link only", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"},
		}

		result := s.findBestContext(file, links, nil, sources)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId)
		assert.Equal(t, "507f1f77bcf86cd799439011", result.blockId)
		assert.Empty(t, result.messageId)
	})

	t.Run("chat only", func(t *testing.T) {
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    1000,
		}

		result := s.findBestContext(file, nil, chatCtx, sources)

		require.NotNil(t, result)
		assert.Equal(t, "chat1", result.objectId)
		assert.Equal(t, "msg1", result.messageId)
		assert.Empty(t, result.blockId)
	})

	t.Run("relation link only (fallback)", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", RelationKey: "attachment"},
		}

		result := s.findBestContext(file, links, nil, sources)

		require.NotNil(t, result)
		assert.Equal(t, contextKindRelation, result.kind)
		assert.Equal(t, "page1", result.objectId)
		assert.Equal(t, "attachment", result.relationKey)
		assert.Empty(t, result.ref())
		assert.Equal(t, int64(0), result.timestamp)
	})

	t.Run("block earlier than chat returns block", func(t *testing.T) {
		// 507f1f77 = timestamp 1350844279
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"},
		}
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    2000000000, // Later than block
		}

		result := s.findBestContext(file, links, chatCtx, sources)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId)
		assert.Equal(t, "507f1f77bcf86cd799439011", result.blockId)
	})

	t.Run("chat earlier than block returns chat", func(t *testing.T) {
		// 507f1f77 = timestamp 1350844279
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"},
		}
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    1000, // Earlier than block
		}

		result := s.findBestContext(file, links, chatCtx, sources)

		require.NotNil(t, result)
		assert.Equal(t, "chat1", result.objectId)
		assert.Equal(t, "msg1", result.messageId)
	})

	t.Run("relation link and chat prefers chat (chat has timestamp)", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", RelationKey: "attachment"},
		}
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    1000,
		}

		result := s.findBestContext(file, links, chatCtx, sources)

		require.NotNil(t, result)
		assert.Equal(t, "chat1", result.objectId)
		assert.Equal(t, "msg1", result.messageId)
	})

	t.Run("chat newer than file is ignored", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", RelationKey: "attachment"},
		}
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    file.createdAt + 1000, // Newer than file
		}

		result := s.findBestContext(file, links, chatCtx, sources)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId) // Falls back to relation
	})

	t.Run("chat that is not indexed is ignored", func(t *testing.T) {
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat2",
			MessageId:    "msg1",
			CreatedAt:    1000,
		}

		result := s.findBestContext(file, nil, chatCtx, sources)

		assert.Nil(t, result)
	})

	t.Run("block newer than file is ignored", func(t *testing.T) {
		smallFile := contextTarget{id: "file1", isFile: true, createdAt: 1000}
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"}, // timestamp 1350844279 > 1000
			{SourceID: "page2", RelationKey: "attachment"},
		}

		result := s.findBestContext(smallFile, links, nil, sources)

		require.NotNil(t, result)
		assert.Equal(t, "page2", result.objectId) // Falls back to relation
	})

	t.Run("object chat message outside the window is ignored", func(t *testing.T) {
		object := contextTarget{id: "obj1", createdAt: objectCreatedAt}
		chatCtx := &ChatAttachmentContext{
			ChatObjectId: "chat1",
			MessageId:    "msg1",
			CreatedAt:    objectCreatedAt - objectContextTimeTolerance - 1, // message edited to attach it later
		}

		result := s.findBestContext(object, nil, chatCtx, sources)

		assert.Nil(t, result)
	})
}

func TestService_filterLinks(t *testing.T) {
	s := &service{}

	t.Run("filters self-references", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "file1", RelationKey: "some"},
			{SourceID: "page1", RelationKey: "attachment"},
		}

		result := s.filterLinks("file1", links)

		require.Len(t, result, 1)
		assert.Equal(t, "page1", result[0].SourceID)
	})

	t.Run("filters system relations", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "creator1", RelationKey: string(bundle.RelationKeyCreator)},
			{SourceID: "page1", RelationKey: "attachment"},
		}

		result := s.filterLinks("file1", links)

		require.Len(t, result, 1)
		assert.Equal(t, "page1", result[0].SourceID)
	})
}

func TestService_findBlockContext(t *testing.T) {
	s := &service{}

	t.Run("finds earliest block", func(t *testing.T) {
		file := contextTarget{id: "file1", isFile: true, createdAt: 9999999999}
		sources := map[string]int64{"page1": 0, "page2": 0}
		// 507f1f77 = 1350844279, 600000000 = 1610612736
		links := []spaceindex.IncomingLink{
			{SourceID: "page2", BlockID: "60000000bcf86cd799439011"}, // Later
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"}, // Earlier
		}

		result := s.findBlockContext(file, links, sources)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId)
	})

	t.Run("skips blocks newer than file", func(t *testing.T) {
		file := contextTarget{id: "file1", isFile: true, createdAt: 1000}
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"}, // timestamp > 1000
		}

		result := s.findBlockContext(file, links, map[string]int64{"page1": 0})

		assert.Nil(t, result)
	})

	t.Run("file accepts a block made long before it", func(t *testing.T) {
		file := contextTarget{id: "file1", isFile: true, createdAt: objectCreatedAt}
		blockId := blockIdAt(objectCreatedAt-24*60*60, 1) // empty file block filled a day later
		links := []spaceindex.IncomingLink{{SourceID: "page1", BlockID: blockId}}

		result := s.findBlockContext(file, links, map[string]int64{"page1": 0})

		require.NotNil(t, result)
		assert.Equal(t, blockId, result.blockId)
	})

	t.Run("object accepts a block made right after it", func(t *testing.T) {
		object := contextTarget{id: "obj1", createdAt: objectCreatedAt}
		blockId := blockIdAt(objectCreatedAt+1, 1)
		links := []spaceindex.IncomingLink{{SourceID: "page1", BlockID: blockId}}
		want := &contextInfo{
			kind:          contextKindBlock,
			objectId:      "page1",
			blockId:       blockId,
			timestamp:     objectCreatedAt + 1,
			sourceCreated: objectCreatedAt - 100,
		}

		got := s.findBlockContext(object, links, map[string]int64{"page1": objectCreatedAt - 100})

		assert.Equal(t, want, got)
	})

	t.Run("object rejects an older block that got the link later", func(t *testing.T) {
		// a mention typed into a paragraph written a year before the object
		object := contextTarget{id: "obj1", createdAt: objectCreatedAt}
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: blockIdAt(objectCreatedAt-365*24*60*60, 1)},
		}

		result := s.findBlockContext(object, links, map[string]int64{"page1": 0})

		assert.Nil(t, result)
	})

	t.Run("object rejects a block made after the window", func(t *testing.T) {
		object := contextTarget{id: "obj1", createdAt: objectCreatedAt}
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: blockIdAt(objectCreatedAt+objectContextTimeTolerance+1, 1)},
		}

		result := s.findBlockContext(object, links, map[string]int64{"page1": 0})

		assert.Nil(t, result)
	})

	t.Run("block copied into a newer object is rejected, the original wins", func(t *testing.T) {
		// page2 is a duplicate of page1 made a day later: it carries the link block with the same id
		object := contextTarget{id: "obj1", createdAt: objectCreatedAt}
		blockId := blockIdAt(objectCreatedAt, 1)
		links := []spaceindex.IncomingLink{
			{SourceID: "page2", BlockID: blockId},
			{SourceID: "page1", BlockID: blockId},
		}
		sources := map[string]int64{"page1": objectCreatedAt - 100, "page2": objectCreatedAt + 24*60*60}

		result := s.findBlockContext(object, links, sources)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId)
	})

	t.Run("same block in two objects resolves to the earlier object", func(t *testing.T) {
		// page2 is a duplicate made within the window, so the copied block passes the checks
		object := contextTarget{id: "obj1", createdAt: objectCreatedAt}
		blockId := blockIdAt(objectCreatedAt, 1)
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: blockId},
			{SourceID: "page2", BlockID: blockId},
		}
		sources := map[string]int64{"page1": objectCreatedAt - 100, "page2": objectCreatedAt - 200}

		result := s.findBlockContext(object, links, sources)

		require.NotNil(t, result)
		assert.Equal(t, "page2", result.objectId)
	})

	t.Run("skips context objects that are not indexed", func(t *testing.T) {
		object := contextTarget{id: "obj1", createdAt: objectCreatedAt}
		links := []spaceindex.IncomingLink{{SourceID: "page1", BlockID: blockIdAt(objectCreatedAt, 1)}}

		result := s.findBlockContext(object, links, map[string]int64{})

		assert.Nil(t, result)
	})

	t.Run("skips context objects created after the target", func(t *testing.T) {
		object := contextTarget{id: "obj1", createdAt: objectCreatedAt}
		links := []spaceindex.IncomingLink{{SourceID: "page1", BlockID: blockIdAt(objectCreatedAt, 1)}}

		result := s.findBlockContext(object, links, map[string]int64{"page1": objectCreatedAt + 24*60*60})

		assert.Nil(t, result)
	})
}

func TestService_findRelationContext(t *testing.T) {
	s := &service{}
	file := contextTarget{id: "file1", isFile: true, createdAt: 9999999999}
	sources := map[string]int64{"page1": 0, "page2": 0}

	t.Run("returns first relation by key", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page2", RelationKey: "b"},
			{SourceID: "page1", RelationKey: "a"},
		}

		result := s.findRelationContext(file, links, sources)

		require.NotNil(t, result)
		assert.Equal(t, "page1", result.objectId)
	})

	t.Run("skips block links", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"},
			{SourceID: "page2", RelationKey: "attachment"},
		}

		result := s.findRelationContext(file, links, sources)

		require.NotNil(t, result)
		assert.Equal(t, "page2", result.objectId)
	})

	t.Run("returns nil if only block links", func(t *testing.T) {
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", BlockID: "507f1f77bcf86cd799439011"},
		}

		result := s.findRelationContext(file, links, sources)

		assert.Nil(t, result)
	})

	t.Run("skips sources created after the target", func(t *testing.T) {
		object := contextTarget{id: "obj1", createdAt: objectCreatedAt}
		links := []spaceindex.IncomingLink{
			{SourceID: "page1", RelationKey: "a"},
			{SourceID: "page2", RelationKey: "b"},
		}
		sources := map[string]int64{"page1": objectCreatedAt + 24*60*60, "page2": objectCreatedAt - 100}

		result := s.findRelationContext(object, links, sources)

		require.NotNil(t, result)
		assert.Equal(t, "page2", result.objectId)
	})
}

func TestNewContextTarget(t *testing.T) {
	for _, tc := range []struct {
		name    string
		details map[domain.RelationKey]domain.Value
		want    contextTarget
		wantOk  bool
	}{
		{
			name: "file is dated by addedDate",
			details: map[domain.RelationKey]domain.Value{
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_image)),
				bundle.RelationKeyAddedDate:      domain.Int64(1000),
				bundle.RelationKeyCreatedDate:    domain.Int64(500),
			},
			want:   contextTarget{id: "id1", isFile: true, createdAt: 1000},
			wantOk: true,
		},
		{
			name: "file without addedDate is out of scope",
			details: map[domain.RelationKey]domain.Value{
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_file)),
			},
			want:   contextTarget{id: "id1", isFile: true},
			wantOk: false,
		},
		{
			name: "object is dated by createdDate",
			details: map[domain.RelationKey]domain.Value{
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyCreatedDate:    domain.Int64(objectCreatedAt),
			},
			want:   contextTarget{id: "id1", createdAt: objectCreatedAt},
			wantOk: true,
		},
		{
			name: "object created after the cutoff is out of scope",
			details: map[domain.RelationKey]domain.Value{
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyCreatedDate:    domain.Int64(objectContextCutoff + 1),
			},
			want:   contextTarget{id: "id1", createdAt: objectContextCutoff + 1},
			wantOk: false,
		},
		{
			name: "imported object is out of scope",
			details: map[domain.RelationKey]domain.Value{
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyCreatedDate:    domain.Int64(objectCreatedAt),
				bundle.RelationKeyOrigin:         domain.Int64(int64(model.ObjectOrigin_import)),
			},
			want:   contextTarget{id: "id1"},
			wantOk: false,
		},
		{
			name: "object created from a snapshot is out of scope",
			details: map[domain.RelationKey]domain.Value{
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_note)),
				bundle.RelationKeyCreatedDate:    domain.Int64(objectCreatedAt),
				bundle.RelationKeyAddedDate:      domain.Int64(objectCreatedAt + 1000),
			},
			want:   contextTarget{id: "id1"},
			wantOk: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			details := domain.NewDetails()
			details.SetString(bundle.RelationKeyId, "id1")
			for key, value := range tc.details {
				details.Set(key, value)
			}

			// when
			got, ok := newContextTarget(details)

			// then
			assert.Equal(t, tc.wantOk, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNew(t *testing.T) {
	service := New()

	assert.NotNil(t, service)
	assert.Equal(t, CName, service.Name())
}

func TestRunMigrationsWhenIdleGate(t *testing.T) {
	const (
		spaceId     = "space1"
		workspaceId = "workspace1"
	)

	t.Run("returns without polling when migration version is current", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:                     domain.String(workspaceId),
				bundle.RelationKeySpaceId:                domain.String(spaceId),
				bundle.RelationKeyMigrationObjectContext: domain.Int64(domain.MigrationObjectContextVersion),
			},
		})
		// only objectStore is set: entering the polling loop would panic on the nil deps
		s := &service{objectStore: store}

		// when
		done := make(chan struct{})
		go func() {
			s.RunMigrationsWhenIdle(spaceId, threads.DerivedSmartblockIds{Workspace: workspaceId})
			close(done)
		}()

		// then
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("RunMigrationsWhenIdle did not return for an already-migrated space")
		}
	})

	t.Run("gate is open when migration version is older", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:                     domain.String(workspaceId),
				bundle.RelationKeySpaceId:                domain.String(spaceId),
				bundle.RelationKeyMigrationObjectContext: domain.Int64(domain.MigrationObjectContextVersion - 1),
			},
		})
		s := &service{objectStore: store}

		// then
		assert.False(t, s.isObjectContextMigrationDone(store.SpaceIndex(spaceId), workspaceId))
	})

	t.Run("gate is open when workspace is not indexed", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		s := &service{objectStore: store}

		// then
		assert.False(t, s.isObjectContextMigrationDone(store.SpaceIndex(spaceId), workspaceId))
	})
}

const (
	testSpaceId        = "space1"
	testWorkspaceId    = "workspace1"
	myParticipantId    = "participant-me"
	otherParticipantId = "participant-other"
)

type fakeAccountService struct {
	participantId string
}

func (a fakeAccountService) MyParticipantId(string) string {
	return a.participantId
}

type fixture struct {
	*service
	objectStore    *objectstore.StoreFixture
	detailsService *mock_detailservice.MockService
}

func newFixture(t *testing.T, permissions model.ParticipantPermissions) *fixture {
	objectStore := objectstore.NewStoreFixture(t)
	detailsService := mock_detailservice.NewMockService(t)
	objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:      domain.String(testWorkspaceId),
			bundle.RelationKeySpaceId: domain.String(testSpaceId),
		},
		{
			bundle.RelationKeyId:                     domain.String(myParticipantId),
			bundle.RelationKeySpaceId:                domain.String(testSpaceId),
			bundle.RelationKeyResolvedLayout:         domain.Int64(int64(model.ObjectType_participant)),
			bundle.RelationKeyParticipantPermissions: domain.Int64(int64(permissions)),
		},
	})
	return &fixture{
		service: &service{
			objectStore:    objectStore,
			detailsService: detailsService,
			accountService: fakeAccountService{participantId: myParticipantId},
		},
		objectStore:    objectStore,
		detailsService: detailsService,
	}
}

// addPage adds a page created at createdAt by creator
func (fx *fixture) addPage(t *testing.T, id string, createdAt int64, creator string) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String(id),
			bundle.RelationKeySpaceId:        domain.String(testSpaceId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedDate:    domain.Int64(createdAt),
			bundle.RelationKeyCreator:        domain.String(creator),
		},
	})
}

func (fx *fixture) addLinks(t *testing.T, sourceId string, links ...spaceindex.OutgoingLink) {
	require.NoError(t, fx.objectStore.SpaceIndex(testSpaceId).UpdateObjectLinksDetailed(context.Background(), sourceId, links))
}

func (fx *fixture) expectMigrationDone() {
	fx.detailsService.EXPECT().SetDetailsInternal(testWorkspaceId, []domain.Detail{
		{Key: bundle.RelationKeyMigrationObjectContext, Value: domain.Int64(domain.MigrationObjectContextVersion)},
	}).Return(nil).Once()
}

func TestRunObjectContextMigration(t *testing.T) {
	t.Run("marks migration done when nothing needs migration", func(t *testing.T) {
		// given
		fx := newFixture(t, model.ParticipantPermissions_Owner)
		fx.expectMigrationDone()

		// when
		err := fx.runObjectContextMigration(context.Background(), testSpaceId, testWorkspaceId)

		// then
		require.NoError(t, err)
	})

	t.Run("object created in a link block gets the block context", func(t *testing.T) {
		// given
		fx := newFixture(t, model.ParticipantPermissions_Owner)
		fx.addPage(t, "parent", objectCreatedAt-1000, otherParticipantId)
		fx.addPage(t, "child", objectCreatedAt, otherParticipantId)
		blockId := blockIdAt(objectCreatedAt+1, 1)
		fx.addLinks(t, "parent", spaceindex.OutgoingLink{TargetID: "child", BlockID: blockId})
		fx.detailsService.EXPECT().SetCreatedInContextInternal("child", "parent", blockId).Return(nil).Once()
		fx.expectMigrationDone()

		// when
		err := fx.runObjectContextMigration(context.Background(), testSpaceId, testWorkspaceId)

		// then
		require.NoError(t, err)
	})

	t.Run("relation fallback is a dry-run for objects", func(t *testing.T) {
		// given
		fx := newFixture(t, model.ParticipantPermissions_Owner)
		fx.addPage(t, "parent", objectCreatedAt-1000, myParticipantId)
		fx.addPage(t, "child", objectCreatedAt, myParticipantId)
		fx.addLinks(t, "parent", spaceindex.OutgoingLink{TargetID: "child", RelationKey: "assignee"})
		fx.expectMigrationDone()

		// when
		err := fx.runObjectContextMigration(context.Background(), testSpaceId, testWorkspaceId)

		// then
		require.NoError(t, err)
		fx.detailsService.AssertNotCalled(t, "SetCreatedInContextInternal")
	})

	t.Run("relation fallback is written for files", func(t *testing.T) {
		// given
		fx := newFixture(t, model.ParticipantPermissions_Owner)
		fx.addPage(t, "parent", objectCreatedAt-1000, myParticipantId)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("file1"),
				bundle.RelationKeySpaceId:        domain.String(testSpaceId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_image)),
				bundle.RelationKeyAddedDate:      domain.Int64(objectCreatedAt),
				bundle.RelationKeyCreator:        domain.String(myParticipantId),
			},
		})
		fx.addLinks(t, "parent", spaceindex.OutgoingLink{TargetID: "file1", RelationKey: "picture"})
		fx.detailsService.EXPECT().SetCreatedInContextInternal("file1", "parent", "").Return(nil).Once()
		fx.expectMigrationDone()

		// when
		err := fx.runObjectContextMigration(context.Background(), testSpaceId, testWorkspaceId)

		// then
		require.NoError(t, err)
	})

	t.Run("member migrates only the objects they created", func(t *testing.T) {
		// given
		fx := newFixture(t, model.ParticipantPermissions_Writer)
		fx.addPage(t, "parent", objectCreatedAt-1000, otherParticipantId)
		fx.addPage(t, "mine", objectCreatedAt, myParticipantId)
		fx.addPage(t, "theirs", objectCreatedAt, otherParticipantId)
		mineBlockId := blockIdAt(objectCreatedAt, 1)
		fx.addLinks(t, "parent",
			spaceindex.OutgoingLink{TargetID: "mine", BlockID: mineBlockId},
			spaceindex.OutgoingLink{TargetID: "theirs", BlockID: blockIdAt(objectCreatedAt, 2)},
		)
		fx.detailsService.EXPECT().SetCreatedInContextInternal("mine", "parent", mineBlockId).Return(nil).Once()
		fx.expectMigrationDone()

		// when
		err := fx.runObjectContextMigration(context.Background(), testSpaceId, testWorkspaceId)

		// then
		require.NoError(t, err)
	})

	t.Run("admin migrates objects of other members", func(t *testing.T) {
		// given
		fx := newFixture(t, model.ParticipantPermissions_Admin)
		fx.addPage(t, "parent", objectCreatedAt-1000, otherParticipantId)
		fx.addPage(t, "theirs", objectCreatedAt, otherParticipantId)
		blockId := blockIdAt(objectCreatedAt, 1)
		fx.addLinks(t, "parent", spaceindex.OutgoingLink{TargetID: "theirs", BlockID: blockId})
		fx.detailsService.EXPECT().SetCreatedInContextInternal("theirs", "parent", blockId).Return(nil).Once()
		fx.expectMigrationDone()

		// when
		err := fx.runObjectContextMigration(context.Background(), testSpaceId, testWorkspaceId)

		// then
		require.NoError(t, err)
	})

	t.Run("imported objects and objects created after the cutoff are skipped", func(t *testing.T) {
		// given
		fx := newFixture(t, model.ParticipantPermissions_Owner)
		fx.addPage(t, "parent", objectCreatedAt-1000, myParticipantId)
		fx.addPage(t, "new", objectContextCutoff+1000, myParticipantId)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("imported"),
				bundle.RelationKeySpaceId:        domain.String(testSpaceId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyCreatedDate:    domain.Int64(objectCreatedAt),
				bundle.RelationKeyOrigin:         domain.Int64(int64(model.ObjectOrigin_import)),
			},
		})
		fx.addLinks(t, "parent",
			spaceindex.OutgoingLink{TargetID: "new", BlockID: blockIdAt(objectContextCutoff+1000, 1)},
			spaceindex.OutgoingLink{TargetID: "imported", BlockID: blockIdAt(objectCreatedAt, 2)},
		)
		fx.expectMigrationDone()

		// when
		err := fx.runObjectContextMigration(context.Background(), testSpaceId, testWorkspaceId)

		// then
		require.NoError(t, err)
		fx.detailsService.AssertNotCalled(t, "SetCreatedInContextInternal")
	})

	t.Run("a failed write does not stop the migration", func(t *testing.T) {
		// given
		fx := newFixture(t, model.ParticipantPermissions_Owner)
		fx.addPage(t, "parent", objectCreatedAt-1000, myParticipantId)
		fx.addPage(t, "child1", objectCreatedAt, myParticipantId)
		fx.addPage(t, "child2", objectCreatedAt, myParticipantId)
		fx.addPage(t, "child3", objectCreatedAt, myParticipantId)
		block1, block2, block3 := blockIdAt(objectCreatedAt, 1), blockIdAt(objectCreatedAt, 2), blockIdAt(objectCreatedAt, 3)
		fx.addLinks(t, "parent",
			spaceindex.OutgoingLink{TargetID: "child1", BlockID: block1},
			spaceindex.OutgoingLink{TargetID: "child2", BlockID: block2},
			spaceindex.OutgoingLink{TargetID: "child3", BlockID: block3},
		)
		fx.detailsService.EXPECT().SetCreatedInContextInternal("child1", "parent", block1).Return(errors.New("load object")).Once()
		fx.detailsService.EXPECT().SetCreatedInContextInternal("child2", "parent", block2).Return(detailservice.ErrCreatedInContextAlreadySet).Once()
		fx.detailsService.EXPECT().SetCreatedInContextInternal("child3", "parent", block3).Return(nil).Once()
		fx.expectMigrationDone()

		// when
		err := fx.runObjectContextMigration(context.Background(), testSpaceId, testWorkspaceId)

		// then
		require.NoError(t, err)
	})
}

func TestCanMigrateAllObjects(t *testing.T) {
	for _, tc := range []struct {
		permissions model.ParticipantPermissions
		want        bool
	}{
		{permissions: model.ParticipantPermissions_Owner, want: true},
		{permissions: model.ParticipantPermissions_Admin, want: true},
		{permissions: model.ParticipantPermissions_Writer, want: false},
		{permissions: model.ParticipantPermissions_Reader, want: false},
	} {
		t.Run(tc.permissions.String(), func(t *testing.T) {
			// given
			fx := newFixture(t, tc.permissions)

			// when
			got := fx.canMigrateAllObjects(fx.objectStore.SpaceIndex(testSpaceId), myParticipantId)

			// then
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("participant that is not indexed", func(t *testing.T) {
		// given
		fx := newFixture(t, model.ParticipantPermissions_Owner)

		// when
		got := fx.canMigrateAllObjects(fx.objectStore.SpaceIndex(testSpaceId), otherParticipantId)

		// then
		assert.False(t, got)
	})
}
