package v2handler

// space_test.go pins the Phase-7 space HTTP layer: the dry-run
// plumbing (a regressed dry_run would CREATE A REAL SPACE), the strict body
// decode, and the status codes.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// spaceRouterFixture mounts the three Phase-7 space routes and stamps
// name/description onto space1's space view.
func spaceRouterFixture(t *testing.T) *v2HandlerFixture {
	fx := newV2HandlerFixture(t)
	fx.store.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("spaceView_space1"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeyTargetSpaceId:  domain.String("space1"),
		bundle.RelationKeyName:           domain.String("Work"),
		bundle.RelationKeyDescription:    domain.String("The wiki"),
	}})
	fx.router.Use(withDryRunFlag())
	fx.router.GET("/v2/spaces/:space_id", GetSpaceHandler(fx.svc))
	fx.router.POST("/v2/spaces", CreateSpaceHandler(fx.svc))
	fx.router.PATCH("/v2/spaces/:space_id", UpdateSpaceHandler(fx.svc))
	return fx
}

func serveSpace(fx *v2HandlerFixture, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, req)
	return w
}

func TestGetSpaceHandler(t *testing.T) {
	t.Run("returns the row from the space view", func(t *testing.T) {
		// given
		fx := spaceRouterFixture(t)

		// when
		w := serveSpace(fx, http.MethodGet, "/v2/spaces/space1", "")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		var got v2model.Space
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.Space{Id: "space1", Name: "Work", Description: "The wiki"}, got)
	})

	t.Run("unknown space is a C6 404", func(t *testing.T) {
		// given
		fx := spaceRouterFixture(t)

		// when
		w := serveSpace(fx, http.MethodGet, "/v2/spaces/bogus", "")

		// then
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), `"not_found"`)
	})
}

func TestCreateSpaceHandler(t *testing.T) {
	t.Run("creates and responds 201", func(t *testing.T) {
		// given
		fx := spaceRouterFixture(t)
		fx.mwMock.EXPECT().WorkspaceCreate(mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceCreateResponse{SpaceId: "newSpace1"})

		// when
		w := serveSpace(fx, http.MethodPost, "/v2/spaces", `{"name":"Research"}`)

		// then
		require.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), `"id":"newSpace1"`)
	})

	t.Run("dry_run=true sends nothing and responds 200 (C9)", func(t *testing.T) {
		// given: any RPC fails the mock — a regressed dry run would create a
		// real space
		fx := spaceRouterFixture(t)

		// when
		w := serveSpace(fx, http.MethodPost, "/v2/spaces?dry_run=true", `{"name":"Research"}`)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"dry_run":true`)
	})

	t.Run("an unknown body field is a strict 400 naming it", func(t *testing.T) {
		// given
		fx := spaceRouterFixture(t)

		// when
		w := serveSpace(fx, http.MethodPost, "/v2/spaces", `{"name":"X","gatewayUrl":"nope"}`)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "gatewayUrl")
	})
}

func TestUpdateSpaceHandler(t *testing.T) {
	t.Run("patches and returns the merged row", func(t *testing.T) {
		// given
		fx := spaceRouterFixture(t)
		fx.mwMock.EXPECT().WorkspaceSetInfo(mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceSetInfoResponse{})

		// when
		w := serveSpace(fx, http.MethodPatch, "/v2/spaces/space1", `{"name":"Renamed"}`)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		var got v2model.Space
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.Space{Id: "space1", Name: "Renamed", Description: "The wiki"}, got)
	})

	t.Run("an empty update body is a 400", func(t *testing.T) {
		// given
		fx := spaceRouterFixture(t)

		// when
		w := serveSpace(fx, http.MethodPatch, "/v2/spaces/space1", `{}`)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "at least one")
	})

	t.Run("dry_run=true reports the would-be row and sends nothing (C9)", func(t *testing.T) {
		// given: any RPC fails the mock
		fx := spaceRouterFixture(t)

		// when
		w := serveSpace(fx, http.MethodPatch, "/v2/spaces/space1?dry_run=true", `{"description":"New"}`)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"dry_run":true`)
		assert.Contains(t, w.Body.String(), `"description":"New"`)
	})
}
