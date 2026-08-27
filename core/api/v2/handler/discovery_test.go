package v2handler

// discovery_test.go — handler-layer pins for the discovery routes. The
// GetType ?ids= regression shipped AT THIS LAYER (the handler hardcoded
// ObjectQuery{}), while the only test lived at the service layer — a
// handler that stops threading the query keeps every service test green.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

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
				"id":   pbtypes.String("type-task"),
				"name": pbtypes.String("Task"),
			}},
			ObjectTypes: []string{"ot-objectType"},
			Blocks: []*model.Block{
				{Id: "type-task", ChildrenIds: []string{testTypeMintedBlockId},
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
				bundle.RelationKeyId:             domain.String("type-task"),
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
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "type-task").Return(typeReadWithMintedIds(), nil).Times(2)

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
