package v2handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestDeleteObjectHandlerQuery(t *testing.T) {
	serve := func(fx *v2HandlerFixture, query string) *httptest.ResponseRecorder {
		fx.router.DELETE("/v2/spaces/:space_id/objects/:object_id", DeleteObjectHandler(fx.svc))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v2/spaces/space1/objects/obj1"+query, nil))
		return w
	}
	decodeError := func(t *testing.T, w *httptest.ResponseRecorder) v2model.Error {
		t.Helper()
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		return got
	}
	pageRead := apicore.ObjectRead{SbType: model.SmartBlockType_Page, Snapshot: &model.SmartBlockSnapshotBase{}}

	t.Run("skip_bin is refused, naming permanent", func(t *testing.T) {
		// given: no reader expectation — the refusal comes before the service
		fx := newV2HandlerFixture(t)

		// when
		w := serve(fx, "?skip_bin=true")

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		got := decodeError(t, w)
		require.Len(t, got.Issues, 1)
		assert.Equal(t, "skip_bin", got.Issues[0].Path)
		assert.Equal(t, []v2model.Ref{v2model.Resend("permanent", "true")}, got.Issues[0].SeeAlso)
	})

	t.Run("a permanent value other than true or false is refused", func(t *testing.T) {
		fx := newV2HandlerFixture(t)

		w := serve(fx, "?permanent=yes")

		require.Equal(t, http.StatusBadRequest, w.Code)
		got := decodeError(t, w)
		require.Len(t, got.Issues, 1)
		assert.Equal(t, "permanent", got.Issues[0].Path)
	})

	t.Run("without permanent the object is archived", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "obj1").Return(pageRead, nil).Once()
		fx.readerMock.EXPECT().InBin(mock.Anything, "space1", "obj1").Return(false, nil).Once()
		fx.mwMock.On("ObjectSetIsArchived", mock.Anything, &pb.RpcObjectSetIsArchivedRequest{ContextId: "obj1", IsArchived: true}).
			Return(&pb.RpcObjectSetIsArchivedResponse{Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL}}).Once()

		w := serve(fx, "?permanent=false")

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("permanent=true reaches the permanent path", func(t *testing.T) {
		// the fixture has no provenance reader, and a permanent delete
		// refuses without one: a 500 here, and no archive RPC, is what
		// shows the flag arrived
		fx := newV2HandlerFixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "obj1").Return(pageRead, nil).Once()
		fx.readerMock.EXPECT().InBin(mock.Anything, "space1", "obj1").Return(false, nil).Once()

		w := serve(fx, "?permanent=true")

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
