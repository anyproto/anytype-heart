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
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
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
		// the whole request, not a resend: a resend keeps the skip_bin the
		// route just refused
		assert.Equal(t, []v2model.Ref{v2model.NewRef(v2model.OpDeleteObject, "space_id", "space1", "object_id", "obj1").With("permanent", "true")}, got.Issues[0].SeeAlso)
	})

	t.Run("skip_bin on a dry run steers to a preview of the delete for good", func(t *testing.T) {
		// given: replacing skip_bin with permanent=true on a dry run still
		// only probes, so the repair says so
		deleteObject := v2model.NewRef(v2model.OpDeleteObject, "space_id", "space1", "object_id", "obj1")
		cases := []struct {
			query string
			want  string
			ref   v2model.Ref
		}{
			{query: "?skip_bin=true", want: "to delete for good, send DELETE /v2/spaces/space1/objects/obj1?permanent=true instead",
				ref: deleteObject.With("permanent", "true")},
			{query: "?skip_bin=true&dry_run=true", want: "to preview a delete for good, send DELETE /v2/spaces/space1/objects/obj1?dry_run=true&permanent=true instead",
				ref: deleteObject.With("permanent", "true").With("dry_run", "true")},
		}
		for _, tc := range cases {
			fx := newV2HandlerFixture(t)
			fx.router.DELETE("/v2/spaces/:space_id/objects/:object_id", withDryRunFlag(), DeleteObjectHandler(fx.svc))
			w := httptest.NewRecorder()

			// when
			fx.router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v2/spaces/space1/objects/obj1"+tc.query, nil))

			// then
			require.Equal(t, http.StatusBadRequest, w.Code, tc.query)
			got := decodeError(t, w)
			require.Len(t, got.Issues, 1, tc.query)
			assert.Equal(t, tc.want, got.Issues[0].Hint, tc.query)
			assert.Equal(t, []v2model.Ref{tc.ref}, got.Issues[0].SeeAlso, tc.query)
		}
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

// A reference a preview hands out is a preview too, however it is followed:
// copied as a REST query in place of the request's (a resend), or sent as
// the whole request it names. The mw mock is strict and holds no delete or
// archive expectation, so a followed reference that writes fails the test.
func TestDeleteObjectPreviewRefsStayPreviews(t *testing.T) {
	const path = "/v2/spaces/space1/objects/obj1"
	newFx := func(t *testing.T) *v2HandlerFixture {
		fx := newV2HandlerFixture(t)
		provenance := mock_apicore.NewMockObjectProvenance(t)
		provenance.EXPECT().CreatorProvenance(mock.Anything, "space1", "obj1").
			Return(apicore.Provenance{AccountMatch: true, IntegrationName: "Claude Desktop"}, nil).Maybe()
		fx.svc = v2service.NewService(fx.mwMock, fx.readerMock, fx.creatorMock, mock_apicore.NewMockObjectMutator(t), provenance,
			fx.widgetsMock, nil, nil, nil, nil, fx.fileMock, fx.store, objectstore.TestTechSpaceId, testAccountId)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "obj1").
			Return(apicore.ObjectRead{SbType: model.SmartBlockType_Page, Snapshot: &model.SmartBlockSnapshotBase{}}, nil).Maybe()
		fx.readerMock.EXPECT().InBin(mock.Anything, "space1", "obj1").Return(false, nil).Maybe()
		// this key created the object, so a permanent delete is allowed: the
		// one shape in which a followed reference could destroy it
		fx.router.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(domain.CtxWithIntegrationName(c.Request.Context(), "Claude Desktop"))
			c.Next()
		})
		fx.router.DELETE("/v2/spaces/:space_id/objects/:object_id", withDryRunFlag(), DeleteObjectHandler(fx.svc))
		return fx
	}
	serve := func(fx *v2HandlerFixture, target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, target, nil))
		return w
	}
	decodeError := func(t *testing.T, w *httptest.ResponseRecorder) v2model.Error {
		t.Helper()
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		return got
	}
	// follow sends what a REST caller would: a resend's query in place of the
	// request's, or the whole request a bound reference renders
	follow := func(t *testing.T, fx *v2HandlerFixture, ref v2model.Ref) v2model.CreateResult {
		t.Helper()
		target := path + ref.String()
		if ref.Op != "" {
			method, route, ok := strings.Cut(ref.String(), " ")
			require.True(t, ok)
			require.Equal(t, http.MethodDelete, method)
			target = route
		}
		w := serve(fx, target)
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", target, w.Body.String())
		var got v2model.CreateResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		return got
	}

	t.Run("the archive preview's permanent reference", func(t *testing.T) {
		// given
		fx := newFx(t)
		w := serve(fx, path+"?dry_run=true")
		require.Equal(t, http.StatusOK, w.Code)
		var preview v2model.CreateResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))
		require.Len(t, preview.Warnings, 1)
		var followed int
		for _, ref := range preview.Warnings[0].SeeAlso {
			if ref.Query["permanent"] != "true" {
				continue
			}
			followed++

			// when
			got := follow(t, fx, ref)

			// then
			assert.True(t, got.DryRun, "a preview's reference previews")
			assert.True(t, got.Permanent)
		}
		assert.Equal(t, 1, followed, "the preview offers the delete for good")
	})

	t.Run("the skip_bin preview's repair", func(t *testing.T) {
		// given
		fx := newFx(t)
		w := serve(fx, path+"?skip_bin=true&dry_run=true")
		require.Equal(t, http.StatusBadRequest, w.Code)
		refusal := decodeError(t, w)
		require.Len(t, refusal.Issues, 1)
		require.Len(t, refusal.Issues[0].SeeAlso, 1)

		// when
		got := follow(t, fx, refusal.Issues[0].SeeAlso[0])

		// then
		assert.True(t, got.DryRun, "a preview's repair previews")
		assert.True(t, got.Permanent)
	})
}
