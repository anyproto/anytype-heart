package v2handler

// The queries/collections read handlers' WIRING is what these tests pin: the
// ?view= and ?fields= query params must actually reach the service, and the
// service's warning-grade issues must actually reach the JSON response —
// replacing listFieldsParam(c)/c.Query("view") with nil/"" or dropping
// resp.Warnings would otherwise leave every suite green.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// listReadRouter mounts the four list-read routes with C10 pagination.
func listReadRouter(fx *v2HandlerFixture) {
	fx.router.Use(pagination.New(pagination.Config{
		DefaultPage:     0,
		DefaultPageSize: 25,
		MinPageSize:     1,
		MaxPageSize:     1000,
	}))
	fx.router.GET("/v2/spaces/:space_id/queries/:query_id/objects", GetQueryObjectsHandler(fx.svc))
	fx.router.GET("/v2/spaces/:space_id/queries/:query_id/views", GetQueryViewsHandler(fx.svc))
	fx.router.GET("/v2/spaces/:space_id/collections/:collection_id/objects", GetCollectionObjectsHandler(fx.svc))
	fx.router.GET("/v2/spaces/:space_id/collections/:collection_id/views", GetCollectionViewsHandler(fx.svc))
}

// handlerQueryRead builds a live query read: layout set (the internal name
// the Query type keeps), setOf type-chore, and an optional dataview block
// under the canonical "dataview" id.
func handlerQueryRead(dv *model.BlockContentDataview) apicore.ObjectRead {
	snapshot := &model.SmartBlockSnapshotBase{
		Details: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyResolvedLayout.String(): pbtypes.Int64(int64(model.ObjectType_set)),
			bundle.RelationKeySetOf.String():          pbtypes.StringList([]string{"type-chore"}),
		}},
	}
	if dv != nil {
		snapshot.Blocks = []*model.Block{{
			Id:      "dataview",
			Content: &model.BlockContentOfDataview{Dataview: dv},
		}}
	}
	return apicore.ObjectRead{Snapshot: snapshot, Heads: []string{"headL"}}
}

// addChoreType registers the type the query's setOf resolves to.
func (fx *v2HandlerFixture) addChoreType(t *testing.T) {
	fx.store.AddObjects(t, "space1", []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("type-chore"),
		bundle.RelationKeyName:           domain.String("Chore"),
		bundle.RelationKeyUniqueKey:      domain.String("ot-chore"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
	}})
}

func TestGetQueryObjectsHandler(t *testing.T) {
	t.Run("?view= reaches the service (an unknown view 404s)", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		listReadRouter(fx)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "query1").Return(handlerQueryRead(nil), nil)

		// when
		req := httptest.NewRequest(http.MethodGet, "/v2/spaces/space1/queries/query1/objects?view=ghost", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), `view \"ghost\" not found`)
	})

	t.Run("?fields= reaches the service (a typoed key 400s)", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		listReadRouter(fx)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "query1").Return(handlerQueryRead(nil), nil)

		// when
		req := httptest.NewRequest(http.MethodGet, "/v2/spaces/space1/queries/query1/objects?fields=bogus", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Len(t, got.Issues, 1)
		assert.Equal(t, "fields", got.Issues[0].Path)
		assert.Contains(t, got.Issues[0].Message, `unknown property key "bogus"`)
	})

	t.Run("placeholder warnings ride the response body", func(t *testing.T) {
		// given: a stored view whose filter carries an unresolvable
		// placeholder — the service drops the leaf and warns; the warning
		// must survive to the wire
		fx := newV2HandlerFixture(t)
		listReadRouter(fx)
		fx.addChoreType(t)
		dv := &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Id: "v1",
			Filters: []*model.BlockContentDataviewFilter{{
				RelationKey: bundle.RelationKeyCreator.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("_filter_template_9_"),
			}},
		}}}
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "query1").Return(handlerQueryRead(dv), nil)

		// when
		req := httptest.NewRequest(http.MethodGet, "/v2/spaces/space1/queries/query1/objects?view=v1", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		var got v2model.ListResponse[v2model.ObjectRow]
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, `"_filter_template_9_" is an unresolvable placeholder`)
	})
}

func TestGetCollectionObjectsHandler(t *testing.T) {
	collectionRead := func(dv *model.BlockContentDataview) apicore.ObjectRead {
		snapshot := &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyResolvedLayout.String(): pbtypes.Int64(int64(model.ObjectType_collection)),
			}},
		}
		if dv != nil {
			snapshot.Blocks = []*model.Block{{
				Id:      "dataview",
				Content: &model.BlockContentOfDataview{Dataview: dv},
			}}
		}
		return apicore.ObjectRead{Snapshot: snapshot, Heads: []string{"headL"}}
	}

	t.Run("?view= reaches the service on the collections route too", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		listReadRouter(fx)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "col1").Return(collectionRead(nil), nil)

		// when
		req := httptest.NewRequest(http.MethodGet, "/v2/spaces/space1/collections/col1/objects?view=ghost", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), `view \"ghost\" not found`)
	})

	t.Run("?fields= reaches the service on the collections route too", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		listReadRouter(fx)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "col1").Return(collectionRead(nil), nil)

		// when
		req := httptest.NewRequest(http.MethodGet, "/v2/spaces/space1/collections/col1/objects?fields=bogus", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), `unknown property key \"bogus\"`)
	})
}
