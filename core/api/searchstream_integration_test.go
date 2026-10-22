package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// searchStreamOverEngine is the v2 search stream end to end below HTTP: the
// v2 service compiles the request, the adapter subscribes the real engine,
// and both read the same store. It is what proves the plan POST search
// compiles (the base row scope included) is a filter the live engine runs.
type searchStreamOverEngine struct {
	*objectSearchFixture
	svc *v2service.Service
}

func newSearchStreamOverEngine(t *testing.T) *searchStreamOverEngine {
	fx := newObjectSearchFixture(t)
	fx.store.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("spaceView_" + searchSpaceId),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeyTargetSpaceId:  domain.String(searchSpaceId),
	}})
	fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("type-page"),
			bundle.RelationKeyName:           domain.String("Page"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-page"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		},
		{
			bundle.RelationKeyId:             domain.String("type-template"),
			bundle.RelationKeyName:           domain.String("Template"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-template"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		},
	})
	svc := v2service.NewService(nil, nil, nil, nil, nil, nil, nil, fx.objectSearchAdapter, nil, fx.store, objectstore.TestTechSpaceId, "")
	return &searchStreamOverEngine{objectSearchFixture: fx, svc: svc}
}

func discussionParent(id string, mentions int64, discussionId string) objectstore.TestObject {
	obj := givenSearchObject(id, "Page "+id, mentions)
	if discussionId != "" {
		obj[bundle.RelationKeyDiscussionId] = domain.String(discussionId)
	}
	return obj
}

func decodeSearchStreamFrames(t *testing.T, frames []v2service.StreamFrame) []v2model.SearchStreamEvent {
	t.Helper()
	out := make([]v2model.SearchStreamEvent, 0, len(frames))
	for _, frame := range frames {
		var event v2model.SearchStreamEvent
		require.NoError(t, json.Unmarshal(frame.Data, &event))
		require.Equal(t, frame.Type, event.Type)
		out = append(out, event)
	}
	return out
}

func waitSearchStreamEvents(t *testing.T, stream *v2service.SearchStream, n int) []v2model.SearchStreamEvent {
	t.Helper()
	deadline := time.After(3 * time.Second)
	var got []v2model.SearchStreamEvent
	for len(got) < n {
		select {
		case <-stream.Ready():
			got = append(got, decodeSearchStreamFrames(t, stream.Drain())...)
		case <-deadline:
			t.Fatalf("got %d of %d events: %+v", len(got), n, got)
		}
	}
	return got
}

func TestSearchStreamOverTheEngine(t *testing.T) {
	ctx := context.Background()
	mentions := v2model.SearchRequest{Filter: "unread_mention_count > 0", Fields: []string{"unread_mention_count", "discussion"}}
	row := func(id string, mentions float64, discussionId string) *v2model.ObjectRow {
		return &v2model.ObjectRow{Id: id, Name: "Page " + id, Type: "page", Discussion: discussionId,
			Properties: map[string]any{"unread_mention_count": mentions}}
	}

	t.Run("the mentions recipe snapshots, follows and drops objects through the real engine", func(t *testing.T) {
		// given: a template and a hidden object carry mentions too, and the
		// search's base scope leaves both out
		fx := newSearchStreamOverEngine(t)
		template := discussionParent("tpl", 5, "disc-tpl")
		template[bundle.RelationKeyType] = domain.String("type-template")
		hidden := discussionParent("hidden", 5, "disc-hidden")
		hidden[bundle.RelationKeyIsHidden] = domain.Bool(true)
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{
			discussionParent("p1", 2, "disc1"),
			discussionParent("p2", 0, "disc2"),
			template,
			hidden,
		})
		wantSnapshot := []v2model.SearchStreamEvent{
			{Type: "object_added", SpaceId: searchSpaceId, Object: row("p1", 2, "disc1")},
			{Type: "snapshot_complete", SpaceId: searchSpaceId},
		}
		wantLive := []v2model.SearchStreamEvent{
			{Type: "object_added", SpaceId: searchSpaceId, Object: row("p2", 1, "disc2")},
			{Type: "object_updated", SpaceId: searchSpaceId, Object: row("p2", 1, "disc2b")},
			{Type: "object_removed", SpaceId: searchSpaceId, ObjectId: "p1"},
		}

		// when
		stream, err := fx.svc.OpenSearchStream(ctx, searchSpaceId, mentions)
		require.NoError(t, err)
		defer stream.Close()
		snapshot := decodeSearchStreamFrames(t, stream.Snapshot())
		// p2 is mentioned, its discussion is replaced, then p1's mention is read
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{discussionParent("p2", 1, "disc2")})
		live := waitSearchStreamEvents(t, stream, 1)
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{discussionParent("p2", 1, "disc2b")})
		live = append(live, waitSearchStreamEvents(t, stream, 1)...)
		fx.store.AddObjects(t, searchSpaceId, []objectstore.TestObject{discussionParent("p1", 0, "disc1")})
		live = append(live, waitSearchStreamEvents(t, stream, 1)...)

		// then
		assert.Equal(t, wantSnapshot, snapshot)
		assert.Equal(t, wantLive, live)
	})

	t.Run("closing the stream leaves no subscription behind", func(t *testing.T) {
		// given
		fx := newSearchStreamOverEngine(t)
		stream, err := fx.svc.OpenSearchStream(ctx, searchSpaceId, mentions)
		require.NoError(t, err)
		require.Len(t, fx.ourSubscriptions(), 1)

		// when
		stream.Close()

		// then
		assert.Empty(t, fx.ourSubscriptions())
	})
}
