package v2handler

import (
	"encoding/json"
	"fmt"
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

	t.Run("skip_bin's repair follows its value, and a preview's previews", func(t *testing.T) {
		// given: true asked for a delete for good, false for the Bin; a value
		// that is neither, or contradicts permanent, gets no reference at all,
		// since guessing could destroy what the caller meant to keep
		deleteObject := v2model.NewRef(v2model.OpDeleteObject, "space_id", "space1", "object_id", "obj1")
		cases := []struct {
			query string
			hint  string
			refs  []v2model.Ref
		}{
			{query: "?skip_bin=true", hint: "to delete for good, send DELETE /v2/spaces/space1/objects/obj1?permanent=true instead",
				refs: []v2model.Ref{deleteObject.With("permanent", "true")}},
			{query: "?skip_bin=true&dry_run=true", hint: "to preview a delete for good, send DELETE /v2/spaces/space1/objects/obj1?dry_run=true&permanent=true instead",
				refs: []v2model.Ref{deleteObject.With("permanent", "true").With("dry_run", "true")}},
			{query: "?skip_bin=false", hint: "DELETE moves the object to the Bin without it: send DELETE /v2/spaces/space1/objects/obj1 instead",
				refs: []v2model.Ref{deleteObject}},
			{query: "?skip_bin=false&dry_run=true", hint: "to preview the move to the Bin, send DELETE /v2/spaces/space1/objects/obj1?dry_run=true instead",
				refs: []v2model.Ref{deleteObject.With("dry_run", "true")}},
			{query: "?skip_bin=yes", hint: "DELETE moves the object to the Bin; permanent=true deletes it for good instead"},
			{query: "?skip_bin=true&permanent=false", hint: "DELETE moves the object to the Bin; permanent=true deletes it for good instead"},
			{query: "?skip_bin=false&permanent=true", hint: "DELETE moves the object to the Bin; permanent=true deletes it for good instead"},
			{query: "?skip_bin=true&skip_bin=false", hint: "DELETE moves the object to the Bin; permanent=true deletes it for good instead"},
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
			assert.Equal(t, tc.hint, got.Issues[0].Hint, tc.query)
			assert.Equal(t, tc.refs, got.Issues[0].SeeAlso, tc.query)
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
// the whole request it names. The mw mock is strict and holds no delete,
// archive or restore expectation, so a followed reference that writes fails
// the test. Every reference a preview emits is followed, not only the
// destructive ones.
func TestDeleteObjectPreviewRefsStayPreviews(t *testing.T) {
	const path = "/v2/spaces/space1/objects/obj1"
	newFx := func(t *testing.T, creator string, archived bool) *v2HandlerFixture {
		fx := newV2HandlerFixture(t)
		provenance := mock_apicore.NewMockObjectProvenance(t)
		provenance.EXPECT().CreatorProvenance(mock.Anything, "space1", "obj1").
			Return(apicore.Provenance{AccountMatch: true, IntegrationName: creator}, nil).Maybe()
		fx.svc = v2service.NewService(fx.mwMock, fx.readerMock, fx.creatorMock, mock_apicore.NewMockObjectMutator(t), provenance,
			fx.widgetsMock, nil, nil, nil, nil, fx.fileMock, nil, fx.store, objectstore.TestTechSpaceId, testAccountId)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "obj1").
			Return(apicore.ObjectRead{SbType: model.SmartBlockType_Page, Snapshot: &model.SmartBlockSnapshotBase{}}, nil).Maybe()
		fx.readerMock.EXPECT().InBin(mock.Anything, "space1", "obj1").Return(archived, nil).Maybe()
		// the caller is the key "Claude Desktop": with that creator, a
		// permanent delete is allowed, the one shape in which a followed
		// reference could destroy the object
		fx.router.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(domain.CtxWithIntegrationName(c.Request.Context(), "Claude Desktop"))
			c.Next()
		})
		fx.router.DELETE("/v2/spaces/:space_id/objects/:object_id", withDryRunFlag(), DeleteObjectHandler(fx.svc))
		fx.router.POST("/v2/spaces/:space_id/objects/:object_id/restore", withDryRunFlag(), RestoreObjectHandler(fx.svc))
		return fx
	}
	serve := func(fx *v2HandlerFixture, method, target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, httptest.NewRequest(method, target, nil))
		return w
	}
	// follow sends what a REST caller would: a resend's query in place of the
	// request's, or the whole request a bound reference renders
	follow := func(t *testing.T, fx *v2HandlerFixture, method string, ref v2model.Ref) *httptest.ResponseRecorder {
		t.Helper()
		target := path + ref.String()
		if ref.Op != "" {
			var route string
			var ok bool
			method, route, ok = strings.Cut(ref.String(), " ")
			require.True(t, ok)
			target = route
		}
		return serve(fx, method, target)
	}
	decode := func(t *testing.T, w *httptest.ResponseRecorder, into any) {
		t.Helper()
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), into), w.Body.String())
	}

	for _, archived := range []bool{false, true} {
		t.Run(fmt.Sprintf("every reference of an archive preview, archived=%v", archived), func(t *testing.T) {
			// given
			fx := newFx(t, "Claude Desktop", archived)
			w := serve(fx, http.MethodDelete, path+"?dry_run=true")
			require.Equal(t, http.StatusOK, w.Code)
			var preview v2model.CreateResult
			decode(t, w, &preview)
			require.Len(t, preview.Warnings, 1)
			refs := preview.Warnings[0].SeeAlso
			require.Len(t, refs, 2, "the restore and the delete for good")

			for _, ref := range refs {
				// when
				got := follow(t, fx, http.MethodDelete, ref)

				// then
				require.Equal(t, http.StatusOK, got.Code, "%s: %s", ref, got.Body.String())
				var result v2model.CreateResult
				decode(t, got, &result)
				assert.True(t, result.DryRun, "%s: a preview's reference previews", ref)
			}
		})
	}

	t.Run("the skip_bin preview's repairs", func(t *testing.T) {
		for _, query := range []string{"?skip_bin=true&dry_run=true", "?skip_bin=false&dry_run=true"} {
			// given
			fx := newFx(t, "Claude Desktop", false)
			w := serve(fx, http.MethodDelete, path+query)
			require.Equal(t, http.StatusBadRequest, w.Code)
			var refusal v2model.Error
			decode(t, w, &refusal)
			require.Len(t, refusal.Issues, 1)
			require.Len(t, refusal.Issues[0].SeeAlso, 1)

			// when
			got := follow(t, fx, http.MethodDelete, refusal.Issues[0].SeeAlso[0])

			// then
			require.Equal(t, http.StatusOK, got.Code, "%s: %s", query, got.Body.String())
			var result v2model.CreateResult
			decode(t, got, &result)
			assert.True(t, result.DryRun, "%s: a preview's repair previews", query)
		}
	})

	t.Run("the ownership probe probes the operation it refused", func(t *testing.T) {
		// given: another key created the object, so a permanent delete is
		// refused; the probe it advertises must reach the same refusal, not
		// an archive preview
		fx := newFx(t, "Linear", false)
		w := serve(fx, http.MethodDelete, path+"?permanent=true")
		require.Equal(t, http.StatusForbidden, w.Code)
		var refusal v2model.Error
		decode(t, w, &refusal)
		require.Len(t, refusal.Issues, 1)
		require.Len(t, refusal.Issues[0].SeeAlso, 1)

		// when
		got := follow(t, fx, http.MethodDelete, refusal.Issues[0].SeeAlso[0])

		// then
		require.Equal(t, http.StatusForbidden, got.Code, got.Body.String())
		var again v2model.Error
		decode(t, got, &again)
		assert.Equal(t, refusal.Code, again.Code)
	})
}
