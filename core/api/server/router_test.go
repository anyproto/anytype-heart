package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// localApiHost is what a real client sends; httptest defaults to example.com,
// which the origin gate treats as a DNS-rebinding attempt.
const localApiHost = "127.0.0.1:31009"

func TestRouter_Unauthenticated(t *testing.T) {
	t.Run("GET /v1/spaces without auth returns 401", func(t *testing.T) {
		// given
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/spaces", nil)
		req.Host = localApiHost

		// when
		engine.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestRouter_AuthRoute(t *testing.T) {
	t.Run("POST /v1/auth/token is accessible without auth", func(t *testing.T) {
		// given
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/auth/token", nil)
		req.Host = localApiHost

		// when
		engine.ServeHTTP(w, req)

		// then
		require.NotEqual(t, http.StatusUnauthorized, w.Code)
	})
}

func TestRouter_MetadataHeader(t *testing.T) {
	t.Run("Response includes Anytype-Version header", func(t *testing.T) {
		// given
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "dummyToken", AppName: "dummyApp"}}
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}, nil).Once()
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/spaces", nil)
		req.Host = localApiHost
		req.Header.Set("Authorization", "Bearer validKey")

		// when
		engine.ServeHTTP(w, req)

		// then
		require.Equal(t, "2025-11-08", w.Header().Get("Anytype-Version"))
	})
}

func TestRouter_TrustedOrigin(t *testing.T) {
	// The API answers with no CORS headers, so a site cannot read a response.
	// It can still reach a handler with a preflight-free "simple" request, and
	// the /v1/auth routes need no token, so the origin gate has to run first.
	tests := []struct {
		name    string
		host    string
		headers map[string]string
		want    int
	}{
		{
			name: "native client without an origin is served",
			host: localApiHost,
			want: http.StatusBadRequest, // reaches the handler, body is empty
		},
		{
			name:    "local browser client on a loopback origin is served",
			host:    localApiHost,
			headers: map[string]string{"Origin": "http://localhost:3000"},
			want:    http.StatusBadRequest,
		},
		{
			name:    "cross-origin form post from a site is refused",
			host:    localApiHost,
			headers: map[string]string{"Origin": "https://evil.com", "Content-Type": "text/plain"},
			want:    http.StatusForbidden,
		},
		{
			name:    "sandboxed iframe is refused",
			host:    localApiHost,
			headers: map[string]string{"Origin": "null"},
			want:    http.StatusForbidden,
		},
		{
			name: "dns rebinding is refused",
			host: "evil.com:31009",
			want: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			fx := newFixture(t)
			engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/auth/challenges", nil)
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			// when
			engine.ServeHTTP(w, req)

			// then
			require.Equal(t, tt.want, w.Code)
		})
	}
}

func TestRouter_BatchGetObjects(t *testing.T) {
	mockObjectShow := func(objectId, name string) *model.ObjectView {
		createdDate := 888888.0
		return &model.ObjectView{
			RootId: objectId,
			Details: []*model.ObjectViewDetailsSet{
				{
					Id: objectId,
					Details: &types.Struct{
						Fields: map[string]*types.Value{
							bundle.RelationKeyId.String():             pbtypes.String(objectId),
							bundle.RelationKeyName.String():           pbtypes.String(name),
							bundle.RelationKeyResolvedLayout.String(): pbtypes.Float64(float64(model.ObjectType_basic)),
							bundle.RelationKeyType.String():           pbtypes.String("mocked-type-id"),
							bundle.RelationKeySpaceId.String():        pbtypes.String("mocked-space-id"),
							bundle.RelationKeyLastModifiedDate.String(): pbtypes.Float64(999999),
							bundle.RelationKeyCreatedDate.String():      pbtypes.Float64(createdDate),
						},
					},
				},
			},
		}
	}

	t.Run("POST /v1/spaces/:space_id/objects/batch without auth returns 401", func(t *testing.T) {
		// given
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/spaces/mocked-space-id/objects/batch", nil)
		req.Host = localApiHost

		// when
		engine.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("POST /v1/spaces/:space_id/objects/batch returns batch results", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "dummyToken", AppName: "dummyApp"}}
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		ids := []string{"obj-1", "missing", "obj-2"}
		for _, objId := range ids {
			if objId == "missing" {
				fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
					SpaceId:  "mocked-space-id",
					ObjectId: objId,
				}).Return(&pb.RpcObjectShowResponse{
					Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NOT_FOUND},
				}, nil).Once()
				continue
			}
			fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
				SpaceId:  "mocked-space-id",
				ObjectId: objId,
			}).Return(&pb.RpcObjectShowResponse{
				Error:      &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: mockObjectShow(objId, objId),
			}, nil).Once()
			fx.mwMock.On("ObjectExport", mock.Anything, &pb.RpcObjectExportRequest{
				SpaceId:  "mocked-space-id",
				ObjectId: objId,
				Format:   model.Export_Markdown,
			}).Return(&pb.RpcObjectExportResponse{
				Result: "body",
				Error:  &pb.RpcObjectExportResponseError{Code: pb.RpcObjectExportResponseError_NULL},
			}, nil).Once()
		}

		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/spaces/mocked-space-id/objects/batch", strings.NewReader(`{"ids":["obj-1","missing","obj-2"]}`))
		req.Host = localApiHost
		req.Header.Set("Authorization", "Bearer validKey")
		req.Header.Set("Content-Type", "application/json")

		// when
		engine.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		var responses []*apimodel.ObjectResponse
		err := json.Unmarshal(w.Body.Bytes(), &responses)
		require.NoError(t, err)
		require.Len(t, responses, 3)
		require.NotNil(t, responses[0])
		require.Equal(t, "obj-1", responses[0].Object.Id)
		require.Equal(t, "obj-1", responses[0].Object.Name)
		require.Nil(t, responses[1])
		require.NotNil(t, responses[2])
		require.Equal(t, "obj-2", responses[2].Object.Id)
		require.Equal(t, "obj-2", responses[2].Object.Name)
	})

	t.Run("POST /v1/spaces/:space_id/objects/batch with invalid body returns 400", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "dummyToken", AppName: "dummyApp"}}
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/spaces/mocked-space-id/objects/batch", strings.NewReader(`not json`))
		req.Host = localApiHost
		req.Header.Set("Authorization", "Bearer validKey")
		req.Header.Set("Content-Type", "application/json")

		// when
		engine.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
}
