package server

// A local-API request's attribution, end to end at the HTTP layer
// (docs/ApiChangeAttribution.md): through the REAL engine — the router, the
// shared auth middleware, the version's own middlewares, the handler and the
// service — the app name of the key that made the request rides the context
// handed to heart, for a v1 route and for a v2 route. From that boundary on:
// core.TestNewContextCarriesTheRequestAttribution (request context → session
// context), api.TestAdaptersStampTheRequest (request context → applied
// state), smartblock.TestSmartBlock_EditAttribution (applied state → pushed
// change) and sourceimpl.TestBuildChange_IntegrationName (→ pb.Change).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestWriteRoutesCarryTheAppName(t *testing.T) {
	newKeyedFixture := func(t *testing.T) *fixture {
		fx := newV2ServerFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"raycastKey":  {Token: "tok1", AppName: "Raycast", Scope: model.AccountAuth_JsonAPI},
			"namelessKey": {Token: "tok2", Scope: model.AccountAuth_JsonAPI},
		}
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("spaceView_space1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:  domain.String("space1"),
		}})
		fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("page1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})
		return fx
	}
	serve := func(t *testing.T, fx *fixture, key, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Host = localApiHost
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		fx.Engine().ServeHTTP(w, req)
		return w
	}

	t.Run("v1 DELETE list object: heart is called with the key's app name on the context", func(t *testing.T) {
		for _, tc := range []struct{ key, want string }{
			// red if ensureAuthenticated stops installing the name, or any
			// v1 middleware, handler or service replaces the request context
			{"raycastKey", "Raycast"},
			// red if a name is ever installed for a key that has none
			{"namelessKey", ""},
		} {
			// given
			fx := newKeyedFixture(t)
			var got []string
			fx.mwMock.EXPECT().ObjectCollectionRemove(mock.Anything, mock.Anything).
				RunAndReturn(func(ctx context.Context, _ *pb.RpcObjectCollectionRemoveRequest) *pb.RpcObjectCollectionRemoveResponse {
					got = append(got, domain.IntegrationNameFromCtx(ctx))
					return &pb.RpcObjectCollectionRemoveResponse{Error: &pb.RpcObjectCollectionRemoveResponseError{Code: pb.RpcObjectCollectionRemoveResponseError_NULL}}
				})

			// when
			w := serve(t, fx, tc.key, "DELETE", "/v1/spaces/space1/lists/list1/objects/page1", "")

			// then
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, []string{tc.want}, got, "key %s", tc.key)
		}
	})

	t.Run("v2 PATCH widget: the adapter is called with the key's app name on the context", func(t *testing.T) {
		for _, tc := range []struct{ key, want string }{
			// red if the v2 group (idempotency, dry-run, space-ref middlewares)
			// or the v2 service detaches the write from the request context
			{"raycastKey", "Raycast"},
			{"namelessKey", ""},
		} {
			// given
			fx := newKeyedFixture(t)
			stored := apicore.WidgetEntry{Id: "w1", Scope: apicore.WidgetScopeSpace, Target: "page1", Layout: model.BlockContentWidget_Tree, Limit: 6}
			fx.widgetsMock.EXPECT().CanEditWidgets(mock.Anything, "space1", mock.Anything).Return(true, nil).Maybe()
			fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, "space1", apicore.WidgetScopeSpace).Return([]apicore.WidgetEntry{stored}, nil).Maybe()
			fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, "space1", apicore.WidgetScopePersonal).Return(nil, nil).Maybe()
			var got []string
			fx.widgetsMock.EXPECT().UpdateWidget(mock.Anything, "space1", apicore.WidgetScopeSpace, "w1", mock.Anything).
				RunAndReturn(func(ctx context.Context, _ string, _ apicore.WidgetScope, _ string, _ func(apicore.WidgetEntry) (apicore.WidgetUpdate, error)) (apicore.WidgetEntry, error) {
					got = append(got, domain.IntegrationNameFromCtx(ctx))
					updated := stored
					updated.Limit = 10
					return updated, nil
				})

			// when
			w := serve(t, fx, tc.key, "PATCH", "/v2/spaces/space1/widgets/w1", `{"limit":10}`)

			// then
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, []string{tc.want}, got, "key %s", tc.key)
		}
	})
}
