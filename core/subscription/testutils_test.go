package subscription

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func genOrder(t *testing.T) database.Order {
	return database.NewKeyOrder(objectstore.NewStoreFixture(t).SpaceIndex(spaceId), nil, nil, database.SortRequest{
		RelationKey: "order",
		Format:      model.RelationFormat_number,
	})
}

func genEntries(n int, backord bool) (res []*entry) {
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("id%d", i)
		ord := i
		if backord {
			ord = n - i
		}
		res = append(res, genEntry(id, int64(ord)))
	}
	rand.Shuffle(len(res), func(i, j int) { res[i], res[j] = res[j], res[i] })
	return
}

func genEntry(id string, ord int64) *entry {
	return newEntry(id, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"id":    domain.String(id),
		"order": domain.Int64(ord),
	}))
}

func assertCtxAdd(t *testing.T, ctx *opCtx, id, afterId string) {
	var found bool
	for _, add := range ctx.position {
		if add.isAdd && add.id == id {
			found = true
			assert.Equal(t, afterId, add.afterId, "add after id not equal")
			break
		}
	}
	assert.True(t, found, fmt.Sprintf("add id %v not found", id))
}

func assertCtxPosition(t *testing.T, ctx *opCtx, id, afterId string) {
	var found bool
	for _, pos := range ctx.position {
		if pos.id == id {
			found = true
			assert.Equal(t, afterId, pos.afterId, "pos after id not equal")
			break
		}
	}
	assert.True(t, found, fmt.Sprintf("pos id %v not found", id))
}

func assertCtxCounters(t *testing.T, ctx *opCtx, counter opCounter) {
	for _, c := range ctx.counters {
		assert.Equal(t, counter, c)
	}
}

func assertCtxRemove(t *testing.T, ctx *opCtx, ids ...string) {
	for _, id := range ids {
		var found bool
		for _, rem := range ctx.remove {
			if rem.id == id {
				found = true
				break
			}
		}
		assert.True(t, found, fmt.Sprintf("remove id %v not found", id))
	}
}

func assertCtxChange(t *testing.T, ctx *opCtx, ids ...string) {
	for _, id := range ids {
		var found bool
		for _, change := range ctx.change {
			if change.id == id {
				found = true
				break
			}
		}
		assert.True(t, found, fmt.Sprintf("change id %v not found", id))
	}
}

func assertCtxEmpty(t *testing.T, ctx *opCtx) {
	assert.Len(t, ctx.remove, 0, "remove not empty")
	assert.Len(t, ctx.counters, 0, "counters not empty")
	assert.Len(t, ctx.change, 0, "change not empty")
	assert.Len(t, ctx.position, 0, "position not empty")
}

func assertCtxGroup(t *testing.T, ctx *opCtx, added, removed int) {
	foundAdded := 0
	for _, g := range ctx.groups {
		if !g.remove {
			foundAdded++
		}
	}
	assert.Equal(t, foundAdded, added)

	foundRemoved := 0
	for _, g := range ctx.groups {
		if g.remove {
			foundRemoved++
		}
	}
	assert.Equal(t, foundRemoved, removed)
}

func applySubscriptionEvents(subId string, records []*domain.Details, events []*pb.EventMessage) (order map[string]int) {
	order = make(map[string]int, len(records))
	for i, record := range records {
		order[record.GetString(bundle.RelationKeyId)] = i
	}

	move := func(id, afterId string) {
		afterPos, found := order[afterId]
		if !found && afterId != "" {
			panic("afterId is not found for SubPos")
		}
		if !found {
			afterPos = -1
		}
		movablePos, found := order[id]
		if !found {
			panic("id is not found for SubPos")
		}
		moveUp := movablePos > afterPos
		for objId, pos := range order {
			if moveUp && afterPos < pos && pos < movablePos {
				order[objId] = pos + 1
				continue
			}
			if !moveUp && movablePos < pos && pos <= afterPos {
				order[objId] = pos - 1
			}
		}
		if moveUp {
			order[id] = afterPos + 1
		} else {
			order[id] = afterPos
		}
	}

	for _, event := range events {
		switch ev := event.Value.(type) {
		case *pb.EventMessageValueOfSubscriptionAdd:
			if ev.SubscriptionAdd.SubId != subId {
				continue
			}
			_, found := order[ev.SubscriptionAdd.Id]
			if !found {
				order[ev.SubscriptionAdd.Id] = len(order)
			}
			move(ev.SubscriptionAdd.Id, ev.SubscriptionAdd.AfterId)
		case *pb.EventMessageValueOfSubscriptionRemove:
			if ev.SubscriptionRemove.SubId != subId {
				continue
			}
			deletePos, found := order[ev.SubscriptionRemove.Id]
			if !found {
				panic("id not found for SubRemove")
			}
			delete(order, ev.SubscriptionRemove.Id)
			for id, pos := range order {
				if pos > deletePos {
					order[id] = pos - 1
				}
			}
		case *pb.EventMessageValueOfSubscriptionPosition:
			if ev.SubscriptionPosition.SubId != subId {
				continue
			}
			move(ev.SubscriptionPosition.Id, ev.SubscriptionPosition.AfterId)
		}
	}
	return order
}

