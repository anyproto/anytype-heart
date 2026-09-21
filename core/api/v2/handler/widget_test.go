package v2handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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
)

// withPagination mimics the group-level pagination middleware for handler
// tests: the C10 defaults land in the context keys the list handlers read.
func withPagination() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(pagination.QueryParamOffset, 0)
		c.Set(pagination.QueryParamLimit, 25)
		c.Next()
	}
}

func (fx *v2HandlerFixture) registerWidgetRoutes() {
	fx.router.GET("/v2/spaces/:space_id/widgets", withPagination(), ListWidgetsHandler(fx.svc))
	fx.router.POST("/v2/spaces/:space_id/widgets", withDryRunFlag(), CreateWidgetHandler(fx.svc))
	fx.router.PATCH("/v2/spaces/:space_id/widgets/:widget_id", withDryRunFlag(), UpdateWidgetHandler(fx.svc))
	fx.router.DELETE("/v2/spaces/:space_id/widgets/:widget_id", withDryRunFlag(), DeleteWidgetHandler(fx.svc))
}

func testWidgetEntry(id, scope, target string) apicore.WidgetEntry {
	return apicore.WidgetEntry{Id: id, LinkId: id + "-link", Scope: apicore.WidgetScope(scope), Target: target, Layout: model.BlockContentWidget_Tree, Limit: 6}
}

