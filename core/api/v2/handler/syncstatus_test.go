package v2handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestGetSyncStatusHandler(t *testing.T) {
	// newRouter serves the route over a service holding the sync port. The
	// port is a strict mock: a refused request that still read the status
	// fails the test.
	newRouter := func(t *testing.T) (*v2HandlerFixture, *mock_apicore.MockSyncStatus) {
		fx := newV2HandlerFixture(t)
		syncStatus := mock_apicore.NewMockSyncStatus(t)
		svc := v2service.NewService(fx.mwMock, fx.readerMock, fx.creatorMock, nil, nil, fx.widgetsMock, nil, nil, nil,
			syncStatus, fx.fileMock, fx.store, objectstore.TestTechSpaceId, testAccountId)
		fx.router.Use(pagination.New(pagination.Config{DefaultPage: 0, DefaultPageSize: 25, MinPageSize: 1, MaxPageSize: 1000}))
		fx.router.GET("/v2/spaces/:space_id/sync", GetSyncStatusHandler(svc))
		return fx, syncStatus
	}

	t.Run("serves the status with snake_case fields; count_only drops the list", func(t *testing.T) {
		// given: one pending object, so a handler ignoring count_only shows
		fx, syncStatus := newRouter(t)
		fx.store.AddObjects(t, "space1", []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Draft"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeySyncStatus:     domain.Int64(int64(domain.ObjectSyncStatusSyncing)),
		}})
		syncStatus.EXPECT().SpaceSyncStatus("space1").Return(&pb.EventSpaceSyncStatusUpdate{
			Status:                pb.EventSpace_Syncing,
			Network:               pb.EventSpace_Anytype,
			SyncingObjectsCounter: 1,
		}, true, nil).Times(2)
		syncStatus.EXPECT().SpaceStorage("space1").Return(apicore.SpaceStorage{
			LimitBytes: 1000, NodeUsedBytes: 10, ReportedAt: time.Now(),
		}, true, nil).Times(2)
		want := map[string]any{
			"space_id":                 "space1",
			"status":                   "syncing",
			"network":                  "anytype",
			"pending_object_count":     float64(1),
			"uploading_file_count":     float64(0),
			"not_backed_up_file_count": float64(0),
			"upgrade_needed":           false,
			"storage": map[string]any{
				"used_bytes":    float64(10),
				"pending_bytes": float64(0),
				"limit_bytes":   float64(1000),
				"left_bytes":    float64(990),
			},
		}
		serve := func(target string) map[string]any {
			w := httptest.NewRecorder()
			fx.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var got map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			return got
		}

		// when
		countOnly := serve("/v2/spaces/space1/sync?count_only=true")
		full := serve("/v2/spaces/space1/sync")

		// then
		assert.Equal(t, want, countOnly)
		require.Contains(t, full, "objects")
		assert.Equal(t, []any{map[string]any{"id": "obj1", "name": "Draft", "type": "", "status": "syncing"}}, full["objects"])
	})

	t.Run("an invalid count_only is a 400 before anything is read", func(t *testing.T) {
		// given
		fx, _ := newRouter(t)

		// when
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v2/spaces/space1/sync?count_only=yes", nil))

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.CodeValidationFailed, got.Code)
		require.Len(t, got.Issues, 1)
		assert.Equal(t, "count_only", got.Issues[0].Path)
	})
}
