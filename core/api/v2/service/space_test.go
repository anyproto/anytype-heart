package v2service

// space_test.go pins the Phase-7 space surface (APIV2_SURFACES.md §2):
// the RPC-free get-one read, the single-RPC create (description rides
// WorkspaceCreate — no WorkspaceSetInfo follow-up), the at-least-one-field
// PATCH contract, and the C9 dry runs that send nothing.

import (
	"context"
	"encoding/json"
	"net/http"
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

// registerSpaceView registers a spaceView carrying name and description for
// spaceId (the fields the space-view sync mirrors from the workspace object).
func (fx *v2Fixture) registerSpaceView(t *testing.T, spaceId, name, description string) {
	fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("spaceView_" + spaceId),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
		bundle.RelationKeyName:           domain.String(name),
		bundle.RelationKeyDescription:    domain.String(description),
	}})
}

func TestV2GetSpace(t *testing.T) {
	t.Run("row from the space view — zero RPCs", func(t *testing.T) {
		// given: NO mock expectations — a WorkspaceOpen or ObjectShow (the v1
		// N+1 shape) would fail the test
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, "spaceS", "Work", "The local-first wiki")
		want := v2model.Space{Id: "spaceS", Name: "Work", Description: "The local-first wiki"}

		// when
		got, err := fx.GetSpace(context.Background(), "spaceS")

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("unknown space is 404 with steering", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)

		// when
		_, err := fx.GetSpace(context.Background(), "bogus")

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, "GET /v2/spaces")
	})

	t.Run("a deleted space is 404, not a live-looking row", func(t *testing.T) {
		// given: v1's GetSpace filters both status axes; without the filter
		// the row is indistinguishable from a live space and an agent picking
		// it would PATCH or write into a space that can never load
		fx := newV2FixtureBare(t)
		fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                 domain.String("spaceView_dead"),
			bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:      domain.String("deadSpace"),
			bundle.RelationKeyName:               domain.String("Deleted space"),
			bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(model.SpaceStatus_SpaceDeleted)),
			bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(model.SpaceStatus_Missing)),
		}})

		// when
		_, err := fx.GetSpace(context.Background(), "deadSpace")

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, "not available")
	})
}