func TestApplySubscriptionEvents(t *testing.T) {
	records := []*domain.Details{
		domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("task0")}),
		domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("task1")}),
		domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("task2")}),
		domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("task3")}),
		domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("task4")}),
	}

	for _, tc := range []struct {
		name     string
		events   []*pb.EventMessage
		expected []int
	}{
		{
			"single SubPos",
			[]*pb.EventMessage{
				{Value: &pb.EventMessageValueOfSubscriptionPosition{SubscriptionPosition: &pb.EventObjectSubscriptionPosition{Id: "task2", AfterId: "task3"}}},
			},
			[]int{0, 1, 3, 2, 4},
		},
		{
			"multiple SubPos",
			[]*pb.EventMessage{
				{Value: &pb.EventMessageValueOfSubscriptionPosition{SubscriptionPosition: &pb.EventObjectSubscriptionPosition{Id: "task4", AfterId: ""}}},
				{Value: &pb.EventMessageValueOfSubscriptionPosition{SubscriptionPosition: &pb.EventObjectSubscriptionPosition{Id: "task0", AfterId: "task2"}}},
				{Value: &pb.EventMessageValueOfSubscriptionPosition{SubscriptionPosition: &pb.EventObjectSubscriptionPosition{Id: "task1", AfterId: "task2"}}},
				{Value: &pb.EventMessageValueOfSubscriptionPosition{SubscriptionPosition: &pb.EventObjectSubscriptionPosition{Id: "task3", AfterId: "task1"}}},
			},
			[]int{4, 2, 1, 3, 0},
		},
		{
			"SubAdd and SubRemove",
			[]*pb.EventMessage{
				{Value: &pb.EventMessageValueOfSubscriptionAdd{SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: "task5", AfterId: ""}}},
				{Value: &pb.EventMessageValueOfSubscriptionAdd{SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: "task6", AfterId: "task3"}}},
				{Value: &pb.EventMessageValueOfSubscriptionAdd{SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: "task7", AfterId: "task5"}}},
				{Value: &pb.EventMessageValueOfSubscriptionRemove{SubscriptionRemove: &pb.EventObjectSubscriptionRemove{Id: "task0"}}},
				{Value: &pb.EventMessageValueOfSubscriptionRemove{SubscriptionRemove: &pb.EventObjectSubscriptionRemove{Id: "task5"}}},
				{Value: &pb.EventMessageValueOfSubscriptionRemove{SubscriptionRemove: &pb.EventObjectSubscriptionRemove{Id: "task2"}}},
			},
			[]int{7, 1, 3, 6, 4},
		},
		{
			"SubAdd, SubRemove and SubPosition",
			[]*pb.EventMessage{
				{Value: &pb.EventMessageValueOfSubscriptionAdd{SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: "task5", AfterId: ""}}},
				{Value: &pb.EventMessageValueOfSubscriptionAdd{SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: "task6", AfterId: "task3"}}},
				{Value: &pb.EventMessageValueOfSubscriptionPosition{SubscriptionPosition: &pb.EventObjectSubscriptionPosition{Id: "task4", AfterId: ""}}},
				{Value: &pb.EventMessageValueOfSubscriptionAdd{SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: "task7", AfterId: "task5"}}},
				{Value: &pb.EventMessageValueOfSubscriptionPosition{SubscriptionPosition: &pb.EventObjectSubscriptionPosition{Id: "task3", AfterId: "task1"}}},
				{Value: &pb.EventMessageValueOfSubscriptionRemove{SubscriptionRemove: &pb.EventObjectSubscriptionRemove{Id: "task0"}}},
				{Value: &pb.EventMessageValueOfSubscriptionPosition{SubscriptionPosition: &pb.EventObjectSubscriptionPosition{Id: "task1", AfterId: "task2"}}},
				{Value: &pb.EventMessageValueOfSubscriptionRemove{SubscriptionRemove: &pb.EventObjectSubscriptionRemove{Id: "task5"}}},
				{Value: &pb.EventMessageValueOfSubscriptionPosition{SubscriptionPosition: &pb.EventObjectSubscriptionPosition{Id: "task7", AfterId: "task2"}}},
				{Value: &pb.EventMessageValueOfSubscriptionRemove{SubscriptionRemove: &pb.EventObjectSubscriptionRemove{Id: "task2"}}},
			},
			[]int{4, 3, 7, 1, 6},
		},
		{
			"SubAdd existing object",
			[]*pb.EventMessage{
				{Value: &pb.EventMessageValueOfSubscriptionAdd{SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: "task0", AfterId: ""}}},
				{Value: &pb.EventMessageValueOfSubscriptionAdd{SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: "task3", AfterId: "task2"}}},
				{Value: &pb.EventMessageValueOfSubscriptionAdd{SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: "task1", AfterId: "task0"}}},
			},
			[]int{0, 1, 2, 3, 4},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := applySubscriptionEvents("", records, tc.events)
			for pos, objectNum := range tc.expected {
				assert.Equal(t, pos, order[fmt.Sprintf("task%d", objectNum)])
			}
		})
	}
}
