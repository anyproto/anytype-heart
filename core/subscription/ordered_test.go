package subscription

import (
	"context"
	"fmt"
	"math/rand"
	"slices"
	"testing"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ordered subscriptions tested through the internal queue (sorts force the
// ordered path regardless of Internal) so exact event sequences are
// assertable without broadcast demultiplexing

func givenOrderedRequest(limit, offset int64) SubscribeRequest {
	return SubscribeRequest{
		SpaceId:           testSpaceId,
		SubId:             "ordered-sub",
		Internal:          true,
		NoDepSubscription: true,
		Limit:             limit,
		Offset:            offset,
		Keys:              []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String()},
		Sorts: []database.SortRequest{
			{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc},
		},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_participant)),
			},
		},
	}
}

func givenNamedParticipant(id, name string) objectstore.TestObject {
	obj := givenParticipant(id)
	obj[bundle.RelationKeyName] = domain.String(name)
	return obj
}

func orderedSetEvent(id, name string) *pb.EventMessage {
	return event.NewMessage(testSpaceId, &pb.EventMessageValueOfObjectDetailsSet{
		ObjectDetailsSet: &pb.EventObjectDetailsSet{
			Id: id,
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String(id),
				bundle.RelationKeyName: domain.String(name),
			}).ToProto(),
			SubIds: []string{"ordered-sub"},
		},
	})
}

func orderedAddEvent(id, afterId string) *pb.EventMessage {
	return event.NewMessage(testSpaceId, &pb.EventMessageValueOfSubscriptionAdd{
		SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: id, AfterId: afterId, SubId: "ordered-sub"},
	})
}

func orderedRemoveEvent(id string) *pb.EventMessage {
	return event.NewMessage(testSpaceId, &pb.EventMessageValueOfSubscriptionRemove{
		SubscriptionRemove: &pb.EventObjectSubscriptionRemove{Id: id, SubId: "ordered-sub"},
	})
}

func orderedPositionEvent(id, afterId string) *pb.EventMessage {
	return event.NewMessage(testSpaceId, &pb.EventMessageValueOfSubscriptionPosition{
		SubscriptionPosition: &pb.EventObjectSubscriptionPosition{Id: id, AfterId: afterId, SubId: "ordered-sub"},
	})
}

func orderedCountersEvent(total int64) *pb.EventMessage {
	return event.NewMessage(testSpaceId, &pb.EventMessageValueOfSubscriptionCounters{
		SubscriptionCounters: &pb.EventObjectSubscriptionCounters{Total: total, SubId: "ordered-sub"},
	})
}

func recordIds(records []*domain.Details) []string {
	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.GetString(bundle.RelationKeyId))
	}
	return ids
}

func TestOrderedSnapshot(t *testing.T) {
	t.Run("records sorted server-side, windowed, accurate total", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "charlie"),
			givenNamedParticipant("p2", "alice"),
			givenNamedParticipant("p3", "bob"),
		})

		resp, err := fx.Search(givenOrderedRequest(2, 0))
		require.NoError(t, err)

		assert.Equal(t, []string{"p2", "p3"}, recordIds(resp.Records))
		assert.Equal(t, int64(3), resp.Counters.Total)
	})

	t.Run("offset window", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "charlie"),
			givenNamedParticipant("p2", "alice"),
			givenNamedParticipant("p3", "bob"),
			givenNamedParticipant("p4", "dave"),
		})

		resp, err := fx.Search(givenOrderedRequest(2, 1))
		require.NoError(t, err)

		assert.Equal(t, []string{"p3", "p1"}, recordIds(resp.Records))
		assert.Equal(t, int64(4), resp.Counters.Total)
	})

	t.Run("limit 0 returns everything sorted", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "charlie"),
			givenNamedParticipant("p2", "alice"),
		})

		resp, err := fx.Search(givenOrderedRequest(0, 0))
		require.NoError(t, err)
		assert.Equal(t, []string{"p2", "p1"}, recordIds(resp.Records))
	})
}

