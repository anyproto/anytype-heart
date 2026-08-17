package v2service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// listReadRead builds one live ObjectRead for a set/collection target.
func listReadRead(layout model.ObjectTypeLayout, details map[string]*types.Value, dv *model.BlockContentDataview, members []string) apicore.ObjectRead {
	fields := map[string]*types.Value{
		bundle.RelationKeyResolvedLayout.String(): pbtypes.Int64(int64(layout)),
	}
	for k, v := range details {
		fields[k] = v
	}
	snapshot := &model.SmartBlockSnapshotBase{
		Details: &types.Struct{Fields: fields},
	}
	if dv != nil {
		snapshot.Blocks = []*model.Block{{
			Id:      dataviewBlockId,
			Content: &model.BlockContentOfDataview{Dataview: dv},
		}}
	}
	if members != nil {
		snapshot.Collections = &types.Struct{Fields: map[string]*types.Value{
			storeSliceKey: pbtypes.StringList(members),
		}}
	}
	return apicore.ObjectRead{Snapshot: snapshot, Heads: []string{"headL"}}
}

func setRead(dv *model.BlockContentDataview) apicore.ObjectRead {
	return listReadRead(model.ObjectType_set, map[string]*types.Value{
		bundle.RelationKeySetOf.String(): pbtypes.StringList([]string{"type-chore"}),
	}, dv, nil)
}

func collectionRead(dv *model.BlockContentDataview, members []string) apicore.ObjectRead {
	return listReadRead(model.ObjectType_collection, nil, dv, members)
}

func (fx *v2Fixture) expectListRead(objectId string, read apicore.ObjectRead) {
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, objectId).Return(read, nil)
}

func TestV2GetSetObjects(t *testing.T) {
	t.Run("a set executes its stored query directly against the store", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		fx.expectListRead("set1", setRead(nil))

		// when
		rows, total, hasMore, warnings, err := fx.GetSetObjects(context.Background(), testSpaceId, "set1", "", nil, 0, 25)

		// then: the two chores, newest-modified first; never the page
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.False(t, hasMore)
		assert.Empty(t, warnings)
		assert.Equal(t, []string{"chore2", "chore1"}, rowIds(rows))
	})

	t.Run("?view= applies the stored view's filters and sorts", func(t *testing.T) {
		// given: a view filtering to severity=opt-high (the stored id form)
		fx := searchSetup(t)
		dv := &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Id: "view1abc",
			Filters: []*model.BlockContentDataviewFilter{{
				RelationKey: "severity",
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList([]string{"opt-high"}),
			}},
		}}}
		fx.expectListRead("set1", setRead(dv))

		// when: the view resolves by unique suffix (C4 leniency)
		rows, total, _, _, err := fx.GetSetObjects(context.Background(), testSpaceId, "set1", "1abc", nil, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
		assert.Equal(t, "chore1", rows[0].Id)
	})

	t.Run("stored-view execution substitutes the current-user placeholder", func(t *testing.T) {
		// given: the fixture's chore1 is created by the caller, chore2 by
		// someone else (addChoreObjects)
		fx := searchSetup(t)
		dv := &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Id: "v1",
			Filters: []*model.BlockContentDataviewFilter{{
				RelationKey: bundle.RelationKeyCreator.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(filterTemplateUser),
			}},
		}}}
		fx.expectListRead("set1", setRead(dv))

		// when
		rows, _, _, warnings, err := fx.GetSetObjects(context.Background(), testSpaceId, "set1", "v1", nil, 0, 25)

		// then: the literal placeholder would match nothing — substitution
		// resolves it to the caller's participant id
		require.NoError(t, err)
		assert.Empty(t, warnings)
		assert.Equal(t, []string{"chore1"}, rowIds(rows))
	})

	t.Run("an unresolvable placeholder degrades to a warning, never a silent no-match", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		dv := &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Id: "v1",
			Filters: []*model.BlockContentDataviewFilter{{
				RelationKey: bundle.RelationKeyCreator.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("_filter_template_9_"),
			}},
		}}}
		fx.expectListRead("set1", setRead(dv))

		// when
		rows, _, _, warnings, err := fx.GetSetObjects(context.Background(), testSpaceId, "set1", "v1", nil, 0, 25)

		// then: the filter is dropped (both chores return) and the response warns
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"chore1", "chore2"}, rowIds(rows))
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, `"_filter_template_9_" is an unresolvable placeholder`)
		assert.Contains(t, warnings[0].Message, "ignored")
	})

	t.Run("unknown view is a 404 listing the view ids", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		dv := &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{Id: "v1"}, {Id: "v2"}}}
		fx.expectListRead("set1", setRead(dv))

		// when
		_, _, _, _, err := fx.GetSetObjects(context.Background(), testSpaceId, "set1", "ghost", nil, 0, 25)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeNotFound, apiErr.Code)
		assert.Contains(t, apiErr.Message, `view "ghost" not found`)
		assert.Contains(t, apiErr.Message, "v1, v2")
	})

	t.Run("a collection addressed through the sets route names the other route", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		fx.expectListRead("col1", collectionRead(nil, []string{"chore1"}))

		// when
		_, _, _, _, err := fx.GetSetObjects(context.Background(), testSpaceId, "col1", "", nil, 0, 25)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		assert.Contains(t, apiErr.Message, `object "col1" is a collection, not a set`)
		assert.Contains(t, apiErr.Message, "GET /v2/spaces/space1/collections/col1/objects")
	})

	t.Run("a set over a file type returns its rows and renders the file fields", func(t *testing.T) {
		// given: §8.8 claims the sets read never had the layout scope (so a
		// file set already worked) — previously verified only by reading
		// listObjects; this pins it, together with the mimeType/size alias
		// rendering on the ?fields= channel
		fx := searchSetup(t)
		fx.addImageObjects(t)
		fx.expectListRead("set1", listReadRead(model.ObjectType_set, map[string]*types.Value{
			bundle.RelationKeySetOf.String(): pbtypes.StringList([]string{"type-image"}),
		}, nil, nil))

		// when
		rows, total, _, _, err := fx.GetSetObjects(context.Background(), testSpaceId, "set1", "",
			[]string{"mimeType", "size"}, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
		assert.Equal(t, "img1", rows[0].Id)
		require.NotNil(t, rows[0].Properties)
		assert.Equal(t, "image/png", rows[0].Properties["mimeType"])
		assert.EqualValues(t, 12345, rows[0].Properties["size"])
	})

	t.Run("an empty setOf is an explicit error, not an unscoped query", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		fx.expectListRead("set1", listReadRead(model.ObjectType_set, nil, nil, nil))

		// when
		_, _, _, _, err := fx.GetSetObjects(context.Background(), testSpaceId, "set1", "", nil, 0, 25)

		// then
		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "queries nothing")
	})

	t.Run("a typoed fields key 400s with did-you-mean, like search does", func(t *testing.T) {
		// given: without the check the response is a 200 whose rows silently
		// carry no properties — indistinguishable from "no object has a value"
		fx := searchSetup(t)
		fx.expectListRead("set1", setRead(nil))

		// when
		_, _, _, _, err := fx.GetSetObjects(context.Background(), testSpaceId, "set1", "", []string{"sevirity"}, 0, 25)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "fields", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `unknown property key "sevirity"`)
		assert.Equal(t, "did you mean severity?", apiErr.Issues[0].Hint)
	})
}

