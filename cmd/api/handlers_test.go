package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/webstradev/gin-pagination/v2/pkg/pagination"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/mock_core"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service/mock_service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	offset      = 0
	limit       = 100
	techSpaceId = "tech-space-id"
	gatewayUrl  = "http://localhost:31006"
	iconImage   = "bafyreialsgoyflf3etjm3parzurivyaukzivwortf32b4twnlwpwocsrri"
)

type fixture struct {
	*ApiServer
	mwMock         *mock_service.MockClientCommandsServer
	mwInternalMock *mock_core.MockMiddlewareInternal
	router         *gin.Engine
}

func newFixture(t *testing.T) *fixture {
	mw := mock_service.NewMockClientCommandsServer(t)
	mwInternal := mock_core.NewMockMiddlewareInternal(t)
	apiServer := &ApiServer{mw: mw, mwInternal: mwInternal, router: gin.Default()}

	paginator := pagination.New(
		pagination.WithPageText("offset"),
		pagination.WithSizeText("limit"),
		pagination.WithDefaultPage(0),
		pagination.WithDefaultPageSize(100),
		pagination.WithMinPageSize(1),
		pagination.WithMaxPageSize(1000),
	)

	auth := apiServer.router.Group("/v1/auth")
	{
		auth.POST("/displayCode", apiServer.authDisplayCodeHandler)
		auth.GET("/token", apiServer.authTokenHandler)
	}
	readOnly := apiServer.router.Group("/v1")
	{
		readOnly.GET("/spaces", paginator, apiServer.getSpacesHandler)
		readOnly.GET("/spaces/:space_id/members", paginator, apiServer.getMembersHandler)
		readOnly.GET("/spaces/:space_id/objects", paginator, apiServer.getObjectsHandler)
		readOnly.GET("/spaces/:space_id/objects/:object_id", apiServer.getObjectHandler)
		readOnly.GET("/spaces/:space_id/objectTypes", paginator, apiServer.getObjectTypesHandler)
		readOnly.GET("/spaces/:space_id/objectTypes/:typeId/templates", paginator, apiServer.getObjectTypeTemplatesHandler)
		readOnly.GET("/search", paginator, apiServer.searchHandler)
	}

	readWrite := apiServer.router.Group("/v1")
	{
		readWrite.POST("/spaces", apiServer.createSpaceHandler)
		readWrite.POST("/spaces/:space_id/objects", apiServer.createObjectHandler)
		readWrite.PUT("/spaces/:space_id/objects/:object_id", apiServer.updateObjectHandler)
	}

	return &fixture{
		ApiServer:      apiServer,
		mwMock:         mw,
		mwInternalMock: mwInternal,
		router:         apiServer.router,
	}
}

