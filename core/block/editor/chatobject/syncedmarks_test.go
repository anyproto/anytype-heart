package chatobject

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// headsTree is a stubTree with heads, whose changes carry order ids as a loaded tree's do.
type headsTree struct {
	stubTree
	heads   []string
	changes map[string]*objecttree.Change
}

func (t *headsTree) Heads() []string {
	return t.heads
}

func (t *headsTree) GetChange(id string) (*objecttree.Change, error) {
	if ch, ok := t.changes[id]; ok {
		return ch, nil
	}
	return nil, fmt.Errorf("change not found")
}

const headId = "headChangeId"

func (fx *fixture) setHeadAt(orderId string) {
	fx.sb.SetTree(&headsTree{
		heads:   []string{headId},
		changes: map[string]*objecttree.Change{headId: {Id: headId, OrderId: orderId}},
	})
}

type storedMessage struct {
	id      string
	creator string
	synced  bool
}

// withStoredMessages puts messages in the store before the object loads, as a replay leaves
// them, with the tree head at the order id of the message headAt.
func withStoredMessages(messages []storedMessage, headAt string) fixtureOption {
	return func(fx *fixture) {
		repo, err := fx.repositoryService.Repository(testSpaceId, chatId)
		if err != nil {
			panic(err)
		}
		var orderId, headOrderId string
		for _, msg := range messages {
			orderId = storestate.LexId.Next(orderId)
			if msg.id == headAt {
				headOrderId = orderId
			}
			err = repo.AddTestMessage(context.Background(), &chatmodel.Message{
				ChatMessage: &model.ChatMessage{
					Id:      msg.id,
					OrderId: orderId,
					Creator: msg.creator,
					Message: &model.ChatMessageMessageContent{Text: msg.id},
					Synced:  msg.synced,
				},
			})
			if err != nil {
				panic(err)
			}
		}
		fx.setHeadAt(headOrderId)
	}
}

// withSyncStatus sets the sync status the object store holds for the chat, which the load
// injects into the local details.
func withSyncStatus(status domain.ObjectSyncStatus) fixtureOption {
	return func(fx *fixture) {
		fx.sb.Doc.(*state.State).SetLocalDetail(bundle.RelationKeySyncStatus, domain.Int64(int64(status)))
	}
}

func syncedFlags(t *testing.T, fx *fixture, ids ...string) map[string]bool {
	t.Helper()
	got := make(map[string]bool, len(ids))
	for _, id := range ids {
		got[id] = fx.getMessage(t, id).Synced
	}
	return got
}

// withLocalOnly loads the chat in an account that runs without the sync node.
func withLocalOnly() fixtureOption {
	return func(fx *fixture) {
		fx.localOnly = true
	}
}

// loadedSyncStatus is the sync status the load left in its state, "" when there is none, and
// whether the load wrote it.
func loadedSyncStatus(fx *fixture) (status string, written bool) {
	details := fx.initState.LocalDetails()
	written = details.Has(bundle.RelationKeySyncDate)
	if !details.Has(bundle.RelationKeySyncStatus) {
		return "", written
	}
	return model.SyncStatus(details.GetInt64(bundle.RelationKeySyncStatus)).String(), written
}