func TestV2GetCollectionObjects(t *testing.T) {
	t.Run("a collection reads in its curated store-slice order", func(t *testing.T) {
		// given: membership order chore2 before chore1, plus a ghost id
		fx := searchSetup(t)
		fx.expectListRead("col1", collectionRead(nil, []string{"chore2", "chore1", "ghost"}))

		// when
		rows, total, hasMore, _, err := fx.GetCollectionObjects(context.Background(), testSpaceId, "col1", "", nil, 0, 25)

		// then: store order preserved; the dangling member is not a row
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.False(t, hasMore)
		assert.Equal(t, []string{"chore2", "chore1"}, rowIds(rows))
	})

	t.Run("an empty collection is an empty page, not an error", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		fx.expectListRead("col1", collectionRead(nil, nil))

		// when
		rows, total, hasMore, _, err := fx.GetCollectionObjects(context.Background(), testSpaceId, "col1", "", nil, 0, 25)

		// then
		require.NoError(t, err)
		assert.Empty(t, rows)
		assert.Equal(t, 0, total)
		assert.False(t, hasMore)
	})

	t.Run("a set addressed through the collections route names the other route", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		fx.expectListRead("set1", setRead(nil))

		// when
		_, _, _, _, err := fx.GetCollectionObjects(context.Background(), testSpaceId, "set1", "", nil, 0, 25)

		// then
		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, `object "set1" is a set, not a collection`)
		assert.Contains(t, apiErr.Message, "GET /v2/spaces/space1/sets/set1/objects")
	})

	t.Run("a plain object is neither — the error names both routes", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		fx.expectListRead("page1", listReadRead(model.ObjectType_basic, nil, nil, nil))

		// when
		_, _, _, _, err := fx.GetCollectionObjects(context.Background(), testSpaceId, "page1", "", nil, 0, 25)

		// then
		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "neither a set nor a collection")
		assert.Contains(t, apiErr.Message, "/sets/{setId}/objects")
		assert.Contains(t, apiErr.Message, "/collections/{collectionId}/objects")
	})

	t.Run("an offset past the membership is an empty page, has_more false", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		fx.expectListRead("col1", collectionRead(nil, []string{"chore2", "chore1"}))

		// when
		rows, total, hasMore, _, err := fx.GetCollectionObjects(context.Background(), testSpaceId, "col1", "", nil, 5, 25)

		// then
		require.NoError(t, err)
		assert.Empty(t, rows)
		assert.Equal(t, 2, total)
		assert.False(t, hasMore)
	})

	t.Run("a stored view's sorts override the membership order", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		dv := &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{
			Id: "v1",
			Sorts: []*model.BlockContentDataviewSort{{
				RelationKey: bundle.RelationKeyLastModifiedDate.String(),
				Type:        model.BlockContentDataviewSort_Asc,
				IncludeTime: true,
			}},
		}}}
		fx.expectListRead("col1", collectionRead(dv, []string{"chore2", "chore1"}))

		// when
		rows, _, _, _, err := fx.GetCollectionObjects(context.Background(), testSpaceId, "col1", "v1", nil, 0, 25)

		// then: oldest-modified first per the view, not membership order
		require.NoError(t, err)
		assert.Equal(t, []string{"chore1", "chore2"}, rowIds(rows))
	})
}

