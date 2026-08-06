package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// addSelectProperty registers a select property "severity" with one existing
// option "High" in the test space.
func (fx *v2Fixture) addSelectProperty(t *testing.T) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("rel-severity"),
			bundle.RelationKeyRelationKey:    domain.String("severity"),
			bundle.RelationKeyName:           domain.String("Severity"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		},
		{
			bundle.RelationKeyId:             domain.String("opt-high"),
			bundle.RelationKeyRelationKey:    domain.String("severity"),
			bundle.RelationKeyName:           domain.String("High"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		},
	})
}

// addTagProperty registers a multiSelect property "tags" with two existing
// options "Urgent" and "Later" in the test space.
func (fx *v2Fixture) addTagProperty(t *testing.T) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("rel-tags"),
			bundle.RelationKeyRelationKey:    domain.String("tags"),
			bundle.RelationKeyName:           domain.String("Tags"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		},
		{
			bundle.RelationKeyId:             domain.String("opt-urgent"),
			bundle.RelationKeyRelationKey:    domain.String("tags"),
			bundle.RelationKeyName:           domain.String("Urgent"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		},
		{
			bundle.RelationKeyId:             domain.String("opt-later"),
			bundle.RelationKeyRelationKey:    domain.String("tags"),
			bundle.RelationKeyName:           domain.String("Later"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		},
	})
}

// expectCreate captures the snapshot handed to the creator and returns id.
func (fx *v2Fixture) expectCreate(id string) **model.SmartBlockSnapshotBase {
	var captured *model.SmartBlockSnapshotBase
	fx.creatorMock.EXPECT().CreateObjectFromSnapshot(mock.Anything, testSpaceId, mock.Anything).
		RunAndReturn(func(ctx context.Context, spaceId string, snapshot *model.SmartBlockSnapshotBase) (string, error) {
			captured = snapshot
			return id, nil
		})
	return &captured
}

func (fx *v2Fixture) expectEtagRead(objectId string) {
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, objectId).
		Return(apicore.ObjectRead{Heads: []string{"headX"}}, nil)
}

func v2Err(t *testing.T, err error) *apimodel.V2Error {
	t.Helper()
	var apiErr *apimodel.V2Error
	require.ErrorAs(t, err, &apiErr)
	return apiErr
}

func TestV2CreateObjectShortcut(t *testing.T) {
	t.Run("shortcut creates a typed object with name", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"task","name":"Buy milk"}`), false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "newObj", result.Id)
		assert.Equal(t, "task", result.Type)
		assert.Equal(t, ComputeEtag([]string{"headX"}), result.Etag)
		require.NotNil(t, *captured)
		snapshot := *captured
		assert.Equal(t, []string{"ot-task"}, snapshot.ObjectTypes)
		assert.Equal(t, "Buy milk", pbtypes.GetString(snapshot.Details, "name"))
	})

	t.Run("markdown is parsed into the create snapshot (one change set)", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when — no BlockCreate/BlockPaste expectations: the paste path is gone
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Doc","markdown":"# Hello\n\n- [ ] first task"}`), false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "newObj", result.Id)
		snapshot := *captured
		require.NotNil(t, snapshot)
		var heading, checkbox *model.Block
		for _, b := range snapshot.Blocks {
			if text := b.GetText(); text != nil {
				switch {
				case text.Style == model.BlockContentText_Header1 && text.Text == "Hello":
					heading = b
				case text.Style == model.BlockContentText_Checkbox && text.Text == "first task":
					checkbox = b
				}
			}
		}
		require.NotNil(t, heading, "markdown heading must be part of the create snapshot")
		require.NotNil(t, checkbox, "markdown checkbox must be part of the create snapshot")
	})

	t.Run("dry run validates the markdown body too", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Doc","markdown":"- [x] done item"}`), true)

		// then
		require.NoError(t, err)
		assert.True(t, result.DryRun)
		assert.Empty(t, result.Id)
		assert.Empty(t, result.Warnings, "the two-change-set dry-run caveat warning is gone")
	})

	t.Run("whitespace-only markdown is rejected, not a silent empty create", func(t *testing.T) {
		// given: no create expectation — nothing may reach the creator
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"page","name":"Doc","markdown":"   \n\t\n"}`), false)

		// then: the same contract as the insertBlocks markdown channel (C6)
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeValidationFailed, apiErr.Code)
		assert.Equal(t, "markdown produced no blocks", apiErr.Message)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/markdown", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "give at least one non-blank line")
	})

	t.Run("markdown over the create block cap is rejected with the limit", func(t *testing.T) {
		// given: 3 bytes per block would reach ~350k blocks in 1 MiB without
		// the parsed-run cap; no create expectation — nothing may be built
		fx := newV2Fixture(t)
		body, err := json.Marshal(map[string]any{
			"type": "page", "name": "Doc",
			"markdown": strings.Repeat("- x\n", v2MaxCreateMarkdownBlocks+1),
		})
		require.NoError(t, err)

		// when
		_, err = fx.CreateObject(context.Background(), testSpaceId, body, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, "markdown produced too many blocks", apiErr.Message)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/markdown", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "2048")
		assert.Contains(t, apiErr.Issues[0].Message, "insertBlocks")
	})

	t.Run("markdown-derived issue paths readdress /blocks to /markdown", func(t *testing.T) {
		// the caller sent markdown, never a blocks array — a /blocks path into
		// the synthesized document is unactionable (C6)
		err := rebaseMarkdownCreateError(apimodel.V2ValidationFailed("the document failed AnyBlock validation",
			apimodel.V2Issue{Path: "/blocks/1", Message: "nested under a divider block"},
			apimodel.V2Issue{Path: "/blocks/3/text", Message: "too long"},
			apimodel.V2Issue{Path: "/type", Message: "untouched"}))
		apiErr := v2Err(t, err)
		assert.Equal(t, "/markdown[1]", apiErr.Issues[0].Path)
		assert.Equal(t, "/markdown[3]/text", apiErr.Issues[1].Path)
		assert.Equal(t, "/type", apiErr.Issues[2].Path)
	})

	t.Run("unknown shortcut key steers to the full document", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"task","title":"oops"}`), false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/title", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, `"version": 1`)
	})

	t.Run("missing type is rejected", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId, []byte(`{"name":"x"}`), false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/type", apiErr.Issues[0].Path)
	})
}

