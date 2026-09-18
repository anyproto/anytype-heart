package v2handler

// discovery_test.go — handler-layer pins for the discovery routes. The
// GetType ?ids= regression shipped AT THIS LAYER (the handler hardcoded
// ObjectQuery{}), while the only test lived at the service layer — a
// handler that stops threading the query keeps every service test green.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestListSpacesHandlerGrantVisibility(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		grant                    *util.ApiGrant
		secondStatus             model.SpaceStatus
		offset                   int
		wantTotal, wantRows      int
		wantMore, wantNotGranted bool
	}{
		{name: "narrow grant", grant: &util.ApiGrant{Spaces: []string{"space1"}, Perms: util.GrantPermsRead}, wantTotal: 1, wantRows: 1, wantNotGranted: true},
		{name: "empty page still reports withheld spaces", grant: &util.ApiGrant{Spaces: []string{"space1"}, Perms: util.GrantPermsRead}, offset: 10, wantTotal: 1, wantNotGranted: true},
		{name: "no granted live spaces", grant: &util.ApiGrant{Spaces: []string{"missing"}, Perms: util.GrantPermsRead}, wantNotGranted: true},
		{name: "explicit grant covers every live space", grant: &util.ApiGrant{Spaces: []string{"space1", "space2"}, Perms: util.GrantPermsRead}, wantTotal: 2, wantRows: 1, wantMore: true},
		{name: "all spaces", grant: &util.ApiGrant{AllSpaces: true, Perms: util.GrantPermsRead}, wantTotal: 2, wantRows: 1, wantMore: true},
		{name: "legacy key", wantTotal: 2, wantRows: 1, wantMore: true},
		{name: "deleted space does not count", grant: &util.ApiGrant{Spaces: []string{"space1"}, Perms: util.GrantPermsRead}, secondStatus: model.SpaceStatus_SpaceDeleted, wantTotal: 1, wantRows: 1},
		{name: "joining space does not count", grant: &util.ApiGrant{Spaces: []string{"space1"}, Perms: util.GrantPermsRead}, secondStatus: model.SpaceStatus_SpaceJoining, wantTotal: 1, wantRows: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newV2HandlerFixture(t)
			fx.store.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:                 domain.String("spaceView_space2"),
				bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
				bundle.RelationKeyTargetSpaceId:      domain.String("space2"),
				bundle.RelationKeyName:               domain.String("Other space"),
				bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(tc.secondStatus)),
			}})
			fx.router.GET("/v2/spaces", func(c *gin.Context) {
				c.Set(pagination.QueryParamOffset, tc.offset)
				c.Set(pagination.QueryParamLimit, 1)
			}, ListSpacesHandler(fx.svc))
			req := httptest.NewRequest(http.MethodGet, "/v2/spaces", nil)
			req = req.WithContext(util.CtxWithApiGrant(req.Context(), tc.grant))
			w := httptest.NewRecorder()
			fx.router.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			var response struct {
				Data                []map[string]any `json:"data"`
				Total               int              `json:"total"`
				HasMore             bool             `json:"has_more"`
				HasNotGrantedSpaces *bool            `json:"has_not_granted_spaces"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.NotNil(t, response.HasNotGrantedSpaces, "the flag is present even when false")
			assert.Equal(t, tc.wantNotGranted, *response.HasNotGrantedSpaces)
			assert.Equal(t, tc.wantTotal, response.Total)
			assert.Equal(t, tc.wantMore, response.HasMore)
			assert.Len(t, response.Data, tc.wantRows)
			assert.NotContains(t, w.Body.String(), "space2", "an ungranted space or a later page is not disclosed")
			assert.NotContains(t, w.Body.String(), "Other space")
		})
	}
}

// testTypeMintedBlockId relabels to "bbbb1" on the default (edit) shape.
const testTypeMintedBlockId = "0000000000000000000bbbb1"

// typeReadWithMintedIds is a type-object read whose block ids are
// minted-shaped, so the two `?ids=` shapes serve different spellings — the
// only way a handler test can tell whether the query reached the service.
func typeReadWithMintedIds() apicore.ObjectRead {
	return apicore.ObjectRead{
		SbType: model.SmartBlockType_Page,
		Snapshot: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("bafyreitasktype"),
				"name": pbtypes.String("Task"),
			}},
			ObjectTypes: []string{"ot-objectType"},
			Blocks: []*model.Block{
				{Id: "bafyreitasktype", ChildrenIds: []string{testTypeMintedBlockId},
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
				{Id: testTypeMintedBlockId,
					Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "about", Style: model.BlockContentText_Paragraph}}},
			},
		},
		Heads: []string{"headA"},
	}
}

func TestGetTypeHandler(t *testing.T) {
	newTypeFixture := func(t *testing.T) *v2HandlerFixture {
		fx := newV2HandlerFixture(t)
		fx.router.GET("/v2/spaces/:space_id/types/:type", GetTypeHandler(fx.svc))
		fx.store.AddObjects(t, "space1", []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("bafyreitasktype"),
				bundle.RelationKeyName:           domain.String("Task"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
		})
		return fx
	}
	get := func(fx *v2HandlerFixture, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	t.Run("?ids=full reaches the service — the export shape is one query parameter away", func(t *testing.T) {
		// given
		fx := newTypeFixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "bafyreitasktype").Return(typeReadWithMintedIds(), nil).Times(2)

		// when / then: default = the edit shape (labels), full = the stored ids
		w := get(fx, "/v2/spaces/space1/types/task")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"id":"bbbb1"`, "the default type read serves compact labels")
		assert.NotContains(t, w.Body.String(), testTypeMintedBlockId)

		w = get(fx, "/v2/spaces/space1/types/task?ids=full")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"id":"`+testTypeMintedBlockId+`"`,
			"?ids=full must thread through the handler to the service")
	})

	t.Run("an invalid ids value is the service's 400, not silently ignored", func(t *testing.T) {
		// given
		fx := newTypeFixture(t)

		// when
		w := get(fx, "/v2/spaces/space1/types/task?ids=compressed")

		// then
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid ids value")
	})
}
