package v2handler

import (
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
	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// testAccountId feeds the §6.2 current-user placeholder substitution.
const testAccountId = "accountA"

type v2HandlerFixture struct {
	svc         *v2service.V2Service
	mwMock      *mock_apicore.MockClientCommands
	readerMock  *mock_apicore.MockObjectReader
	creatorMock *mock_apicore.MockObjectCreator
	store       *objectstore.StoreFixture
	router      *gin.Engine
}

func newV2HandlerFixture(t *testing.T) *v2HandlerFixture {
	gin.SetMode(gin.TestMode)
	mwMock := mock_apicore.NewMockClientCommands(t)
	readerMock := mock_apicore.NewMockObjectReader(t)
	store := objectstore.NewStoreFixture(t)
	// register space1 so the C2 ensureSpace guard resolves the test space
	store.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("spaceView_space1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:  domain.String("space1"),
		},
	})
	creatorMock := mock_apicore.NewMockObjectCreator(t)
	svc := v2service.NewV2Service(mwMock, readerMock, creatorMock, mock_apicore.NewMockObjectMutator(t), store, objectstore.TestTechSpaceId, testAccountId)
	return &v2HandlerFixture{svc: svc, mwMock: mwMock, readerMock: readerMock, creatorMock: creatorMock, store: store, router: gin.New()}
}

func TestValidateV2Handler(t *testing.T) {
	t.Run("valid body returns empty issue lists", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.router.POST("/v2/validate", ValidateV2Handler(fx.svc))
		want := v2model.ValidateResponse{Issues: []v2model.Issue{}, Warnings: []v2model.Issue{}}

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/validate",
			strings.NewReader(`{"version":1,"blocks":[{"type":"paragraph","text":"hi"}]}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		var got v2model.ValidateResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, want, got)
	})

	t.Run("invalid body returns issues as data with status 200", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.router.POST("/v2/validate", ValidateV2Handler(fx.svc))

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/validate", strings.NewReader(`{"version":1,"blocks":[{"type":"wat"}]}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		var got v2model.ValidateResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.NotEmpty(t, got.Issues)
	})
}

func TestGetObjectV2Handler(t *testing.T) {
	t.Run("conflicting params map to 400 ambiguous_input", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.router.GET("/v2/spaces/:space_id/objects/:object_id", GetObjectV2Handler(fx.svc))

		// when
		req := httptest.NewRequest(http.MethodGet, "/v2/spaces/space1/objects/obj1?outline=true&block=b1", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.CodeAmbiguousInput, got.Code)
		require.Len(t, got.Issues, 2)
		assert.Equal(t, "outline", got.Issues[0].Path)
	})

	t.Run("internal errors map to 500 internal_error", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.router.GET("/v2/spaces/:space_id/objects/:object_id", GetObjectV2Handler(fx.svc))
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "obj1").Return(apicore.ObjectRead{}, assert.AnError)

		// when
		req := httptest.NewRequest(http.MethodGet, "/v2/spaces/space1/objects/obj1", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusInternalServerError, w.Code)
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.CodeInternalError, got.Code)
	})
}