func TestV2CreateObjectDocument(t *testing.T) {
	t.Run("full document creates with blocks", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"type":"page","properties":{"name":"Doc"},"blocks":[{"type":"paragraph","text":"hi"}]}`), false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "newObj", result.Id)
		snapshot := *captured
		require.NotNil(t, snapshot)
		require.Len(t, snapshot.Blocks, 2, "root + paragraph")
		assert.NotNil(t, snapshot.Blocks[0].GetSmartblock())
		assert.Equal(t, "hi", snapshot.Blocks[1].GetText().GetText())
	})

	t.Run("absent type defaults to page", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"blocks":[{"type":"paragraph","text":"hi"}]}`), false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "page", result.Type)
		assert.Equal(t, []string{"ot-page"}, (*captured).ObjectTypes)
	})

	t.Run("unknown type key gets a did-you-mean 400", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("type-recipe"),
			bundle.RelationKeyName:           domain.String("Recipe"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-recipe"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}})

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"type":"recipes","blocks":[]}`), false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/type", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "recipe")
		assert.Contains(t, apiErr.Issues[0].Hint, "recipe")
	})

	t.Run("unknown property key is rejected, never silently created", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"type":"page","properties":{"name":"ok","madeUpProp":"x"}}`), false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/properties/madeUpProp", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "POST /v2/spaces/"+testSpaceId+"/properties")
	})

	t.Run("structurally invalid document returns path-addressed issues", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"type":"page","blocks":[{"type":"wat"}]}`), false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Path, "/blocks/0")
	})

	t.Run("newer format version is version_unsupported", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":2,"type":"page"}`), false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeVersionUnsupported, apiErr.Code)
		assert.Contains(t, apiErr.Message, "document version 2")
		assert.Contains(t, apiErr.Message, "supported version 1")
	})

	t.Run("restricted type cannot be created", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"type":"participant"}`), false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeValidationFailed, apiErr.Code)
	})

	t.Run("objectType kind steers to POST types", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"kind":"objectType","key":"thing","typeProperties":[]}`), false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Hint, "/types")
	})

	t.Run("items on a non-collection document is rejected", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"type":"page","items":["obj1"]}`), false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/items", apiErr.Issues[0].Path)
	})

	t.Run("unknown select option names are created (SPEC §3)", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")
		fx.mwMock.EXPECT().ObjectCreateRelationOption(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectCreateRelationOptionRequest) bool {
			return req.SpaceId == testSpaceId &&
				pbtypes.GetString(req.Details, bundle.RelationKeyRelationKey.String()) == "severity" &&
				pbtypes.GetString(req.Details, bundle.RelationKeyName.String()) == "Blocker"
		})).Return(&pb.RpcObjectCreateRelationOptionResponse{
			ObjectId: "opt-blocker",
			Error:    &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
		})

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"type":"task","properties":{"severity":["High","Blocker"]}}`), false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Created)
		assert.Equal(t, []apimodel.V2CreatedOption{{Property: "severity", Name: "Blocker"}}, result.Created.Options)
		values := pbtypes.GetStringList((*captured).Details, "severity")
		assert.Equal(t, []string{"opt-high", "opt-blocker"}, values, "existing option resolved, missing one created")
	})

	t.Run("dry run creates nothing and reports would-be side effects", func(t *testing.T) {
		// given: no creator or RPC expectations — any call would fail the test
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"version":1,"type":"task","properties":{"severity":["Blocker"]}}`), true)

		// then
		require.NoError(t, err)
		assert.True(t, result.DryRun)
		assert.Empty(t, result.Id)
		require.NotNil(t, result.Created)
		assert.Equal(t, []apimodel.V2CreatedOption{{Property: "severity", Name: "Blocker"}}, result.Created.Options)
	})
}

func TestV2CreateTemplate(t *testing.T) {
	t.Run("template resolves templateFor into targetObjectType", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectCreate("newTemplate")
		fx.expectEtagRead("newTemplate")
		fx.creatorMock.EXPECT().TypeIdByKey(mock.Anything, testSpaceId, domain.TypeKey("task")).
			Return("taskTypeId", nil)

		// when
		result, err := fx.CreateTemplate(context.Background(), testSpaceId,
			[]byte(`{"version":1,"type":"template","templateFor":"task","properties":{"name":"Weekly"}}`), false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "newTemplate", result.Id)
		snapshot := *captured
		assert.Equal(t, []string{"ot-template", "ot-task"}, snapshot.ObjectTypes)
		assert.Equal(t, "taskTypeId", pbtypes.GetString(snapshot.Details, bundle.RelationKeyTargetObjectType.String()))
	})

	t.Run("templateFor is required", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateTemplate(context.Background(), testSpaceId,
			[]byte(`{"version":1,"type":"template","properties":{"name":"Weekly"}}`), false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/templateFor", apiErr.Issues[0].Path)
	})

	t.Run("unknown space is a 404", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateObject(context.Background(), "ghost", []byte(`{"type":"page"}`), false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
	})
}