func TestV2ListViews(t *testing.T) {
	t.Run("views render as §6.2 view objects with option names", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		dv := &model.BlockContentDataview{
			RelationLinks: []*model.RelationLink{{Key: "severity", Format: model.RelationFormat_status}},
			Views: []*model.BlockContentDataviewView{{
				Id:   "v1",
				Name: "High only",
				Type: model.BlockContentDataviewView_Kanban,
				Filters: []*model.BlockContentDataviewFilter{{
					RelationKey: "severity",
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.StringList([]string{"opt-high"}),
				}},
				Sorts: []*model.BlockContentDataviewSort{{
					RelationKey: "severity",
					Type:        model.BlockContentDataviewSort_Desc,
				}},
			}},
		}
		fx.expectListRead("set1", setRead(dv))

		// when
		views, total, hasMore, err := fx.GetSetViews(context.Background(), testSpaceId, "set1", 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.False(t, hasMore)
		require.Len(t, views, 1)
		var view map[string]any
		require.NoError(t, json.Unmarshal(views[0], &view))
		assert.Equal(t, "v1", view["id"])
		assert.Equal(t, "High only", view["name"])
		assert.Equal(t, "kanban", view["type"], "the §6.2 lowerCamel view-type vocabulary")
		filters, _ := view["filters"].([]any)
		require.Len(t, filters, 1)
		leaf, _ := filters[0].(map[string]any)
		assert.Equal(t, "severity", leaf["property"])
		assert.Equal(t, "in", leaf["condition"])
		assert.Equal(t, []any{"High"}, leaf["value"], "option ids render as NAMES (C2)")
	})

	t.Run("an object without a dataview has zero views", func(t *testing.T) {
		// given
		fx := searchSetup(t)
		fx.expectListRead("col1", collectionRead(nil, []string{"chore1"}))

		// when
		views, total, hasMore, err := fx.GetCollectionViews(context.Background(), testSpaceId, "col1", 0, 25)

		// then
		require.NoError(t, err)
		assert.Empty(t, views)
		assert.Equal(t, 0, total)
		assert.False(t, hasMore)
	})
}

func TestV2SubstitutePlaceholders(t *testing.T) {
	placeholderFilter := func(value string) []*model.BlockContentDataviewFilter {
		return []*model.BlockContentDataviewFilter{{
			RelationKey: bundle.RelationKeyCreator.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(value),
		}}
	}

	t.Run("the host placeholder resolves to the hosting object id", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		out, warnings := fx.substitutePlaceholders(testSpaceId, "set1", placeholderFilter(filterTemplateHost))

		// then: resolved, not dropped-with-warning (the default placeholder arm)
		require.Empty(t, warnings)
		require.Len(t, out, 1)
		assert.Equal(t, "set1", out[0].Value.GetStringValue())
	})

	t.Run("an empty account identity degrades the user placeholder to a warning", func(t *testing.T) {
		// given: a service wired without an account identity
		fx := newV2Fixture(t)
		fx.Service.accountId = ""

		// when
		out, warnings := fx.substitutePlaceholders(testSpaceId, "set1", placeholderFilter(filterTemplateUser))

		// then: the leaf drops (evaluated literally it would match nothing)
		assert.Empty(t, out)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, `the current-user placeholder "_filter_template_2_" could not be resolved`)
		assert.Contains(t, warnings[0].Message, "ignored")
	})
}