func TestLoadRechecksSyncOfStaleSyncedMarks(t *testing.T) {
	const (
		other   = "otherAccountId"
		synced  = "SyncStatusSynced"
		syncing = "SyncStatusSyncing"
		failed  = "SyncStatusError"
	)
	replayed := []storedMessage{
		{id: "own1", creator: testCreator},
		{id: "other1", creator: other, synced: true},
		{id: "own2", creator: testCreator},
	}
	healthy := []storedMessage{
		{id: "own1", creator: testCreator, synced: true},
		{id: "own2", creator: testCreator, synced: true},
		{id: "own3", creator: testCreator},
	}

	for _, tc := range []struct {
		name        string
		stored      []storedMessage
		headAt      string
		opts        []fixtureOption
		wantStatus  string
		wantWritten bool
	}{
		{
			name:        "a synced chat whose own messages a replay left unsynced goes back to syncing",
			stored:      replayed,
			headAt:      "own2",
			opts:        []fixtureOption{withSyncStatus(domain.ObjectSyncStatusSynced)},
			wantStatus:  syncing,
			wantWritten: true,
		},
		{
			name: "a synced chat whose oldest own message is unsynced goes back to syncing",
			stored: []storedMessage{
				{id: "own1", creator: testCreator},
				{id: "own2", creator: testCreator, synced: true},
			},
			headAt:      "own2",
			opts:        []fixtureOption{withSyncStatus(domain.ObjectSyncStatusSynced)},
			wantStatus:  syncing,
			wantWritten: true,
		},
		{
			name: "a synced chat whose newest own message is unsynced goes back to syncing",
			stored: []storedMessage{
				{id: "own1", creator: testCreator, synced: true},
				{id: "own2", creator: testCreator},
			},
			headAt:      "own2",
			opts:        []fixtureOption{withSyncStatus(domain.ObjectSyncStatusSynced)},
			wantStatus:  syncing,
			wantWritten: true,
		},
		{
			name:       "a synced chat with its own messages synced up to the head stays synced",
			stored:     healthy,
			headAt:     "own2",
			opts:       []fixtureOption{withSyncStatus(domain.ObjectSyncStatusSynced)},
			wantStatus: synced,
		},
		{
			name:       "a synced chat without own messages stays synced",
			stored:     []storedMessage{{id: "other1", creator: other, synced: true}},
			headAt:     "other1",
			opts:       []fixtureOption{withSyncStatus(domain.ObjectSyncStatusSynced)},
			wantStatus: synced,
		},
		{
			name:        "a chat without a sync status yet whose own messages are unsynced goes to syncing",
			stored:      replayed,
			headAt:      "own2",
			wantStatus:  syncing,
			wantWritten: true,
		},
		{
			name:   "a chat without a sync status yet and with its own messages synced gets none",
			stored: healthy,
			headAt: "own2",
		},
		{
			name:       "a syncing chat waits for its move to synced",
			stored:     replayed,
			headAt:     "own2",
			opts:       []fixtureOption{withSyncStatus(domain.ObjectSyncStatusSyncing)},
			wantStatus: syncing,
		},
		{
			name:       "a chat with a sync error is left alone",
			stored:     replayed,
			headAt:     "own2",
			opts:       []fixtureOption{withSyncStatus(domain.ObjectSyncStatusError)},
			wantStatus: failed,
		},
		{
			name:       "a local-only account leaves a synced chat alone",
			stored:     replayed,
			headAt:     "own2",
			opts:       []fixtureOption{withSyncStatus(domain.ObjectSyncStatusSynced), withLocalOnly()},
			wantStatus: synced,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			flagsBefore := make(map[string]bool, len(tc.stored))
			ids := make([]string, 0, len(tc.stored))
			for _, msg := range tc.stored {
				flagsBefore[msg.id] = msg.synced
				ids = append(ids, msg.id)
			}
			opts := append([]fixtureOption{withStoredMessages(tc.stored, tc.headAt)}, tc.opts...)

			// when
			fx := newFixture(t, opts...)

			// then
			status, written := loadedSyncStatus(fx)
			assert.Equal(t, tc.wantStatus, status)
			assert.Equal(t, tc.wantWritten, written, "whether the load wrote a sync status")
			assert.Equal(t, flagsBefore, syncedFlags(t, fx, ids...), "the load writes no synced flag")
		})
	}
}

func TestHandleSyncStatusUpdate(t *testing.T) {
	addOwnMessages := func(t *testing.T, fx *fixture, n int) []string {
		t.Helper()
		ids := make([]string, 0, n)
		for i := range n {
			id, err := fx.AddMessage(context.Background(), nil, givenSimpleMessage(fmt.Sprintf("message %d", i+1)))
			require.NoError(t, err)
			ids = append(ids, id)
		}
		return ids
	}

	t.Run("synced marks the messages up to the heads and tells the subscription", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ids := addOwnMessages(t, fx, 3)
		fx.setHeadAt(fx.appliedChangeSets[1].Order)
		subscribeForEvents(t, fx)
		fx.events = nil

		// when
		fx.locked(t, func() error {
			fx.HandleSyncStatusUpdate([]string{headId}, domain.ObjectSyncStatusSynced, domain.SyncErrorNull)
			return nil
		})

		// then
		assert.Equal(t, map[string]bool{ids[0]: true, ids[1]: true, ids[2]: false}, syncedFlags(t, fx, ids...))
		var syncedEvents []string
		for _, ev := range fx.events {
			if upd := ev.GetChatUpdateMessageSyncStatus(); upd != nil && upd.IsSynced {
				syncedEvents = append(syncedEvents, upd.Ids...)
			}
		}
		assert.ElementsMatch(t, ids[:2], syncedEvents)
	})

	t.Run("syncing changes nothing", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ids := addOwnMessages(t, fx, 2)
		fx.setHeadAt(fx.appliedChangeSets[1].Order)

		// when
		fx.locked(t, func() error {
			fx.HandleSyncStatusUpdate([]string{headId}, domain.ObjectSyncStatusSyncing, domain.SyncErrorNull)
			return nil
		})

		// then
		assert.Equal(t, map[string]bool{ids[0]: false, ids[1]: false}, syncedFlags(t, fx, ids...))
	})
}