func TestApiServer_AuthDisplayCodeHandler(t *testing.T) {
	t.Run("successful challenge creation", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("AccountLocalLinkNewChallenge", mock.Anything, &pb.RpcAccountLocalLinkNewChallengeRequest{AppName: "api-test"}).
			Return(&pb.RpcAccountLocalLinkNewChallengeResponse{
				ChallengeId: "mocked-challenge-id",
				Error:       &pb.RpcAccountLocalLinkNewChallengeResponseError{Code: pb.RpcAccountLocalLinkNewChallengeResponseError_NULL},
			}).Once()

		// when
		req, _ := http.NewRequest("POST", "/v1/auth/displayCode", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)

		var response AuthDisplayCodeResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		require.Equal(t, "mocked-challenge-id", response.ChallengeId)
	})

	t.Run("failed challenge creation", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("AccountLocalLinkNewChallenge", mock.Anything, &pb.RpcAccountLocalLinkNewChallengeRequest{AppName: "api-test"}).
			Return(&pb.RpcAccountLocalLinkNewChallengeResponse{
				Error: &pb.RpcAccountLocalLinkNewChallengeResponseError{Code: pb.RpcAccountLocalLinkNewChallengeResponseError_UNKNOWN_ERROR},
			}).Once()

		// when
		req, _ := http.NewRequest("POST", "/v1/auth/displayCode", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiServer_AuthTokenHandler(t *testing.T) {
	t.Run("successful token retrieval", func(t *testing.T) {
		// given
		fx := newFixture(t)

		challengeId := "mocked-challenge-id"
		code := "mocked-code"
		sessionToken := "mocked-session-token"
		appKey := "mocked-app-key"

		fx.mwMock.On("AccountLocalLinkSolveChallenge", mock.Anything, &pb.RpcAccountLocalLinkSolveChallengeRequest{
			ChallengeId: challengeId,
			Answer:      code,
		}).
			Return(&pb.RpcAccountLocalLinkSolveChallengeResponse{
				SessionToken: sessionToken,
				AppKey:       appKey,
				Error:        &pb.RpcAccountLocalLinkSolveChallengeResponseError{Code: pb.RpcAccountLocalLinkSolveChallengeResponseError_NULL},
			}).Once()

		// when
		req, _ := http.NewRequest("GET", "/v1/auth/token?challenge_id="+challengeId+"&code="+code, nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)

		var response AuthTokenResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		require.Equal(t, sessionToken, response.SessionToken)
		require.Equal(t, appKey, response.AppKey)
	})

	t.Run("bad request", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		req, _ := http.NewRequest("GET", "/v1/auth/token", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("failed token retrieval", func(t *testing.T) {
		// given
		fx := newFixture(t)
		challengeId := "mocked-challenge-id"
		code := "mocked-code"

		fx.mwMock.On("AccountLocalLinkSolveChallenge", mock.Anything, &pb.RpcAccountLocalLinkSolveChallengeRequest{
			ChallengeId: challengeId,
			Answer:      code,
		}).
			Return(&pb.RpcAccountLocalLinkSolveChallengeResponse{
				Error: &pb.RpcAccountLocalLinkSolveChallengeResponseError{Code: pb.RpcAccountLocalLinkSolveChallengeResponseError_UNKNOWN_ERROR},
			}).Once()

		// when
		req, _ := http.NewRequest("GET", "/v1/auth/token?challenge_id="+challengeId+"&code="+code, nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiServer_GetSpacesHandler(t *testing.T) {
	t.Run("successful retrieval of spaces", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.accountInfo = &model.AccountInfo{TechSpaceId: techSpaceId, GatewayUrl: gatewayUrl}

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: techSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
				{
					RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
				},
				{
					RelationKey: bundle.RelationKeySpaceRemoteStatus.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
				},
			},
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: "name",
					Type:        model.BlockContentDataviewSort_Asc,
				},
			},
			Keys: []string{"targetSpaceId", "name", "iconEmoji", "iconImage"},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						"name":          pbtypes.String("Another Workspace"),
						"targetSpaceId": pbtypes.String("another-space-id"),
						"iconEmoji":     pbtypes.String(""),
						"iconImage":     pbtypes.String(iconImage),
					},
				},
				{
					Fields: map[string]*types.Value{
						"name":          pbtypes.String("My Workspace"),
						"targetSpaceId": pbtypes.String("my-space-id"),
						"iconEmoji":     pbtypes.String("🚀"),
						"iconImage":     pbtypes.String(""),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		fx.mwMock.On("WorkspaceOpen", mock.Anything, mock.Anything).Return(&pb.RpcWorkspaceOpenResponse{
			Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_NULL},
			Info: &model.AccountInfo{
				HomeObjectId:           "home-object-id",
				ArchiveObjectId:        "archive-object-id",
				ProfileObjectId:        "profile-object-id",
				MarketplaceWorkspaceId: "marketplace-workspace-id",
				WorkspaceObjectId:      "workspace-object-id",
				DeviceId:               "device-id",
				AccountSpaceId:         "account-space-id",
				WidgetsId:              "widgets-id",
				SpaceViewId:            "space-view-id",
				TechSpaceId:            "tech-space-id",
				GatewayUrl:             "gateway-url",
				LocalStoragePath:       "local-storage-path",
				TimeZone:               "time-zone",
				AnalyticsId:            "analytics-id",
				NetworkId:              "network-id",
			},
		}, nil).Twice()

		// when
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/spaces?offset=%d&limit=%d", offset, limit), nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)

		var response PaginatedResponse[Space]
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		require.Len(t, response.Data, 2)
		require.Equal(t, "Another Workspace", response.Data[0].Name)
		require.Equal(t, "another-space-id", response.Data[0].Id)
		require.Regexpf(t, regexp.MustCompile(gatewayUrl+`/image/`+iconImage), response.Data[0].Icon, "Icon URL does not match")
		require.Equal(t, "My Workspace", response.Data[1].Name)
		require.Equal(t, "my-space-id", response.Data[1].Id)
		require.Equal(t, "🚀", response.Data[1].Icon)
	})

	t.Run("no spaces found", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.accountInfo = &model.AccountInfo{TechSpaceId: techSpaceId}

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/spaces?offset=%d&limit=%d", offset, limit), nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("failed workspace open", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.accountInfo = &model.AccountInfo{TechSpaceId: techSpaceId}

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							"name":          pbtypes.String("My Workspace"),
							"targetSpaceId": pbtypes.String("my-space-id"),
							"iconEmoji":     pbtypes.String("🚀"),
							"iconImage":     pbtypes.String(""),
						},
					},
				},
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		fx.mwMock.On("WorkspaceOpen", mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceOpenResponse{
				Error: &pb.RpcWorkspaceOpenResponseError{Code: pb.RpcWorkspaceOpenResponseError_UNKNOWN_ERROR},
			}, nil).Once()

		// when
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/spaces?offset=%d&limit=%d", offset, limit), nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiServer_CreateSpaceHandler(t *testing.T) {
	t.Run("successful create space", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.mwMock.On("WorkspaceCreate", mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceCreateResponse{
				Error:   &pb.RpcWorkspaceCreateResponseError{Code: pb.RpcWorkspaceCreateResponseError_NULL},
				SpaceId: "new-space-id",
			}).Once()

		// when
		body := strings.NewReader(`{"name":"New Space"}`)
		req, _ := http.NewRequest("POST", "/v1/spaces", body)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "new-space-id")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		body := strings.NewReader(`{invalid json}`)
		req, _ := http.NewRequest("POST", "/v1/spaces", body)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("failed workspace creation", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.mwMock.On("WorkspaceCreate", mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceCreateResponse{
				Error: &pb.RpcWorkspaceCreateResponseError{Code: pb.RpcWorkspaceCreateResponseError_UNKNOWN_ERROR},
			}).Once()

		// when
		body := strings.NewReader(`{"name":"Fail Space"}`)
		req, _ := http.NewRequest("POST", "/v1/spaces", body)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiServer_GetMembersHandler(t *testing.T) {
	t.Run("successfully get members", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.accountInfo = &model.AccountInfo{TechSpaceId: techSpaceId, GatewayUrl: gatewayUrl}

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							"id":         pbtypes.String("member-1"),
							"name":       pbtypes.String("John Doe"),
							"iconEmoji":  pbtypes.String("👤"),
							"identity":   pbtypes.String("AAjEaEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMDZ"),
							"globalName": pbtypes.String("john.any"),
						},
					},
					{
						Fields: map[string]*types.Value{
							"id":         pbtypes.String("member-2"),
							"name":       pbtypes.String("Jane Doe"),
							"iconImage":  pbtypes.String(iconImage),
							"identity":   pbtypes.String("AAjLbEwPF4nkEh7AWkqEnzcQ8HziGB4ETjiTpvRCQvWnSMD4"),
							"globalName": pbtypes.String("jane.any"),
						},
					},
				},
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/spaces/my-space/members?offset=%d&limit=%d", offset, limit), nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)

		var response PaginatedResponse[Member]
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		require.Len(t, response.Data, 2)
		require.Equal(t, "member-1", response.Data[0].Id)
		require.Equal(t, "John Doe", response.Data[0].Name)
		require.Equal(t, "👤", response.Data[0].Icon)
		require.Equal(t, "john.any", response.Data[0].GlobalName)
		require.Equal(t, "member-2", response.Data[1].Id)
		require.Equal(t, "Jane Doe", response.Data[1].Name)
		require.Regexpf(t, regexp.MustCompile(gatewayUrl+`/image/`+iconImage), response.Data[1].Icon, "Icon URL does not match")
		require.Equal(t, "jane.any", response.Data[1].GlobalName)
	})

	t.Run("no members found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		req, _ := http.NewRequest("GET", fmt.Sprintf("/v1/spaces/empty-space/members?offset=%d&limit=%d", offset, limit), nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestApiServer_GetObjectsForSpaceHandler(t *testing.T) {
	t.Run("successfully get objects for a space", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							"id":               pbtypes.String("object-1"),
							"name":             pbtypes.String("My Object"),
							"type":             pbtypes.String("basic-type-id"),
							"layout":           pbtypes.Float64(float64(model.ObjectType_basic)),
							"iconEmoji":        pbtypes.String("📄"),
							"lastModifiedDate": pbtypes.Float64(1234567890),
						},
					},
				},
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Twice()

		// Mock type resolution
		fx.mwMock.On("ObjectShow", mock.Anything, mock.Anything).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								"name": pbtypes.String("Basic Type"),
							},
						},
					},
				},
			},
		}, nil).Maybe()

		// when
		req, _ := http.NewRequest("GET", "/v1/spaces/my-space/objects", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "My Object")
	})

	t.Run("no objects found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		req, _ := http.NewRequest("GET", "/v1/spaces/empty-space/objects", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestApiServer_GetObjectHandler(t *testing.T) {
	t.Run("object found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  "my-space",
			ObjectId: "obj-1",
		}).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
				ObjectView: &model.ObjectView{
					RootId: "root-1",
					Details: []*model.ObjectViewDetailsSet{
						{
							Details: &types.Struct{
								Fields: map[string]*types.Value{
									"name":      pbtypes.String("Found Object"),
									"type":      pbtypes.String("basic-type-id"),
									"iconEmoji": pbtypes.String("🔍"),
								},
							},
						},
					},
				},
			}, nil).Once()

		// Type resolution mock
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						"name": pbtypes.String("Basic Type"),
					},
				},
			},
		}, nil).Once()

		// when
		req, _ := http.NewRequest("GET", "/v1/spaces/my-space/objects/obj-1", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "Found Object")
	})

	t.Run("object not found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectShow", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectShowResponse{
				Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NOT_FOUND},
			}, nil).Once()

		// when
		req, _ := http.NewRequest("GET", "/v1/spaces/my-space/objects/missing-obj", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestApiServer_CreateObjectHandler(t *testing.T) {
	t.Run("successful object creation", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectCreate", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateResponse{
				Error:    &pb.RpcObjectCreateResponseError{Code: pb.RpcObjectCreateResponseError_NULL},
				ObjectId: "new-obj-id",
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						"name":      pbtypes.String("New Object"),
						"iconEmoji": pbtypes.String("🆕"),
						"spaceId":   pbtypes.String("my-space"),
					},
				},
			}).Once()

		// when
		body := strings.NewReader(`{"name":"New Object","icon":"🆕","template_id":"","object_type_unique_key":"basic","with_chat":false}`)
		req, _ := http.NewRequest("POST", "/v1/spaces/my-space/objects", body)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "new-obj-id")
	})

	t.Run("invalid json", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		body := strings.NewReader(`{invalid json}`)
		req, _ := http.NewRequest("POST", "/v1/spaces/my-space/objects", body)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("creation error", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectCreate", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateResponse{
				Error: &pb.RpcObjectCreateResponseError{Code: pb.RpcObjectCreateResponseError_UNKNOWN_ERROR},
			}).Once()

		// when
		body := strings.NewReader(`{"name":"Fail Object"}`)
		req, _ := http.NewRequest("POST", "/v1/spaces/my-space/objects", body)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiServer_UpdateObjectHandler(t *testing.T) {
	t.Run("not implemented", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		body := strings.NewReader(`{"name":"Updated Object"}`)
		req, _ := http.NewRequest("PUT", "/v1/spaces/my-space/objects/obj-1", body)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusNotImplemented, w.Code)
	})

	// TODO: further tests
}