func TestOrderedWindowEvents(t *testing.T) {
	t.Run("entering the window middle evicts the last entry", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "alice"),
			givenNamedParticipant("p2", "cara"),
			givenNamedParticipant("p3", "erin"),
		})
		resp, err := fx.Search(givenOrderedRequest(2, 0))
		require.NoError(t, err)
		require.Equal(t, []string{"p1", "p2"}, recordIds(resp.Records))

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p4", "bob"),
		})

		want := []*pb.EventMessage{
			orderedRemoveEvent("p2"),
			orderedSetEvent("p4", "bob"),
			orderedAddEvent("p4", "p1"),
			orderedCountersEvent(4),
		}
		assert.Equal(t, want, waitMessages(t, resp.Output, 4))
	})

	t.Run("entering beyond the window changes only the total", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "alice"),
			givenNamedParticipant("p2", "bob"),
		})
		resp, err := fx.Search(givenOrderedRequest(2, 0))
		require.NoError(t, err)

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p3", "zoe"),
		})

		want := []*pb.EventMessage{orderedCountersEvent(3)}
		assert.Equal(t, want, waitMessages(t, resp.Output, 1))
	})

	t.Run("leaving the window pulls in the successor", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "alice"),
			givenNamedParticipant("p2", "bob"),
			givenNamedParticipant("p3", "cara"),
		})
		resp, err := fx.Search(givenOrderedRequest(2, 0))
		require.NoError(t, err)
		require.Equal(t, []string{"p1", "p2"}, recordIds(resp.Records))

		// p1 stops matching: p3 slides into the window
		obj := givenNamedParticipant("p1", "alice")
		obj[bundle.RelationKeyResolvedLayout] = domain.Int64(int64(model.ObjectType_basic))
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

		want := []*pb.EventMessage{
			orderedRemoveEvent("p1"),
			orderedSetEvent("p3", "cara"),
			orderedAddEvent("p3", "p2"),
			orderedCountersEvent(2),
		}
		assert.Equal(t, want, waitMessages(t, resp.Output, 4))
	})

	t.Run("sort key change reorders within the window", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "alice"),
			givenNamedParticipant("p2", "bob"),
			givenNamedParticipant("p3", "cara"),
		})
		resp, err := fx.Search(givenOrderedRequest(0, 0))
		require.NoError(t, err)
		require.Equal(t, []string{"p1", "p2", "p3"}, recordIds(resp.Records))

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p3", "abe"),
		})

		want := []*pb.EventMessage{
			orderedPositionEvent("p3", ""),
			event.NewMessage(testSpaceId, &pb.EventMessageValueOfObjectDetailsAmend{
				ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
					Id: "p3",
					Details: []*pb.EventObjectDetailsAmendKeyValue{
						{Key: bundle.RelationKeyName.String(), Value: domain.String("abe").ToProto()},
					},
					SubIds: []string{"ordered-sub"},
				},
			}),
		}
		assert.Equal(t, want, waitMessages(t, resp.Output, 2))
	})

	t.Run("non-sort key change emits no ordering events", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "alice"),
		})
		req := givenOrderedRequest(0, 0)
		req.Keys = append(req.Keys, bundle.RelationKeyDescription.String())
		resp, err := fx.Search(req)
		require.NoError(t, err)

		obj := givenNamedParticipant("p1", "alice")
		obj[bundle.RelationKeyDescription] = domain.String("hello")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

		msgs := waitMessages(t, resp.Output, 1)
		require.Len(t, msgs, 1)
		require.NotNil(t, msgs[0].GetObjectDetailsAmend())
	})

	t.Run("entering before an offset window shifts it", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "bob"),
			givenNamedParticipant("p2", "cara"),
			givenNamedParticipant("p3", "dave"),
		})
		resp, err := fx.Search(givenOrderedRequest(2, 1))
		require.NoError(t, err)
		require.Equal(t, []string{"p2", "p3"}, recordIds(resp.Records))

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p4", "alice"),
		})

		// new order: alice, bob, cara, dave → window(offset 1, limit 2) = [bob, cara]
		want := []*pb.EventMessage{
			orderedRemoveEvent("p3"),
			orderedSetEvent("p1", "bob"),
			orderedAddEvent("p1", ""),
			orderedCountersEvent(4),
		}
		assert.Equal(t, want, waitMessages(t, resp.Output, 4))
	})
}