func TestWidgetHandlers(t *testing.T) {
	t.Run("list serves both roots with the listing spellings", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.registerWidgetRoutes()
		fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return([]apicore.WidgetEntry{testWidgetEntry("w1", "space", "bin")}, nil)
		fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, "space1", apicore.WidgetScopePersonal).Return([]apicore.WidgetEntry{testWidgetEntry("p1", "personal", "obj1")}, nil)

		// when
		req := httptest.NewRequest(http.MethodGet, "/v2/spaces/space1/widgets", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got v2model.ListResponse[v2model.WidgetRow]
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Len(t, got.Data, 2)
		assert.Equal(t, v2model.WidgetRow{Id: "w1", Scope: "space", Target: "_bin", Layout: "tree", Limit: 6}, got.Data[0])
		assert.Equal(t, "p1", got.Data[1].Id)
		assert.Equal(t, 2, got.Total)
	})

	t.Run("create answers 201 with the stored widget, 200 on a dry run", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.registerWidgetRoutes()
		fx.store.AddObjects(t, "space1", []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("page1"),
			bundle.RelationKeyName:           domain.String("Page"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})
		fx.widgetsMock.EXPECT().CanEditWidgets(mock.Anything, "space1", apicore.WidgetScopePersonal).Return(true, nil)
		fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, "space1", apicore.WidgetScopePersonal).Return(nil, nil)
		fx.widgetsMock.EXPECT().CreateWidget(mock.Anything, "space1", apicore.WidgetScopePersonal,
			apicore.WidgetCreate{Target: "page1", Layout: model.BlockContentWidget_Tree, Limit: 6}).
			Return(testWidgetEntry("p1", "personal", "page1"), nil).Once()

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/widgets", strings.NewReader(`{"target":"page1","scope":"personal"}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var got v2model.WidgetResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.WidgetRow{Id: "p1", Scope: "personal", Target: "page1", Layout: "tree", Limit: 6}, got.WidgetRow)
		assert.False(t, got.DryRun)

		// and the dry run commits nothing (CreateWidget is .Once() above)
		req = httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/widgets?dry_run=true", strings.NewReader(`{"target":"page1","scope":"personal","layout":"list"}`))
		w = httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.True(t, got.DryRun)
		assert.Equal(t, "tree", got.Layout, "a page cannot render list; the fallback is stored and reported")
		require.Len(t, got.Warnings, 1)
	})

	t.Run("a body with an unknown member is a 400 naming it", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		fx.registerWidgetRoutes()

		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/widgets", strings.NewReader(`{"target":"page1","scope":"personal","name":"x"}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Len(t, got.Issues, 1)
		assert.Equal(t, "/name", got.Issues[0].Path)
		require.Len(t, got.Issues[0].SeeAlso, 1)
		assert.Equal(t, v2model.OpGetSchema, got.Issues[0].SeeAlso[0].Op)
	})

	t.Run("update and delete pass the path and the scope query through", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.registerWidgetRoutes()
		fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return([]apicore.WidgetEntry{testWidgetEntry("w1", "space", "page1")}, nil)
		fx.widgetsMock.EXPECT().CanEditWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return(true, nil)
		fx.store.AddObjects(t, "space1", []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("page1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})
		limit := int32(10)
		tree := model.BlockContentWidget_Tree
		// the update is planned under the lock: the mock runs the plan against
		// the stored widget and the layout rides along with a limit-only patch
		fx.widgetsMock.EXPECT().UpdateWidget(mock.Anything, "space1", apicore.WidgetScopeSpace, "w1", mock.Anything).RunAndReturn(
			func(_ context.Context, _ string, _ apicore.WidgetScope, _ string, plan func(apicore.WidgetEntry) (apicore.WidgetUpdate, error)) (apicore.WidgetEntry, error) {
				update, err := plan(testWidgetEntry("w1", "space", "page1"))
				require.NoError(t, err)
				assert.Equal(t, apicore.WidgetUpdate{Layout: &tree, Limit: &limit}, update)
				return apicore.WidgetEntry{Id: "w1", Scope: apicore.WidgetScopeSpace, Target: "page1", Layout: model.BlockContentWidget_Tree, Limit: 10}, nil
			})
		fx.widgetsMock.EXPECT().DeleteWidget(mock.Anything, "space1", apicore.WidgetScopeSpace, "w1", "page1").Return(testWidgetEntry("w1", "space", "page1"), nil)

		// when: addressed by target, scoped by the query
		req := httptest.NewRequest(http.MethodPatch, "/v2/spaces/space1/widgets/page1?scope=space", strings.NewReader(`{"limit":10}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got v2model.WidgetResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, 10, got.Limit)

		req = httptest.NewRequest(http.MethodDelete, "/v2/spaces/space1/widgets/w1?scope=space", nil)
		w = httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.True(t, got.Removed)
		assert.Equal(t, "w1", got.Id)
	})

	t.Run("the wire shape: a flattened result, no limit on a link widget, the removed marker", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		fx.registerWidgetRoutes()
		linkWidget := apicore.WidgetEntry{Id: "w1", Scope: apicore.WidgetScopeSpace, Target: "favorite", Layout: model.BlockContentWidget_Link, Limit: 6}
		fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return([]apicore.WidgetEntry{linkWidget}, nil)
		fx.widgetsMock.EXPECT().CanEditWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return(true, nil)
		fx.widgetsMock.EXPECT().DeleteWidget(mock.Anything, "space1", apicore.WidgetScopeSpace, "w1", "favorite").Return(linkWidget, nil)

		req := httptest.NewRequest(http.MethodDelete, "/v2/spaces/space1/widgets/_favorite?scope=space", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.JSONEq(t, `{"id":"w1","scope":"space","target":"_favorite","layout":"link","removed":true}`, w.Body.String())
	})

	t.Run("dry-run PATCH and DELETE write nothing", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		fx.registerWidgetRoutes()
		fx.store.AddObjects(t, "space1", []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("page1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})
		fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return([]apicore.WidgetEntry{testWidgetEntry("w1", "space", "page1")}, nil)
		fx.widgetsMock.EXPECT().CanEditWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return(true, nil)
		// no UpdateWidget / DeleteWidget expectation: a call would fail the mock

		req := httptest.NewRequest(http.MethodPatch, "/v2/spaces/space1/widgets/w1?scope=space&dry_run=true", strings.NewReader(`{"limit":10}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got v2model.WidgetResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.True(t, got.DryRun)
		assert.Equal(t, 10, got.Limit)

		req = httptest.NewRequest(http.MethodDelete, "/v2/spaces/space1/widgets/w1?scope=space&dry_run=true", nil)
		w = httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.True(t, got.DryRun)
		assert.False(t, got.Removed)
	})

	t.Run("PATCH refuses the identity members even beside a valid one", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		fx.registerWidgetRoutes()
		for _, body := range []string{`{"target":"other","limit":10}`, `{"scope":"personal","limit":10}`} {
			req := httptest.NewRequest(http.MethodPatch, "/v2/spaces/space1/widgets/w1", strings.NewReader(body))
			w := httptest.NewRecorder()
			fx.router.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, body)
			assert.Contains(t, w.Body.String(), `"path":"/`, body)
		}
	})

	t.Run("a committed move carries its receipt on the wire", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		fx.registerWidgetRoutes()
		fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return([]apicore.WidgetEntry{testWidgetEntry("w1", "space", "a"), testWidgetEntry("w2", "space", "b")}, nil)
		fx.widgetsMock.EXPECT().CanEditWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return(true, nil)
		fx.widgetsMock.EXPECT().UpdateWidget(mock.Anything, "space1", apicore.WidgetScopeSpace, "w2", mock.Anything).RunAndReturn(
			func(_ context.Context, _ string, _ apicore.WidgetScope, _ string, plan func(apicore.WidgetEntry) (apicore.WidgetUpdate, error)) (apicore.WidgetEntry, error) {
				update, err := plan(testWidgetEntry("w2", "space", "b"))
				require.NoError(t, err)
				assert.Equal(t, &apicore.WidgetPlacement{BeforeId: "w1"}, update.Placement)
				return testWidgetEntry("w2", "space", "b"), nil
			})

		req := httptest.NewRequest(http.MethodPatch, "/v2/spaces/space1/widgets/w2?scope=space", strings.NewReader(`{"before":"a"}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.JSONEq(t, `{"id":"w2","scope":"space","target":"b","layout":"tree","limit":6,"placed":{"before":"w1"}}`, w.Body.String())
	})

	t.Run("an explicit null member is refused, on POST and beside a valid PATCH member", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		fx.registerWidgetRoutes()
		for _, probe := range []struct{ method, path, body, want string }{
			{http.MethodPost, "/v2/spaces/space1/widgets", `{"target":"page1","scope":"personal","after":null}`, "/after"},
			{http.MethodPatch, "/v2/spaces/space1/widgets/w1", `{"view_id":null,"limit":10}`, "/view_id"},
			{http.MethodPatch, "/v2/spaces/space1/widgets/w1", `{"layout":null}`, "/layout"},
		} {
			req := httptest.NewRequest(probe.method, probe.path, strings.NewReader(probe.body))
			w := httptest.NewRecorder()
			fx.router.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, probe.body)
			var got v2model.Error
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.Len(t, got.Issues, 1, probe.body)
			assert.Equal(t, probe.want, got.Issues[0].Path, probe.body)
			assert.Contains(t, got.Issues[0].Message, "null is not a value here")
		}
	})

	t.Run("a list carries the unknown-layout warning on the wire", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		fx.registerWidgetRoutes()
		fx.store.AddObjects(t, "space1", []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("set1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
		}})
		fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return([]apicore.WidgetEntry{{Id: "w1", Scope: apicore.WidgetScopeSpace, Target: "set1", Layout: model.BlockContentWidgetLayout(99), Limit: 6}}, nil)
		fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, "space1", apicore.WidgetScopePersonal).Return(nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/v2/spaces/space1/widgets", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got v2model.ListResponse[v2model.WidgetRow]
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Len(t, got.Data, 1)
		assert.Equal(t, "view", got.Data[0].Layout)
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, "layout 99")
	})

	t.Run("a forbidden scope is a 403 with the personal steer", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		fx.registerWidgetRoutes()
		fx.widgetsMock.EXPECT().CanEditWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return(false, nil)

		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/widgets", strings.NewReader(`{"target":"page1","scope":"space"}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "scope personal")
	})
}