func TestApiServer_GetObjectTypesHandler(t *testing.T) {
	t.Run("types found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{
					{
						Fields: map[string]*types.Value{
							"id":        pbtypes.String("type-1"),
							"name":      pbtypes.String("Type One"),
							"uniqueKey": pbtypes.String("type-one-key"),
							"iconEmoji": pbtypes.String("🗂️"),
						},
					},
				},
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		req, _ := http.NewRequest("GET", "/v1/spaces/my-space/objectTypes", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "Type One")
	})

	t.Run("no types found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		req, _ := http.NewRequest("GET", "/v1/spaces/my-space/objectTypes", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestApiServer_GetObjectTypeTemplatesHandler(t *testing.T) {
	t.Run("templates found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// Mock template type search
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						"id":        pbtypes.String("template-type-id"),
						"uniqueKey": pbtypes.String("ot-template"),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// Mock actual template objects search
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						"id":               pbtypes.String("template-1"),
						"targetObjectType": pbtypes.String("target-type-id"),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// Mock object show for template details
		fx.mwMock.On("ObjectShow", mock.Anything, mock.Anything).Return(&pb.RpcObjectShowResponse{
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
			ObjectView: &model.ObjectView{
				Details: []*model.ObjectViewDetailsSet{
					{
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								"name":      pbtypes.String("Template Name"),
								"iconEmoji": pbtypes.String("📝"),
							},
						},
					},
				},
			},
		}, nil).Once()

		// when
		req, _ := http.NewRequest("GET", "/v1/spaces/my-space/objectTypes/target-type-id/templates", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "Template Name")
	})

	t.Run("no template type found", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}).Once()

		// when
		req, _ := http.NewRequest("GET", "/v1/spaces/my-space/objectTypes/missing-type-id/templates", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestApiServer_SearchHandler(t *testing.T) {
	t.Run("objects found globally", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// Mock retrieving spaces first
		fx.accountInfo = &model.AccountInfo{TechSpaceId: techSpaceId}
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: techSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyLayout.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
				},
				{
					RelationKey: bundle.RelationKeySpaceLocalStatus.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
				},
				{
					RelationKey: bundle.RelationKeySpaceRemoteStatus.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Int64(int64(model.SpaceStatus_Ok)),
				},
			},
			Keys: []string{"targetSpaceId"},
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						"targetSpaceId": pbtypes.String("space-1"),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// Mock objects in space-1
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						"id":               pbtypes.String("obj-global-1"),
						"name":             pbtypes.String("Global Object"),
						"type":             pbtypes.String("global-type-id"),
						"layout":           pbtypes.Float64(float64(model.ObjectType_basic)),
						"iconEmoji":        pbtypes.String("🌐"),
						"lastModifiedDate": pbtypes.Float64(999999),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Twice()

		// Mock object show for object blocks and details
		fx.mwMock.On("ObjectShow", mock.Anything, &pb.RpcObjectShowRequest{
			SpaceId:  "space-1",
			ObjectId: "obj-global-1",
		}).Return(&pb.RpcObjectShowResponse{
			ObjectView: &model.ObjectView{
				RootId: "root-123",
				Blocks: []*model.Block{
					{
						Id: "root-123",
						Restrictions: &model.BlockRestrictions{
							Read:   false,
							Edit:   false,
							Remove: false,
							Drag:   false,
							DropOn: false,
						},
						ChildrenIds: []string{"header", "text-block", "relation-block"},
					},
					{
						Id: "header",
						Restrictions: &model.BlockRestrictions{
							Read:   false,
							Edit:   true,
							Remove: true,
							Drag:   true,
							DropOn: true,
						},
						ChildrenIds: []string{"title", "featuredRelations"},
					},
					{
						Id: "text-block",
						Content: &model.BlockContentOfText{
							Text: &model.BlockContentText{
								Text:  "This is a sample text block",
								Style: model.BlockContentText_Paragraph,
							},
						},
					},
				},
				Details: []*model.ObjectViewDetailsSet{
					{
						Id: "root-123",
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								"name":             pbtypes.String("Global Object"),
								"iconEmoji":        pbtypes.String("🌐"),
								"lastModifiedDate": pbtypes.Float64(999999),
								"createdDate":      pbtypes.Float64(888888),
								"tag":              pbtypes.StringList([]string{"tag-1", "tag-2"}),
							},
						},
					},
					{
						Id: "tag-1",
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								"name":                pbtypes.String("Important"),
								"relationOptionColor": pbtypes.String("red"),
							},
						},
					},
					{
						Id: "tag-2",
						Details: &types.Struct{
							Fields: map[string]*types.Value{
								"name":                pbtypes.String("Optional"),
								"relationOptionColor": pbtypes.String("blue"),
							},
						},
					},
				},
			},
			Error: &pb.RpcObjectShowResponseError{Code: pb.RpcObjectShowResponseError_NULL},
		}, nil).Once()

		// when
		req, _ := http.NewRequest("GET", "/v1/search", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)

		var response PaginatedResponse[Object]
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		require.Len(t, response.Data, 1)
		require.Equal(t, "space-1", response.Data[0].SpaceId)
		require.Equal(t, "Global Object", response.Data[0].Name)
		require.Equal(t, "obj-global-1", response.Data[0].Id)
		require.Equal(t, "🌐", response.Data[0].Icon)

		// check details
		for _, detail := range response.Data[0].Details {
			if detail.Id == "createdDate" {
				require.Equal(t, float64(888888), detail.Details["createdDate"])
			} else if detail.Id == "lastModifiedDate" {
				require.Equal(t, float64(999999), detail.Details["lastModifiedDate"])
			}
		}

		// check tags
		tags := []Tag{}
		for _, detail := range response.Data[0].Details {
			if tagList, ok := detail.Details["tags"].([]interface{}); ok {
				for _, tagValue := range tagList {
					tagStruct := tagValue.(map[string]interface{})
					tag := Tag{
						Id:    tagStruct["id"].(string),
						Name:  tagStruct["name"].(string),
						Color: tagStruct["color"].(string),
					}
					tags = append(tags, tag)
				}
			}
		}
		require.Len(t, tags, 2)
		require.Equal(t, "tag-1", tags[0].Id)
		require.Equal(t, "Important", tags[0].Name)
		require.Equal(t, "red", tags[0].Color)
		require.Equal(t, "tag-2", tags[1].Id)
		require.Equal(t, "Optional", tags[1].Name)
		require.Equal(t, "blue", tags[1].Color)
	})
}