// TestOrderedWindowReplay drives a windowed sorted subscription through a
// deterministic random workload and replays every emitted event on a
// simulated client list using the dispatcher's application rules. After each
// step the simulated list must converge to the independently computed window
// — this pins the hard invariant that every afterId refers to an id already
// present in the client's list.
func TestOrderedWindowReplay(t *testing.T) {
	const (
		objectCount = 30
		steps       = 80
		limit       = 5
	)
	fx := newEngineFixture(t)
	rnd := rand.New(rand.NewSource(42))

	type objState struct {
		name    string
		matches bool
	}
	world := map[string]*objState{}

	writeObj := func(id string) {
		st := world[id]
		obj := objectstore.TestObject{
			bundle.RelationKeyId:   domain.String(id),
			bundle.RelationKeyName: domain.String(st.name),
		}
		if st.matches {
			obj[bundle.RelationKeyResolvedLayout] = domain.Int64(int64(model.ObjectType_participant))
		} else {
			obj[bundle.RelationKeyResolvedLayout] = domain.Int64(int64(model.ObjectType_basic))
		}
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})
	}

	expectedWindow := func() []string {
		type pair struct{ id, name string }
		var matching []pair
		for id, st := range world {
			if st.matches {
				matching = append(matching, pair{id, st.name})
			}
		}
		slices.SortFunc(matching, func(a, b pair) int {
			if a.name != b.name {
				if a.name < b.name {
					return -1
				}
				return 1
			}
			if a.id < b.id {
				return -1
			}
			return 1
		})
		if len(matching) > limit {
			matching = matching[:limit]
		}
		ids := make([]string, 0, len(matching))
		for _, p := range matching {
			ids = append(ids, p.id)
		}
		return ids
	}

	for i := 0; i < objectCount; i++ {
		id := fmt.Sprintf("obj%02d", i)
		world[id] = &objState{name: randomName(rnd), matches: true}
		writeObj(id)
	}

	resp, err := fx.Search(givenOrderedRequest(limit, 0))
	require.NoError(t, err)

	client := newClientList(recordIds(resp.Records))
	require.Equal(t, expectedWindow(), client.list)

	for step := 0; step < steps; step++ {
		id := fmt.Sprintf("obj%02d", rnd.Intn(objectCount))
		st := world[id]
		switch rnd.Intn(3) {
		case 0:
			st.name = randomName(rnd)
		case 1:
			st.matches = !st.matches
		case 2:
			st.name = randomName(rnd)
			st.matches = true
		}
		writeObj(id)

		expected := expectedWindow()
		client.waitConverge(t, resp.Output, expected, "step %d (%s)", step, id)
	}
}

func randomName(rnd *rand.Rand) string {
	return fmt.Sprintf("n%03dx%c", rnd.Intn(1000), 'a'+rune(rnd.Intn(26)))
}

// clientList replays subscription events with the anytype-ts dispatcher's
// application rules (subscriptionPosition logic)
type clientList struct {
	list []string
}

func newClientList(ids []string) *clientList {
	return &clientList{list: slices.Clone(ids)}
}

func (c *clientList) apply(t *testing.T, msg *pb.EventMessage) {
	switch v := msg.Value.(type) {
	case *pb.EventMessageValueOfSubscriptionAdd:
		c.position(t, v.SubscriptionAdd.Id, v.SubscriptionAdd.AfterId, true)
	case *pb.EventMessageValueOfSubscriptionPosition:
		c.position(t, v.SubscriptionPosition.Id, v.SubscriptionPosition.AfterId, false)
	case *pb.EventMessageValueOfSubscriptionRemove:
		c.list = slices.DeleteFunc(c.list, func(id string) bool { return id == v.SubscriptionRemove.Id })
	}
}

func (c *clientList) position(t *testing.T, id, afterId string, isAdding bool) {
	if afterId != "" {
		// the hard invariant: afterId must already be in the client list
		require.Contains(t, c.list, afterId, "afterId %q not in client list %v (event for %q)", afterId, c.list, id)
	}
	newIndex := slices.Index(c.list, afterId)
	oldIndex := slices.Index(c.list, id)
	if isAdding && oldIndex >= 0 {
		return
	}
	if afterId == "" {
		newIndex = 0
	} else if newIndex >= 0 && newIndex < oldIndex {
		newIndex++
	}
	if oldIndex < 0 {
		insertAt := 0
		if afterId != "" {
			insertAt = newIndex + 1
		}
		c.list = slices.Insert(c.list, insertAt, id)
	} else if oldIndex != newIndex {
		c.list = slices.Delete(c.list, oldIndex, oldIndex+1)
		c.list = slices.Insert(c.list, newIndex, id)
	}
}

func (c *clientList) waitConverge(t *testing.T, queue *mb.MB[*pb.EventMessage], expected []string, msgFmt string, args ...any) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !slices.Equal(c.list, expected) {
		remaining := time.Until(deadline)
		require.Positivef(t, remaining, "client list %v never converged to %v: "+msgFmt, append([]any{c.list, expected}, args...)...)
		ctx, cancel := context.WithTimeout(context.Background(), remaining)
		msg, err := queue.WaitOne(ctx)
		cancel()
		require.NoErrorf(t, err, "waiting events: client %v, expected %v: "+msgFmt, append([]any{c.list, expected}, args...)...)
		c.apply(t, msg)
	}
}