func TestV2CreateSpace(t *testing.T) {
	t.Run("one WorkspaceCreate call carries name AND description", func(t *testing.T) {
		// given: no WorkspaceSetInfo expectation — v1 spends a second RPC on
		// the description; v2 folds it into the create (the mock fails on any
		// unexpected call)
		fx := newV2FixtureBare(t)
		fx.mwMock.EXPECT().WorkspaceCreate(mock.Anything, mock.MatchedBy(func(req *pb.RpcWorkspaceCreateRequest) bool {
			fields := req.Details.GetFields()
			return fields[bundle.RelationKeyName.String()].GetStringValue() == "Research" &&
				fields[bundle.RelationKeyDescription.String()].GetStringValue() == "Scratch space" &&
				int64(fields[bundle.RelationKeySpaceType.String()].GetNumberValue()) == int64(model.SpaceType_SpaceTypeRegular)
		})).Return(&pb.RpcWorkspaceCreateResponse{SpaceId: "newSpace1"})
		want := &v2model.Space{Id: "newSpace1", Name: "Research", Description: "Scratch space"}

		// when
		got, err := fx.CreateSpace(context.Background(), v2model.CreateSpaceRequest{Name: "Research", Description: "Scratch space"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("empty name is a path-addressed 400", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)

		// when
		_, err := fx.CreateSpace(context.Background(), v2model.CreateSpaceRequest{Name: "   "}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/name", apiErr.Issues[0].Path)
	})

	t.Run("dry run validates only and sends nothing (C9)", func(t *testing.T) {
		// given: any RPC fails the mock — a dry-run space create must not
		// create a space
		fx := newV2FixtureBare(t)
		want := &v2model.Space{Name: "Research", DryRun: true}

		// when
		got, err := fx.CreateSpace(context.Background(), v2model.CreateSpaceRequest{Name: "Research"}, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("RPC BAD_INPUT maps to 400, other codes to 500", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			code       pb.RpcWorkspaceCreateResponseErrorCode
			wantStatus int
		}{
			{"bad input", pb.RpcWorkspaceCreateResponseError_BAD_INPUT, http.StatusBadRequest},
			{"unknown error", pb.RpcWorkspaceCreateResponseError_UNKNOWN_ERROR, http.StatusInternalServerError},
		} {
			t.Run(tc.name, func(t *testing.T) {
				// given
				fx := newV2FixtureBare(t)
				fx.mwMock.EXPECT().WorkspaceCreate(mock.Anything, mock.Anything).Return(&pb.RpcWorkspaceCreateResponse{
					Error: &pb.RpcWorkspaceCreateResponseError{Code: tc.code, Description: "boom"},
				})

				// when
				_, err := fx.CreateSpace(context.Background(), v2model.CreateSpaceRequest{Name: "X"}, false)

				// then
				apiErr := v2Err(t, err)
				assert.Equal(t, tc.wantStatus, apiErr.Status)
			})
		}
	})

	t.Run("a success without a space id is a 500, never a 201 the agent cannot act on", func(t *testing.T) {
		// given: C8 states the response always returns created ids — and a
		// cached id-less 201 would replay forever under a keyed retry, while a
		// 500 is not cached and the retry re-executes
		fx := newV2FixtureBare(t)
		fx.mwMock.EXPECT().WorkspaceCreate(mock.Anything, mock.Anything).Return(&pb.RpcWorkspaceCreateResponse{})

		// when
		_, err := fx.CreateSpace(context.Background(), v2model.CreateSpaceRequest{Name: "X"}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusInternalServerError, apiErr.Status)
		assert.Contains(t, apiErr.Message, "no space id")
	})

	t.Run("an over-long name or description is a path-addressed 400 (the advertised cap)", func(t *testing.T) {
		// given: the space kind advertises maxLength 4096 — the schema must
		// not out-promise the endpoint (no RPC expectations: validation only)
		fx := newV2FixtureBare(t)
		long := strings.Repeat("x", maxSpaceFieldLength+1)

		// when / then
		_, err := fx.CreateSpace(context.Background(), v2model.CreateSpaceRequest{Name: long}, false)
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/name", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "4096")

		_, err = fx.CreateSpace(context.Background(), v2model.CreateSpaceRequest{Name: "ok", Description: long}, false)
		apiErr = v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/description", apiErr.Issues[0].Path)
	})

	t.Run("the space kind's advertised maxLength matches the enforced cap", func(t *testing.T) {
		// pin the schema and the endpoint together — the cap exists in two
		// places and silent drift would out-promise one of them
		var schema struct {
			Properties map[string]struct {
				MaxLength int `json:"maxLength"`
			} `json:"properties"`
		}
		require.NoError(t, json.Unmarshal([]byte(v2SchemaKinds["space"].schema), &schema))
		assert.Equal(t, maxSpaceFieldLength, schema.Properties["name"].MaxLength)
		assert.Equal(t, maxSpaceFieldLength, schema.Properties["description"].MaxLength)
	})
}

func TestV2UpdateSpace(t *testing.T) {
	name := func(s string) *string { return &s }

	t.Run("patching the name keeps the stored description", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, "spaceS", "Old name", "Keep me")
		fx.mwMock.EXPECT().WorkspaceSetInfo(mock.Anything, mock.MatchedBy(func(req *pb.RpcWorkspaceSetInfoRequest) bool {
			fields := req.Details.GetFields()
			_, hasDescription := fields[bundle.RelationKeyDescription.String()]
			return req.SpaceId == "spaceS" &&
				fields[bundle.RelationKeyName.String()].GetStringValue() == "New name" &&
				!hasDescription // omitted fields must not ride the RPC
		})).Return(&pb.RpcWorkspaceSetInfoResponse{})
		want := &v2model.Space{Id: "spaceS", Name: "New name", Description: "Keep me"}

		// when
		got, err := fx.UpdateSpace(context.Background(), "spaceS", v2model.UpdateSpaceRequest{Name: name("New name")}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("patching the description alone works and may clear it", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, "spaceS", "Work", "Old description")
		fx.mwMock.EXPECT().WorkspaceSetInfo(mock.Anything, mock.MatchedBy(func(req *pb.RpcWorkspaceSetInfoRequest) bool {
			fields := req.Details.GetFields()
			v, hasDescription := fields[bundle.RelationKeyDescription.String()]
			_, hasName := fields[bundle.RelationKeyName.String()]
			return hasDescription && v.GetStringValue() == "" && !hasName
		})).Return(&pb.RpcWorkspaceSetInfoResponse{})
		want := &v2model.Space{Id: "spaceS", Name: "Work"}

		// when
		got, err := fx.UpdateSpace(context.Background(), "spaceS", v2model.UpdateSpaceRequest{Description: name("")}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("an empty update is a 400, not a silent no-op", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, "spaceS", "Work", "")

		// when
		_, err := fx.UpdateSpace(context.Background(), "spaceS", v2model.UpdateSpaceRequest{}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Contains(t, apiErr.Message, "at least one")
	})

	t.Run("an empty name is rejected — omit it to keep the current one", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, "spaceS", "Work", "")

		// when
		_, err := fx.UpdateSpace(context.Background(), "spaceS", v2model.UpdateSpaceRequest{Name: name(" ")}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/name", apiErr.Issues[0].Path)
	})

	t.Run("unknown space 404s before validation", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)

		// when: even an invalid (empty) update reports the missing space first
		_, err := fx.UpdateSpace(context.Background(), "bogus", v2model.UpdateSpaceRequest{}, false)

		// then
		assert.Equal(t, http.StatusNotFound, v2Err(t, err).Status)
	})

	t.Run("dry run reports the would-be row and sends nothing (C9)", func(t *testing.T) {
		// given: any RPC fails the mock
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, "spaceS", "Old", "Keep me")
		want := &v2model.Space{Id: "spaceS", Name: "New", Description: "Keep me", DryRun: true}

		// when
		got, err := fx.UpdateSpace(context.Background(), "spaceS", v2model.UpdateSpaceRequest{Name: name("New")}, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("an over-long name is a path-addressed 400 before any RPC", func(t *testing.T) {
		// given: no RPC expectations — validation must fire first
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, "spaceS", "Work", "")
		long := strings.Repeat("x", maxSpaceFieldLength+1)

		// when
		_, err := fx.UpdateSpace(context.Background(), "spaceS", v2model.UpdateSpaceRequest{Name: name(long)}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/name", apiErr.Issues[0].Path)
	})

	t.Run("workspace RPC failures classify on the description, not a blanket 500", func(t *testing.T) {
		// The workspace RPCs answer UNKNOWN_ERROR for everything reachable
		// (core mapErrorCode has no workspace mappings), so without the
		// description classification a PATCH on a space the account left, or
		// by a reader in a shared space, would 500 and retry-loop — the class
		// of defect Phase 6 fixed for chats.
		for _, tc := range []struct {
			name        string
			description string
			wantStatus  int
			wantCode    string
		}{
			{"space not exists is 404", "space not exists", http.StatusNotFound, v2model.CodeNotFound},
			{"space is deleted is 404", "space is deleted", http.StatusNotFound, v2model.CodeNotFound},
			{"restricted is 403", "restricted", http.StatusForbidden, v2model.CodeForbidden},
			{"anything else stays 500", "disk exploded", http.StatusInternalServerError, v2model.CodeInternalError},
		} {
			t.Run(tc.name, func(t *testing.T) {
				// given: the view exists (the GetSpace precheck passes) but the
				// workspace RPC fails — the race window the precheck cannot close
				fx := newV2FixtureBare(t)
				fx.registerSpaceView(t, "spaceS", "Work", "")
				fx.mwMock.EXPECT().WorkspaceSetInfo(mock.Anything, mock.Anything).Return(&pb.RpcWorkspaceSetInfoResponse{
					Error: &pb.RpcWorkspaceSetInfoResponseError{
						Code:        pb.RpcWorkspaceSetInfoResponseError_UNKNOWN_ERROR,
						Description: tc.description,
					},
				})

				// when
				_, err := fx.UpdateSpace(context.Background(), "spaceS", v2model.UpdateSpaceRequest{Name: name("New")}, false)

				// then
				apiErr := v2Err(t, err)
				assert.Equal(t, tc.wantStatus, apiErr.Status)
				assert.Equal(t, tc.wantCode, apiErr.Code)
				assert.Contains(t, apiErr.Message, tc.description)
			})
		}
	})
}
