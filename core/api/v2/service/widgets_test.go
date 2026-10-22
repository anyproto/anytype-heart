package v2service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/anyproto/any-block/codec/anyblockjson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// addWidgetTarget registers an object of the given layout as a widget target.
func (fx *v2Fixture) addWidgetTarget(t *testing.T, id string, layout model.ObjectTypeLayout, extra ...objectstore.TestObject) {
	obj := objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyName:           domain.String("Object " + id),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(layout)),
	}
	for _, e := range extra {
		for k, v := range e {
			obj[k] = v
		}
	}
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})
}

func (fx *v2Fixture) allowWidgets(scope apicore.WidgetScope, allowed bool) {
	fx.widgetsMock.EXPECT().CanEditWidgets(mock.Anything, testSpaceId, scope).Return(allowed, nil).Maybe()
}

func (fx *v2Fixture) holdWidgets(scope apicore.WidgetScope, entries ...apicore.WidgetEntry) {
	fx.widgetsMock.EXPECT().ListWidgets(mock.Anything, testSpaceId, scope).Return(entries, nil).Maybe()
}

func widgetEntry(id, scope, target string, layout model.BlockContentWidgetLayout, limit int32) apicore.WidgetEntry {
	return apicore.WidgetEntry{Id: id, LinkId: id + "-link", Scope: apicore.WidgetScope(scope), Target: target, Layout: layout, Limit: limit}
}

func strPtr(v string) *string                                                    { return &v }
func layoutPtr(l model.BlockContentWidgetLayout) *model.BlockContentWidgetLayout { return &l }

// expectUpdate registers an UpdateWidget expectation that runs the service's
// plan against `current` (what the root holds under the lock) and asserts
// the update it yields, then answers `returned`.
func (fx *v2Fixture) expectUpdate(t *testing.T, scope apicore.WidgetScope, id string, current apicore.WidgetEntry, want apicore.WidgetUpdate, returned apicore.WidgetEntry) {
	t.Helper()
	fx.widgetsMock.EXPECT().UpdateWidget(mock.Anything, testSpaceId, scope, id, mock.Anything).RunAndReturn(
		func(_ context.Context, _ string, _ apicore.WidgetScope, _ string, plan func(apicore.WidgetEntry) (apicore.WidgetUpdate, error)) (apicore.WidgetEntry, error) {
			update, err := plan(current)
			require.NoError(t, err)
			assert.Equal(t, want, update)
			return returned, nil
		}).Once()
}

func TestWidgetLayoutNamesMatchTheCodec(t *testing.T) {
	// the wire layout vocabulary is the format's: every name this service
	// serves must be one the any-block codec reads into the same enum, so a
	// widget row copied into an index.json installs as the same widget
	for layout, name := range widgetLayoutNames {
		t.Run(name, func(t *testing.T) {
			// an id-shaped target: the codec's lift refuses a target that is
			// neither a listing nor an object id
			snapshot, err := anyblockjson.WidgetsSnapshot(&anyblockjson.Index{Widgets: []anyblockjson.Widget{{Target: "bafyreiamuhvd4f72swuxg6ejudiyfsinp56dkpr7crbnq3ulrdmvu7fryy", Layout: name}}})
			require.NoError(t, err)
			require.NotNil(t, snapshot)
			var found bool
			for _, block := range snapshot.Blocks {
				if content := block.GetWidget(); content != nil {
					assert.Equal(t, int32(layout), int32(content.Layout))
					found = true
				}
			}
			require.True(t, found, "the codec wrote no widget block for %q", name)
			// and back: the codec's own lift spells it the same way (it omits
			// the default, link)
			var idx anyblockjson.Index
			anyblockjson.IndexFromWidgetObject(&idx, snapshot)
			require.Len(t, idx.Widgets, 1)
			if layout == model.BlockContentWidget_Link {
				assert.Empty(t, idx.Widgets[0].Layout)
			} else {
				assert.Equal(t, name, idx.Widgets[0].Layout)
			}
			parsed, ok := parseWidgetLayout(name)
			require.True(t, ok)
			assert.Equal(t, layout, parsed)
		})
	}
	// the inventory is stated independently of the map under test, so a
	// dropped entry fails here rather than silently shrinking the loop
	assert.ElementsMatch(t, []string{"link", "tree", "list", "compact_list", "view"}, func() (names []string) {
		for _, name := range widgetLayoutNames {
			names = append(names, name)
		}
		return
	}())
	// and the listing spellings, both directions, for every stored listing
	for stored, served := range map[string]string{
		"favorite": "_favorite", "recent": "_recent", "recentOpen": "_recent_open", "set": "_set",
		"collection": "_collection", "allObjects": "_all_objects", "chat": "_chat", "bin": "_bin",
	} {
		assert.Equal(t, served, widgetRow(apicore.WidgetEntry{Target: stored, Layout: model.BlockContentWidget_Link}).Target, stored)
		assert.Len(t, matchWidgetRef([]apicore.WidgetEntry{{Id: "w", Target: stored}}, served), 1, served)
		assert.True(t, widget.IsPredefinedWidgetTargetId(stored), stored)
	}
}

func TestListWidgets(t *testing.T) {
	t.Run("both scopes in sidebar order, space first", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.holdWidgets(apicore.WidgetScopeSpace,
			widgetEntry("w1", "space", "favorite", model.BlockContentWidget_CompactList, 6),
			widgetEntry("w2", "space", "obj1", model.BlockContentWidget_Link, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal,
			widgetEntry("p1", "personal", "obj2", model.BlockContentWidget_Tree, 10))
		want := []v2model.WidgetRow{
			{Id: "w1", Scope: "space", Target: "_favorite", Layout: "compact_list", Limit: 6},
			{Id: "w2", Scope: "space", Target: "obj1", Layout: "link"},
			{Id: "p1", Scope: "personal", Target: "obj2", Layout: "tree", Limit: 10},
		}

		// when
		rows, total, hasMore, _, err := fx.ListWidgets(context.Background(), testSpaceId, "", 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
		assert.Equal(t, 3, total)
		assert.False(t, hasMore)
	})

	t.Run("a page crosses the root boundary in sidebar order", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "a", model.BlockContentWidget_Tree, 6), widgetEntry("w2", "space", "b", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal, widgetEntry("p1", "personal", "c", model.BlockContentWidget_Tree, 6))

		rows, total, hasMore, _, err := fx.ListWidgets(context.Background(), testSpaceId, "", 1, 1)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "w2", rows[0].Id)
		assert.Equal(t, 3, total)
		assert.True(t, hasMore)

		rows, _, hasMore, _, err = fx.ListWidgets(context.Background(), testSpaceId, "", 2, 5)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "p1", rows[0].Id)
		assert.False(t, hasMore)
	})

	t.Run("scope narrows to one root", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.holdWidgets(apicore.WidgetScopePersonal, widgetEntry("p1", "personal", "obj2", model.BlockContentWidget_Tree, 10))

		// when
		rows, _, _, _, err := fx.ListWidgets(context.Background(), testSpaceId, "personal", 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "p1", rows[0].Id)
	})

	t.Run("unknown scope is a 400", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, _, _, _, err := fx.ListWidgets(context.Background(), testSpaceId, "shared", 0, 25)
		require.Equal(t, http.StatusBadRequest, v2Err(t, err).Status)
	})

	t.Run("unknown space is a 404", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, _, _, _, err := fx.ListWidgets(context.Background(), "nowhere", "", 0, 25)
		require.Equal(t, http.StatusNotFound, v2Err(t, err).Status)
	})
}

func TestCreateWidget(t *testing.T) {
	t.Run("a page target defaults to tree with the smallest limit", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopePersonal, true)
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.widgetsMock.EXPECT().CreateWidget(mock.Anything, testSpaceId, apicore.WidgetScopePersonal,
			apicore.WidgetCreate{Target: "page1", Layout: model.BlockContentWidget_Tree, Limit: 6}).
			Return(widgetEntry("p1", "personal", "page1", model.BlockContentWidget_Tree, 6), nil)
		want := &v2model.WidgetResult{WidgetRow: v2model.WidgetRow{Id: "p1", Scope: "personal", Target: "page1", Layout: "tree", Limit: 6}}

		// when
		got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "personal"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("a query target defaults to view; a type target too", func(t *testing.T) {
		for _, layout := range []model.ObjectTypeLayout{model.ObjectType_set, model.ObjectType_collection, model.ObjectType_objectType} {
			fx := newV2Fixture(t)
			fx.addWidgetTarget(t, "list1", layout)
			fx.allowWidgets(apicore.WidgetScopeSpace, true)
			fx.holdWidgets(apicore.WidgetScopeSpace)

			got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "list1", Scope: "space"}, true)

			require.NoError(t, err)
			assert.Equal(t, "view", got.Layout, "layout %d", layout)
			assert.Equal(t, 6, got.Limit)
			assert.True(t, got.DryRun)
			assert.Empty(t, got.Id)
		}
	})

	t.Run("the list layout defaults to 4, and views resolve on every list-kind target", func(t *testing.T) {
		for name, layout := range map[string]model.ObjectTypeLayout{"query": model.ObjectType_set, "collection": model.ObjectType_collection, "type": model.ObjectType_objectType} {
			fx := newV2Fixture(t)
			fx.addWidgetTarget(t, "list1", layout)
			fx.allowWidgets(apicore.WidgetScopeSpace, true)
			fx.holdWidgets(apicore.WidgetScopeSpace)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "list1").Return(apicore.ObjectRead{
				Snapshot: &model.SmartBlockSnapshotBase{Blocks: []*model.Block{{Id: "dataview", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
					Views: []*model.BlockContentDataviewView{{Id: "view-one"}, {Id: "view-two"}},
				}}}}},
			}, nil).Maybe()
			fx.widgetsMock.EXPECT().CreateWidget(mock.Anything, testSpaceId, apicore.WidgetScopeSpace,
				apicore.WidgetCreate{Target: "list1", Layout: model.BlockContentWidget_List, Limit: 4, ViewId: "view-two"}).
				Return(apicore.WidgetEntry{Id: "w1", Scope: "space", Target: "list1", Layout: model.BlockContentWidget_List, Limit: 4, ViewId: "view-two"}, nil)
			got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "list1", Scope: "space", Layout: strPtr("list"), ViewId: "two"}, false)
			require.NoError(t, err, name)
			assert.Equal(t, 4, got.Limit, name)
			assert.Equal(t, "view-two", got.ViewId, name)
		}
	})

	t.Run("a file target is link only, and the sent limit is dropped with a warning", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "file1", model.ObjectType_file)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)

		// when
		got, err := fx.CreateWidget(context.Background(), testSpaceId,
			v2model.CreateWidgetRequest{Target: "file1", Scope: "space", Layout: strPtr("tree"), Limit: intPtr(10)}, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, "link", got.Layout)
		assert.Zero(t, got.Limit)
		require.Len(t, got.Warnings, 2)
		assert.Equal(t, "/layout", got.Warnings[0].Path)
		assert.Contains(t, got.Warnings[0].Message, "renders as link, not tree")
		assert.Equal(t, "/limit", got.Warnings[1].Path)
	})

	t.Run("an off-list limit is rounded to the smallest with a warning", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)

		// when
		got, err := fx.CreateWidget(context.Background(), testSpaceId,
			v2model.CreateWidgetRequest{Target: "set1", Scope: "space", Layout: strPtr("list"), Limit: intPtr(7)}, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, "list", got.Layout)
		assert.Equal(t, 4, got.Limit, "the list layout's own pick-list starts at 4")
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, "4, 6, 8, 30, 50")
	})

	t.Run("scope is required and must be known", func(t *testing.T) {
		fx := newV2Fixture(t)
		for _, scope := range []string{"", "shared"} {
			_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: scope}, true)
			e := v2Err(t, err)
			require.Equal(t, http.StatusBadRequest, e.Status)
			require.Len(t, e.Issues, 1)
			assert.Equal(t, "/scope", e.Issues[0].Path)
		}
	})

	t.Run("the space sidebar refuses a member who cannot manage the space", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, false)

		// when
		_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space"}, true)

		// then
		e := v2Err(t, err)
		require.Equal(t, http.StatusForbidden, e.Status)
		assert.Equal(t, v2model.CodeForbidden, e.Code)
		assert.Contains(t, e.Message, "scope personal")
	})

	t.Run("the personal sidebar refuses a member who cannot write", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopePersonal, false)
		for _, dry := range []bool{true, false} {
			_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "personal"}, dry)
			require.Equal(t, http.StatusForbidden, v2Err(t, err).Status)
		}
	})

	t.Run("a page of an ordinary type is accepted; only the template type refuses", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{bundle.RelationKeyId: domain.String("type-page"), bundle.RelationKeyUniqueKey: domain.String("ot-page")})
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic, objectstore.TestObject{bundle.RelationKeyType: domain.String("type-page")})
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space"}, true)
		require.NoError(t, err)
		assert.Equal(t, "tree", got.Layout)
	})

	t.Run("present-but-empty POST members are refused, as the schema says", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopePersonal, true)
		fx.holdWidgets(apicore.WidgetScopePersonal)
		for path, req := range map[string]v2model.CreateWidgetRequest{
			"/layout":   {Target: "page1", Scope: "personal", Layout: strPtr("")},
			"/after":    {Target: "page1", Scope: "personal", After: strPtr("")},
			"/before":   {Target: "page1", Scope: "personal", Before: strPtr("")},
			"/position": {Target: "page1", Scope: "personal", Position: strPtr("")},
		} {
			_, err := fx.CreateWidget(context.Background(), testSpaceId, req, false)
			e := v2Err(t, err)
			require.Equal(t, http.StatusBadRequest, e.Status, path)
			assert.Equal(t, path, e.Issues[0].Path)
		}
	})

	t.Run("the target refusals hold in the personal sidebar too", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopePersonal, true)
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.addWidgetTarget(t, "rel1", model.ObjectType_relation)
		fx.addWidgetTarget(t, "bin1", model.ObjectType_basic, objectstore.TestObject{bundle.RelationKeyIsArchived: domain.Bool(true)})
		for _, target := range []string{"rel1", "bin1", "_favorite"} {
			for _, dry := range []bool{true, false} {
				_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: target, Scope: "personal"}, dry)
				require.Equal(t, http.StatusBadRequest, v2Err(t, err).Status, target)
			}
		}
	})

	t.Run("cross-family limits are refused both ways", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		for _, layout := range []string{"view", "compact_list"} {
			for _, sent := range []int{4, 8} {
				got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "set1", Scope: "space", Layout: strPtr(layout), Limit: intPtr(sent)}, true)
				require.NoError(t, err)
				assert.Equal(t, 6, got.Limit, "%s %d", layout, sent)
			}
		}
	})

	t.Run("target refusals name the rule", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "dup1", model.BlockContentWidget_Tree, 6))
		fx.addWidgetTarget(t, "dup1", model.ObjectType_basic)
		fx.addWidgetTarget(t, "bin1", model.ObjectType_basic, objectstore.TestObject{bundle.RelationKeyIsArchived: domain.Bool(true)})
		fx.addWidgetTarget(t, "gone1", model.ObjectType_basic, objectstore.TestObject{bundle.RelationKeyIsDeleted: domain.Bool(true)})
		fx.addWidgetTarget(t, "rel1", model.ObjectType_relation)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:        domain.String("type-template"),
			bundle.RelationKeyUniqueKey: domain.String(bundle.TypeKeyTemplate.URL()),
		})
		fx.addWidgetTarget(t, "tpl1", model.ObjectType_basic, objectstore.TestObject{bundle.RelationKeyType: domain.String("type-template")})
		fx.addWidgetTarget(t, "tpl2", model.ObjectType_todo, objectstore.TestObject{bundle.RelationKeyType: domain.String("type-template")})

		for target, want := range map[string]string{
			"":          "target is required",
			"_favorite": "built-in listing",
			"favorite":  "built-in listing",
			"_unread":   "unknown built-in listing",
			"missing1":  "no object",
			"bin1":      "in the bin",
			"gone1":     "is deleted",
			"rel1":      "is a property",
			"tpl1":      "is a template",
			"tpl2":      "is a template",
			"dup1":      "already holds a widget",
		} {
			_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: target, Scope: "space"}, true)
			e := v2Err(t, err)
			require.Equal(t, http.StatusBadRequest, e.Status, target)
			assert.Contains(t, e.Message, want, target)
			require.Len(t, e.Issues, 1, target)
			assert.Equal(t, "/target", e.Issues[0].Path, target)
		}
	})

	t.Run("view_id resolves against the target's views, by suffix too", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "set1").Return(apicore.ObjectRead{
			Snapshot: &model.SmartBlockSnapshotBase{Blocks: []*model.Block{{Id: "dataview", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{Id: "view-aaa111"}, {Id: "view-bbb222"}},
			}}}}},
		}, nil)

		// when
		got, err := fx.CreateWidget(context.Background(), testSpaceId,
			v2model.CreateWidgetRequest{Target: "set1", Scope: "space", ViewId: "bbb222"}, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, "view-bbb222", got.ViewId)

		// and an unknown view names the ones there are
		_, err = fx.CreateWidget(context.Background(), testSpaceId,
			v2model.CreateWidgetRequest{Target: "set1", Scope: "space", ViewId: "nope"}, true)
		e := v2Err(t, err)
		require.Equal(t, http.StatusBadRequest, e.Status)
		assert.Equal(t, "/view_id", e.Issues[0].Path)
		assert.Contains(t, e.Issues[0].Message, "view-aaa111, view-bbb222")
	})

	t.Run("view_id is refused where the app never writes one", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.allowWidgets(apicore.WidgetScopePersonal, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		fx.holdWidgets(apicore.WidgetScopePersonal)

		_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space", ViewId: "v1"}, true)
		assert.Contains(t, v2Err(t, err).Message, "has no views")

		_, err = fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "set1", Scope: "personal", ViewId: "v1"}, true)
		assert.Contains(t, v2Err(t, err).Message, "personal widget keeps no view")
	})

	t.Run("placement names a sibling by target, and at most one member", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "favorite", model.BlockContentWidget_CompactList, 6))
		fx.widgetsMock.EXPECT().CreateWidget(mock.Anything, testSpaceId, apicore.WidgetScopeSpace,
			apicore.WidgetCreate{Target: "page1", Layout: model.BlockContentWidget_Tree, Limit: 6, Placement: apicore.WidgetPlacement{AfterId: "w1"}}).
			Return(widgetEntry("w2", "space", "page1", model.BlockContentWidget_Tree, 6), nil)

		// when
		got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space", After: strPtr("_favorite")}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "w2", got.Id)

		_, err = fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space", After: strPtr("w1"), Position: strPtr("first")}, true)
		assert.Equal(t, v2model.CodeAmbiguousInput, v2Err(t, err).Code)

		_, err = fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space", Before: strPtr("nope")}, true)
		assert.Equal(t, "/before", v2Err(t, err).Issues[0].Path)

		_, err = fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space", Position: strPtr("middle")}, true)
		assert.Equal(t, "/position", v2Err(t, err).Issues[0].Path)
	})

	t.Run("the editor's space lock surfaces as a 403", func(t *testing.T) {
		// given: the up-front check passed (a stale verdict) and Apply refused
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		fx.widgetsMock.EXPECT().CreateWidget(mock.Anything, testSpaceId, apicore.WidgetScopeSpace, mock.Anything).
			Return(apicore.WidgetEntry{}, restriction.ErrRestricted)

		// when
		_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space"}, false)

		// then
		require.Equal(t, http.StatusForbidden, v2Err(t, err).Status)
	})

	t.Run("the layout table is the desktop's, class by class", func(t *testing.T) {
		type row struct {
			layout  model.ObjectTypeLayout
			def     string
			allowed string
		}
		rows := map[string]row{
			"page":         {model.ObjectType_basic, "tree", "tree, link"},
			"profile":      {model.ObjectType_profile, "tree", "tree, link"},
			"todo":         {model.ObjectType_todo, "tree", "tree, link"},
			"note":         {model.ObjectType_note, "tree", "tree, link"},
			"bookmark":     {model.ObjectType_bookmark, "tree", "tree, link"},
			"tag":          {model.ObjectType_tag, "link", "tree, link"},
			"query":        {model.ObjectType_set, "view", "view, compact_list, list, link"},
			"collection":   {model.ObjectType_collection, "view", "view, compact_list, list, link"},
			"type":         {model.ObjectType_objectType, "view", "view, compact_list, list, link"},
			"file":         {model.ObjectType_file, "link", "link"},
			"image":        {model.ObjectType_image, "link", "link"},
			"participant":  {model.ObjectType_participant, "link", "link"},
			"date":         {model.ObjectType_date, "link", "link"},
			"chat":         {model.ObjectType_chatDerived, "link", "link"},
			"option":       {model.ObjectType_relationOption, "link", "link"},
			"audio":        {model.ObjectType_audio, "link", "link"},
			"video":        {model.ObjectType_video, "link", "link"},
			"pdf":          {model.ObjectType_pdf, "link", "link"},
			"dashboard":    {model.ObjectType_dashboard, "link", "link"},
			"space":        {model.ObjectType_space, "link", "link"},
			"space_view":   {model.ObjectType_spaceView, "link", "link"},
			"discussion":   {model.ObjectType_discussion, "link", "link"},
			"option_list":  {model.ObjectType_relationOptionsList, "link", "tree, link"},
			"notification": {model.ObjectType_notification, "link", "tree, link"},
			"devices":      {model.ObjectType_devices, "link", "tree, link"},
			"legacy_chat":  {model.ObjectType_chatDeprecated, "link", "tree, link"},
		}
		for name, r := range rows {
			t.Run(name, func(t *testing.T) {
				fx := newV2Fixture(t)
				fx.addWidgetTarget(t, "t1", r.layout)
				fx.allowWidgets(apicore.WidgetScopeSpace, true)
				fx.holdWidgets(apicore.WidgetScopeSpace)

				got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "t1", Scope: "space"}, true)
				require.NoError(t, err)
				assert.Equal(t, r.def, got.Layout, "default")

				// every layout of the set is accepted as sent; every other is
				// normalised to the set's first with a warning naming the set
				for _, name := range []string{"link", "tree", "list", "compact_list", "view"} {
					got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "t1", Scope: "space", Layout: strPtr(name)}, true)
					require.NoError(t, err)
					if strings.Contains(r.allowed, name) {
						assert.Equal(t, name, got.Layout, name)
						assert.Empty(t, got.Warnings, name)
					} else {
						assert.Equal(t, strings.Split(r.allowed, ", ")[0], got.Layout, name)
						require.Len(t, got.Warnings, 1, name)
						assert.Contains(t, got.Warnings[0].Message, "It takes "+r.allowed, name)
					}
				}
			})
		}
	})

	t.Run("the limit lists are the desktop's, value by value", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		for _, tc := range []struct {
			target, layout string
			list           []int
		}{{"set1", "view", []int{6, 10, 14, 30, 50}}, {"set1", "compact_list", []int{6, 10, 14, 30, 50}}, {"set1", "list", []int{4, 6, 8, 30, 50}}, {"page1", "tree", []int{6, 10, 14, 30, 50}}} {
			layout, list, target := tc.layout, tc.list, tc.target
			for _, value := range list {
				got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: target, Scope: "space", Layout: strPtr(layout), Limit: intPtr(value)}, true)
				require.NoError(t, err)
				assert.Equal(t, value, got.Limit, "%s %d", layout, value)
				assert.Empty(t, got.Warnings, "%s %d", layout, value)
			}
			for _, value := range []int{0, -1, 7, 4294967306, 4294967304, 1 << 40} {
				got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: target, Scope: "space", Layout: strPtr(layout), Limit: intPtr(value)}, true)
				require.NoError(t, err)
				assert.Equal(t, list[0], got.Limit, "%s %d rounds to the smallest, never to an int32 accident", layout, value)
				require.Len(t, got.Warnings, 1, "%s %d", layout, value)
			}
		}
	})

	t.Run("participant and date targets are real objects despite the underscore", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopePersonal, true)
		fx.holdWidgets(apicore.WidgetScopePersonal)
		participant := domain.NewParticipantId(testSpaceId, "identity1")
		fx.addWidgetTarget(t, participant, model.ObjectType_participant)

		got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: participant, Scope: "personal"}, true)
		require.NoError(t, err)
		assert.Equal(t, "link", got.Layout)
		assert.Equal(t, participant, got.Target, "a real id keeps its spelling")

		// a date has no store row: its details come from the id-based source
		// behind QueryByIds, which the fixture answers from AddVirtualDetails
		fx.objectStore.AddVirtualDetails("_date_2026-09-21", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String("_date_2026-09-21"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_date)),
			bundle.RelationKeySpaceId:        domain.String(testSpaceId),
		}))
		got, err = fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "_date_2026-09-21", Scope: "personal"}, true)
		require.NoError(t, err)
		assert.Equal(t, "link", got.Layout)
		// and a date the source does not know is a missing OBJECT, never a
		// misspelled listing
		_, err = fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "_date_1999-01-01", Scope: "personal"}, true)
		assert.Contains(t, v2Err(t, err).Message, "no object")

		// an unknown underscore id still reads as a misspelled listing, and
		// the personal sidebar itself is never a target
		_, err = fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "_unread", Scope: "personal"}, true)
		assert.Contains(t, v2Err(t, err).Message, "unknown built-in listing")
		_, err = fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: domain.NewPersonalWidgetsId(testSpaceId), Scope: "personal"}, true)
		assert.Contains(t, v2Err(t, err).Message, "is a sidebar, not an object")
	})

	t.Run("target refusals hold on a real write too, and every listing spelling is refused", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w0", "space", "other", model.BlockContentWidget_Tree, 6), widgetEntry("w1", "space", "dup1", model.BlockContentWidget_Tree, 6))
		fx.addWidgetTarget(t, "dup1", model.ObjectType_basic)
		fx.addWidgetTarget(t, "rel1", model.ObjectType_relation)
		fx.addWidgetTarget(t, "bin1", model.ObjectType_basic, objectstore.TestObject{bundle.RelationKeyIsArchived: domain.Bool(true)})
		fx.addWidgetTarget(t, "gone1", model.ObjectType_basic, objectstore.TestObject{bundle.RelationKeyIsDeleted: domain.Bool(true)})
		fx.addType(t, testSpaceId, objectstore.TestObject{bundle.RelationKeyId: domain.String("type-template"), bundle.RelationKeyUniqueKey: domain.String(bundle.TypeKeyTemplate.URL())})
		fx.addWidgetTarget(t, "tpl1", model.ObjectType_note, objectstore.TestObject{bundle.RelationKeyType: domain.String("type-template")})
		targets := []string{"", "rel1", "dup1", "missing1", "bin1", "gone1", "tpl1"}
		for _, stored := range []string{"favorite", "recent", "recentOpen", "set", "collection", "allObjects", "chat", "bin"} {
			targets = append(targets, stored, anyblockjson.FormatWidgetTarget(stored))
		}
		for _, target := range targets {
			// no CreateWidget expectation is registered: a call would fail the mock
			_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: target, Scope: "space"}, false)
			require.Equal(t, http.StatusBadRequest, v2Err(t, err).Status, target)
		}
	})

	t.Run("placement translates before, first and last; a foreign scope's widget is no anchor", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "other", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal, widgetEntry("p1", "personal", "mine", model.BlockContentWidget_Tree, 6))
		for _, tc := range []struct {
			req  v2model.CreateWidgetRequest
			want apicore.WidgetPlacement
		}{
			{v2model.CreateWidgetRequest{Before: strPtr("w1")}, apicore.WidgetPlacement{BeforeId: "w1"}},
			{v2model.CreateWidgetRequest{After: strPtr("other")}, apicore.WidgetPlacement{AfterId: "w1"}},
			{v2model.CreateWidgetRequest{Position: strPtr("first")}, apicore.WidgetPlacement{First: true}},
			{v2model.CreateWidgetRequest{Position: strPtr("last")}, apicore.WidgetPlacement{}},
			{v2model.CreateWidgetRequest{}, apicore.WidgetPlacement{}},
		} {
			tc.req.Target, tc.req.Scope = "page1", "space"
			fx.widgetsMock.EXPECT().CreateWidget(mock.Anything, testSpaceId, apicore.WidgetScopeSpace,
				apicore.WidgetCreate{Target: "page1", Layout: model.BlockContentWidget_Tree, Limit: 6, Placement: tc.want}).
				Return(widgetEntry("w2", "space", "page1", model.BlockContentWidget_Tree, 6), nil).Once()
			got, err := fx.CreateWidget(context.Background(), testSpaceId, tc.req, false)
			require.NoError(t, err, "%+v", tc.req)
			if tc.req.After == nil && tc.req.Before == nil && tc.req.Position == nil {
				assert.Nil(t, got.Placed)
			} else {
				assert.Equal(t, widgetPlaced(tc.want), got.Placed, "the committed create receipts the resolved placement")
			}
		}
		_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space", After: strPtr("p1")}, true)
		assert.Equal(t, "/after", v2Err(t, err).Issues[0].Path, "the personal widget is not a sibling of a space widget")
	})

	t.Run("a committed create is receipted as the adapter placed it", func(t *testing.T) {
		// given: the read saw no trailing bin; the adapter, under the lock,
		// found one and put the widget before it
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "a", model.BlockContentWidget_Tree, 6))
		applied := widgetEntry("w2", "space", "page1", model.BlockContentWidget_Tree, 6)
		applied.Placed = &apicore.WidgetPlacement{BeforeId: "wb"}
		fx.widgetsMock.EXPECT().CreateWidget(mock.Anything, testSpaceId, apicore.WidgetScopeSpace,
			apicore.WidgetCreate{Target: "page1", Layout: model.BlockContentWidget_Tree, Limit: 6}).Return(applied, nil)
		got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space", Position: strPtr("last")}, false)
		require.NoError(t, err)
		assert.Equal(t, &v2model.WidgetPlaced{Before: "wb"}, got.Placed)
	})

	t.Run("a duplicate that slipped past the pre-check is the same 400", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		fx.widgetsMock.EXPECT().CreateWidget(mock.Anything, testSpaceId, apicore.WidgetScopeSpace, mock.Anything).
			Return(apicore.WidgetEntry{}, apicore.ErrWidgetExists)
		_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space"}, false)
		e := v2Err(t, err)
		require.Equal(t, http.StatusBadRequest, e.Status)
		assert.Equal(t, "/target", e.Issues[0].Path)
	})

	t.Run("a committed create sends the normalised pair, not the requested one", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		fx.widgetsMock.EXPECT().CreateWidget(mock.Anything, testSpaceId, apicore.WidgetScopeSpace,
			apicore.WidgetCreate{Target: "page1", Layout: model.BlockContentWidget_Tree, Limit: 6}).
			Return(widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 6), nil)
		got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space", Layout: strPtr("list"), Limit: intPtr(7)}, false)
		require.NoError(t, err)
		require.Len(t, got.Warnings, 2)
		assert.Nil(t, got.Placed, "no placement asked, none receipted")
	})

	t.Run("limits from the other family are not accepted across lists", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		for _, tc := range []struct {
			layout string
			sent   int
			want   int
		}{{"list", 10, 4}, {"list", 14, 4}, {"view", 4, 6}, {"compact_list", 8, 6}} {
			got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "set1", Scope: "space", Layout: strPtr(tc.layout), Limit: intPtr(tc.sent)}, true)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Limit, "%s %d", tc.layout, tc.sent)
			require.Len(t, got.Warnings, 1)
		}
	})

	t.Run("a list-kind target without views refuses a view_id", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		for name, blocks := range map[string][]*model.Block{
			"no dataview": {{Id: "title", Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}},
			"no views":    {{Id: "dataview", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{}}}},
		} {
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "set1").Return(apicore.ObjectRead{Snapshot: &model.SmartBlockSnapshotBase{Blocks: blocks}}, nil).Once()
			_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "set1", Scope: "space", ViewId: "v1"}, false)
			e := v2Err(t, err)
			require.Equal(t, http.StatusBadRequest, e.Status, name)
			assert.Equal(t, "/view_id", e.Issues[0].Path, name)
		}
	})

	t.Run("a duplicate in the personal sidebar is refused on a preview too, the other root stays free", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopePersonal, true)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopePersonal, widgetEntry("p1", "personal", "page1", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopeSpace)
		_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "personal"}, true)
		assert.Contains(t, v2Err(t, err).Message, "already holds a widget")
		got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space"}, true)
		require.NoError(t, err)
		assert.Equal(t, "space", got.Scope)
	})

	t.Run("an anchor naming two legacy siblings is ambiguous; the id resolves it", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "twin", model.BlockContentWidget_Tree, 6), widgetEntry("w2", "space", "twin", model.BlockContentWidget_Tree, 6))
		_, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space", After: strPtr("twin")}, true)
		e := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, e.Code)
		assert.Equal(t, "/after", e.Issues[0].Path)
		got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space", After: strPtr("w2")}, true)
		require.NoError(t, err)
		assert.Equal(t, &v2model.WidgetPlaced{After: "w2"}, got.Placed)
	})

	t.Run("the service's own grant backstop refuses a read-only or foreign key on create", func(t *testing.T) {
		fx := newV2Fixture(t)
		for _, ctx := range []context.Context{grantCtx(util.GrantPermsRead, testSpaceId), grantCtx(util.GrantPermsReadWrite, "someOtherSpace")} {
			_, err := fx.CreateWidget(ctx, testSpaceId, v2model.CreateWidgetRequest{Target: "page1", Scope: "space"}, false)
			require.Equal(t, http.StatusForbidden, v2Err(t, err).Status)
		}
	})
}

func TestUpdateWidget(t *testing.T) {
	t.Run("a layout change re-fits the limit and moves in one write", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace,
			widgetEntry("w1", "space", "favorite", model.BlockContentWidget_CompactList, 6),
			widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.expectUpdate(t, apicore.WidgetScopeSpace, "w2", widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, 6),
			apicore.WidgetUpdate{Layout: layoutPtr(model.BlockContentWidget_List), Limit: int32Ptr(6), Placement: &apicore.WidgetPlacement{First: true}},
			widgetEntry("w2", "space", "set1", model.BlockContentWidget_List, 6))

		// when: addressed by target, no scope needed when only one root holds it
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "set1", "",
			v2model.UpdateWidgetRequest{Layout: strPtr("list"), Position: strPtr("first")}, false)

		// then: the stored 6 is on the list layout's list too, so it stays
		require.NoError(t, err)
		assert.Equal(t, v2model.WidgetRow{Id: "w2", Scope: "space", Target: "set1", Layout: "list", Limit: 6}, got.WidgetRow)
		assert.Empty(t, got.Warnings)
	})

	t.Run("a layout change re-fits a stored limit the new list lacks, and always writes the pair", func(t *testing.T) {
		// given: view with limit 10; list has no 10
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, 10))
		fx.holdWidgets(apicore.WidgetScopePersonal)

		// when
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{Layout: strPtr("list")}, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, 4, got.Limit)
		require.Len(t, got.Warnings, 1)
		assert.Equal(t, "/limit", got.Warnings[0].Path)

		// and a limit-only patch still carries the layout, so a concurrent
		// layout change cannot leave a pair the app would not show
		fx.expectUpdate(t, apicore.WidgetScopeSpace, "w2", widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, 10),
			apicore.WidgetUpdate{Layout: layoutPtr(model.BlockContentWidget_View), Limit: int32Ptr(14)},
			widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, 14))
		got, err = fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{Limit: intPtr(14)}, false)
		require.NoError(t, err)
		assert.Equal(t, 14, got.Limit)
	})

	t.Run("the pair is planned against the widget as stored under the lock", func(t *testing.T) {
		// given: the service read view/6, but by the time the lock is held
		// another request has made it list/4
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.expectUpdate(t, apicore.WidgetScopeSpace, "w2", widgetEntry("w2", "space", "set1", model.BlockContentWidget_List, 4),
			// the limit-only body keeps the CONCURRENT layout, and 30 is on
			// the list layout's list
			apicore.WidgetUpdate{Layout: layoutPtr(model.BlockContentWidget_List), Limit: int32Ptr(30)},
			widgetEntry("w2", "space", "set1", model.BlockContentWidget_List, 30))

		// when
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{Limit: intPtr(30)}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "list", got.Layout)
		assert.Empty(t, got.Warnings)
	})

	t.Run("a limit-only patch re-validates a stored layout the target cannot render", func(t *testing.T) {
		// given: heart let a page widget be stored as list; the app renders tree
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_List, 8))
		fx.holdWidgets(apicore.WidgetScopePersonal)

		// when
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{Limit: intPtr(8)}, true)

		// then: the pair is made one the app shows, and both substitutions are said
		require.NoError(t, err)
		assert.Equal(t, "tree", got.Layout)
		assert.Equal(t, 6, got.Limit)
		require.Len(t, got.Warnings, 2)
		assert.Equal(t, "/layout", got.Warnings[0].Path)
		assert.Equal(t, "/limit", got.Warnings[1].Path)
	})

	t.Run("a layout change keeps 30 and 50, which both lists hold", func(t *testing.T) {
		for _, limit := range []int32{30, 50} {
			fx := newV2Fixture(t)
			fx.addWidgetTarget(t, "set1", model.ObjectType_set)
			fx.allowWidgets(apicore.WidgetScopeSpace, true)
			fx.holdWidgets(apicore.WidgetScopePersonal)
			fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, limit))
			got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{Layout: strPtr("list")}, true)
			require.NoError(t, err)
			assert.Equal(t, int(limit), got.Limit)
			assert.Empty(t, got.Warnings)
		}
	})

	t.Run("PATCH placement translates last, after and before", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace,
			widgetEntry("w1", "space", "a", model.BlockContentWidget_Tree, 6),
			widgetEntry("w2", "space", "b", model.BlockContentWidget_Tree, 6),
			widgetEntry("w3", "space", "c", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		for _, tc := range []struct {
			req  v2model.UpdateWidgetRequest
			want apicore.WidgetPlacement
		}{
			{v2model.UpdateWidgetRequest{Position: strPtr("last")}, apicore.WidgetPlacement{}},
			{v2model.UpdateWidgetRequest{After: strPtr("c")}, apicore.WidgetPlacement{AfterId: "w3"}},
			{v2model.UpdateWidgetRequest{Before: strPtr("w1")}, apicore.WidgetPlacement{BeforeId: "w1"}},
		} {
			want := tc.want
			fx.expectUpdate(t, apicore.WidgetScopeSpace, "w2", widgetEntry("w2", "space", "b", model.BlockContentWidget_Tree, 6),
				apicore.WidgetUpdate{Placement: &want}, widgetEntry("w2", "space", "b", model.BlockContentWidget_Tree, 6))
			got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", tc.req, false)
			require.NoError(t, err, "%+v", tc.req)
			assert.Equal(t, widgetPlaced(want), got.Placed, "the committed move is receipted")
		}
	})

	t.Run("view_id changes and clears on PATCH, and an ambiguous suffix is refused", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "set1").Return(apicore.ObjectRead{
			Snapshot: &model.SmartBlockSnapshotBase{Blocks: []*model.Block{{Id: "dataview", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{Id: "view-aaa111"}, {Id: "view-baa111"}},
			}}}}},
		}, nil).Maybe()
		fx.expectUpdate(t, apicore.WidgetScopeSpace, "w2", widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, 6),
			apicore.WidgetUpdate{ViewId: strPtr("view-baa111")},
			apicore.WidgetEntry{Id: "w2", Scope: "space", Target: "set1", Layout: model.BlockContentWidget_View, Limit: 6, ViewId: "view-baa111"})
		fx.expectUpdate(t, apicore.WidgetScopeSpace, "w2", widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, 6),
			apicore.WidgetUpdate{ViewId: strPtr("")}, widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, 6))

		// when / then: an exact id, a suffix shared by two views, an empty string
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{ViewId: strPtr("view-baa111")}, false)
		require.NoError(t, err)
		assert.Equal(t, "view-baa111", got.ViewId)

		_, err = fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{ViewId: strPtr("aa111")}, false)
		assert.Equal(t, v2model.CodeAmbiguousInput, v2Err(t, err).Code)

		got, err = fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{ViewId: strPtr("")}, false)
		require.NoError(t, err)
		assert.Empty(t, got.ViewId)
	})

	t.Run("a dry run previews a view change and a clear without a write", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.holdWidgets(apicore.WidgetScopeSpace, apicore.WidgetEntry{Id: "w2", Scope: "space", Target: "set1", Layout: model.BlockContentWidget_View, Limit: 6, ViewId: "view-aaa111"})
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "set1").Return(apicore.ObjectRead{
			Snapshot: &model.SmartBlockSnapshotBase{Blocks: []*model.Block{{Id: "dataview", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{Id: "view-aaa111"}, {Id: "view-baa111"}},
			}}}}},
		}, nil).Maybe()
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{ViewId: strPtr("view-baa111")}, true)
		require.NoError(t, err)
		assert.Equal(t, "view-baa111", got.ViewId)
		got, err = fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{ViewId: strPtr("")}, true)
		require.NoError(t, err)
		assert.Empty(t, got.ViewId)
		assert.True(t, got.DryRun)
	})

	t.Run("the canonical dataview block wins over a stray one", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		dv := func(id, view string) *model.Block {
			return &model.Block{Id: id, Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{Views: []*model.BlockContentDataviewView{{Id: view}}}}}
		}
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "set1").Return(apicore.ObjectRead{
			Snapshot: &model.SmartBlockSnapshotBase{Blocks: []*model.Block{dv("stray-before", "view-x"), dv("dataview", "view-canon"), dv("stray-after", "view-y")}},
		}, nil)
		got, err := fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "set1", Scope: "space", ViewId: "view-canon"}, true)
		require.NoError(t, err)
		assert.Equal(t, "view-canon", got.ViewId)
		_, err = fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "set1", Scope: "space", ViewId: "view-x"}, true)
		require.Error(t, err, "a view of a stray dataview is not the target's")
	})

	t.Run("two widgets for one target in ONE root are told apart by id alone", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 6), widgetEntry("w2", "space", "page1", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		_, err := fx.UpdateWidget(context.Background(), testSpaceId, "page1", "", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, true)
		e := v2Err(t, err)
		require.Equal(t, v2model.CodeAmbiguousInput, e.Code)
		assert.Contains(t, e.Message, "more than one widget in the space sidebar")
		require.Len(t, e.Issues[0].SeeAlso, 1)
		assert.Equal(t, v2model.OpListWidgets, e.Issues[0].SeeAlso[0].Op, "a scope resend would not disambiguate here")
		// an explicit scope does not pick one either; the wrapper id does
		_, err = fx.UpdateWidget(context.Background(), testSpaceId, "page1", "space", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, true)
		require.Equal(t, v2model.CodeAmbiguousInput, v2Err(t, err).Code)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "space", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, true)
		require.NoError(t, err)
		assert.Equal(t, "w2", got.Id)
	})

	t.Run("the service's own grant backstop refuses a read-only or foreign key on update", func(t *testing.T) {
		fx := newV2Fixture(t)
		for _, ctx := range []context.Context{grantCtx(util.GrantPermsRead, testSpaceId), grantCtx(util.GrantPermsReadWrite, "someOtherSpace")} {
			_, err := fx.UpdateWidget(ctx, testSpaceId, "w1", "", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, false)
			require.Equal(t, http.StatusForbidden, v2Err(t, err).Status)
		}
	})

	t.Run("the permission gate holds on PATCH for both scopes", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, false)
		fx.allowWidgets(apicore.WidgetScopePersonal, false)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal, widgetEntry("p1", "personal", "page2", model.BlockContentWidget_Tree, 6))
		for _, id := range []string{"w1", "p1"} {
			_, err := fx.UpdateWidget(context.Background(), testSpaceId, id, "", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, true)
			require.Equal(t, http.StatusForbidden, v2Err(t, err).Status, id)
		}
	})

	t.Run("a retarget seen under the lock is a 409", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w2", "space", "set1", model.BlockContentWidget_View, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.widgetsMock.EXPECT().UpdateWidget(mock.Anything, testSpaceId, apicore.WidgetScopeSpace, "w2", mock.Anything).RunAndReturn(
			func(_ context.Context, _ string, _ apicore.WidgetScope, _ string, plan func(apicore.WidgetEntry) (apicore.WidgetUpdate, error)) (apicore.WidgetEntry, error) {
				_, err := plan(widgetEntry("w2", "space", "other-collection", model.BlockContentWidget_View, 6))
				return apicore.WidgetEntry{}, err
			})
		_, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, false)
		e := v2Err(t, err)
		require.Equal(t, http.StatusConflict, e.Status)
		assert.Contains(t, e.Message, "target changed")
	})

	t.Run("a present-but-empty placement or layout member is refused, not read as a default", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "a", model.BlockContentWidget_Tree, 6), widgetEntry("w2", "space", "b", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		for path, req := range map[string]v2model.UpdateWidgetRequest{
			"/after":    {After: strPtr("")},
			"/before":   {Before: strPtr("")},
			"/position": {Position: strPtr("")},
			"/layout":   {Layout: strPtr("")},
		} {
			_, err := fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", req, true)
			e := v2Err(t, err)
			require.Equal(t, http.StatusBadRequest, e.Status, path)
			assert.Equal(t, path, e.Issues[0].Path)
		}
	})

	t.Run("a reorder preview receipts the move without a write", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "a", model.BlockContentWidget_Tree, 6), widgetEntry("w2", "space", "b", model.BlockContentWidget_Tree, 6), widgetEntry("w3", "space", "c", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		for req, want := range map[*v2model.UpdateWidgetRequest]*v2model.WidgetPlaced{
			{Position: strPtr("first")}: {Position: "first"},
			{Position: strPtr("last")}:  {Position: "last"},
			{After: strPtr("c")}:        {After: "w3"},
			{Before: strPtr("a")}:       {Before: "w1"},
		} {
			got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", *req, true)
			require.NoError(t, err)
			assert.Equal(t, want, got.Placed)
			assert.True(t, got.DryRun)
		}
	})

	t.Run("every built-in listing keeps the app's layout set on PATCH; an unknown name is refused", func(t *testing.T) {
		for stored, want := range map[string]string{
			"favorite": "compact_list, list, tree", "recent": "compact_list, list, tree", "recentOpen": "compact_list, list, tree",
			"bin": "link, compact_list, list, tree", "allObjects": "link", "chat": "link",
			"set": "link", "collection": "link",
		} {
			fx := newV2Fixture(t)
			fx.allowWidgets(apicore.WidgetScopeSpace, true)
			fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", stored, model.BlockContentWidget_CompactList, 6))
			fx.holdWidgets(apicore.WidgetScopePersonal)
			for _, name := range []string{"link", "tree", "list", "compact_list", "view"} {
				got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{Layout: strPtr(name)}, true)
				require.NoError(t, err, stored)
				if strings.Contains(want, name) {
					assert.Equal(t, name, got.Layout, "%s %s", stored, name)
				} else {
					assert.Equal(t, strings.Split(want, ", ")[0], got.Layout, "%s %s", stored, name)
					assert.Contains(t, got.Warnings[0].Message, "It takes "+want)
				}
			}
			_, err := fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{Layout: strPtr("grid")}, true)
			assert.Equal(t, "/layout", v2Err(t, err).Issues[0].Path)
		}
	})

	t.Run("an unknown stored layout falls back to the target's first, not to link", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w2", "space", "set1", model.BlockContentWidgetLayout(99), 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, true)
		require.NoError(t, err)
		assert.Equal(t, "view", got.Layout)
		assert.Equal(t, 10, got.Limit, "the requested limit is kept, the view layout shows 10")
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, "layout 99")
	})

	t.Run("the bin widget keeps its place and nothing goes after it", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 6), widgetEntry("wb", "space", "bin", model.BlockContentWidget_Link, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		_, err := fx.UpdateWidget(context.Background(), testSpaceId, "_bin", "", v2model.UpdateWidgetRequest{Position: strPtr("first")}, true)
		assert.Contains(t, v2Err(t, err).Message, "bin widget keeps its place")
		_, err = fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{After: strPtr("_bin")}, true)
		assert.Equal(t, "/after", v2Err(t, err).Issues[0].Path)
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{Before: strPtr("_bin")}, true)
		require.NoError(t, err)
		assert.Equal(t, &v2model.WidgetPlaced{Before: "wb"}, got.Placed)
		// last, with a trailing bin, is before the bin — and the receipt says so
		got, err = fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{Position: strPtr("last")}, true)
		require.NoError(t, err)
		assert.Equal(t, &v2model.WidgetPlaced{Before: "wb"}, got.Placed)
		// a default create too, committed: the adapter is asked for before-bin
		fx.addWidgetTarget(t, "page2", model.ObjectType_basic)
		fx.widgetsMock.EXPECT().CreateWidget(mock.Anything, testSpaceId, apicore.WidgetScopeSpace,
			apicore.WidgetCreate{Target: "page2", Layout: model.BlockContentWidget_Tree, Limit: 6, Placement: apicore.WidgetPlacement{BeforeId: "wb"}}).
			Return(widgetEntry("w3", "space", "page2", model.BlockContentWidget_Tree, 6), nil)
		got, err = fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page2", Scope: "space"}, false)
		require.NoError(t, err)
		assert.Nil(t, got.Placed, "no placement was asked, so none is receipted")
		got, err = fx.CreateWidget(context.Background(), testSpaceId, v2model.CreateWidgetRequest{Target: "page2", Scope: "space", Position: strPtr("last")}, true)
		require.NoError(t, err)
		assert.Equal(t, &v2model.WidgetPlaced{Before: "wb"}, got.Placed)
	})

	t.Run("a limit survives a trip through link and back", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 14))
		fx.expectUpdate(t, apicore.WidgetScopeSpace, "w1", widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 14),
			apicore.WidgetUpdate{Layout: layoutPtr(model.BlockContentWidget_Link), Limit: int32Ptr(14)},
			widgetEntry("w1", "space", "page1", model.BlockContentWidget_Link, 14))
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{Layout: strPtr("link")}, false)
		require.NoError(t, err)
		assert.Zero(t, got.Limit, "a link row serves no limit")
		fx.expectUpdate(t, apicore.WidgetScopeSpace, "w1", widgetEntry("w1", "space", "page1", model.BlockContentWidget_Link, 14),
			apicore.WidgetUpdate{Layout: layoutPtr(model.BlockContentWidget_Tree), Limit: int32Ptr(14)},
			widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 14))
		got, err = fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{Layout: strPtr("tree")}, false)
		require.NoError(t, err)
		assert.Equal(t, 14, got.Limit, "the stored limit came back with the layout")
	})

	t.Run("an empty view_id clears a legacy personal view", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopePersonal, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		fx.holdWidgets(apicore.WidgetScopePersonal, apicore.WidgetEntry{Id: "p1", Scope: "personal", Target: "set1", Layout: model.BlockContentWidget_View, Limit: 6, ViewId: "view-old"})
		fx.expectUpdate(t, apicore.WidgetScopePersonal, "p1", apicore.WidgetEntry{Id: "p1", Scope: "personal", Target: "set1", Layout: model.BlockContentWidget_View, Limit: 6, ViewId: "view-old"},
			apicore.WidgetUpdate{ViewId: strPtr("")}, widgetEntry("p1", "personal", "set1", model.BlockContentWidget_View, 6))
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "p1", "", v2model.UpdateWidgetRequest{ViewId: strPtr("")}, false)
		require.NoError(t, err)
		assert.Empty(t, got.ViewId)
	})

	t.Run("identical wrapper ids in both roots are ambiguous without a scope", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopePersonal, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("same", "space", "a", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal, widgetEntry("same", "personal", "b", model.BlockContentWidget_Tree, 6))
		_, err := fx.UpdateWidget(context.Background(), testSpaceId, "same", "", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, true)
		assert.Equal(t, v2model.CodeAmbiguousInput, v2Err(t, err).Code)
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "same", "personal", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, true)
		require.NoError(t, err)
		assert.Equal(t, "b", got.Target)
	})

	t.Run("a patch to link keeps the stored limit even when one is sent", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 14))
		fx.expectUpdate(t, apicore.WidgetScopeSpace, "w1", widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 14),
			apicore.WidgetUpdate{Layout: layoutPtr(model.BlockContentWidget_Link), Limit: int32Ptr(14)},
			widgetEntry("w1", "space", "page1", model.BlockContentWidget_Link, 14))
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{Layout: strPtr("link"), Limit: intPtr(30)}, false)
		require.NoError(t, err)
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, "the stored one stays")
	})

	t.Run("an unknown stored layout is served as the app's fallback, with a warning, on reads and answers", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w2", "space", "set1", model.BlockContentWidgetLayout(99), 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)

		rows, _, _, warnings, err := fx.ListWidgets(context.Background(), testSpaceId, "", 0, 25)
		require.NoError(t, err)
		assert.Equal(t, "view", rows[0].Layout)
		assert.Equal(t, 6, rows[0].Limit, "the fallback view shows a limit, so the stored one is served")
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, "widget w2 stores layout 99")

		// a placement-only patch answers the same way
		fx.expectUpdate(t, apicore.WidgetScopeSpace, "w2", widgetEntry("w2", "space", "set1", model.BlockContentWidgetLayout(99), 6),
			apicore.WidgetUpdate{Placement: &apicore.WidgetPlacement{First: true}},
			widgetEntry("w2", "space", "set1", model.BlockContentWidgetLayout(99), 6))
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{Position: strPtr("first")}, false)
		require.NoError(t, err)
		assert.Equal(t, "view", got.Layout)
		require.Len(t, got.Warnings, 1)

		// and a delete preview
		got, err = fx.DeleteWidget(context.Background(), testSpaceId, "w2", "", true)
		require.NoError(t, err)
		assert.Equal(t, "view", got.Layout)
		require.Len(t, got.Warnings, 1)
	})

	t.Run("a committed move is receipted as applied under the lock, not as asked", func(t *testing.T) {
		// given: the service read no trailing bin; by commit time the
		// adapter found one and put the widget before it
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "a", model.BlockContentWidget_Tree, 6), widgetEntry("w2", "space", "b", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		applied := widgetEntry("w1", "space", "a", model.BlockContentWidget_Tree, 6)
		applied.Placed = &apicore.WidgetPlacement{BeforeId: "wb"}
		fx.expectUpdate(t, apicore.WidgetScopeSpace, "w1", widgetEntry("w1", "space", "a", model.BlockContentWidget_Tree, 6),
			apicore.WidgetUpdate{Placement: &apicore.WidgetPlacement{}}, applied)
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{Position: strPtr("last")}, false)
		require.NoError(t, err)
		assert.Equal(t, &v2model.WidgetPlaced{Before: "wb"}, got.Placed)
	})

	t.Run("a link widget keeps a view for its target, and before with position is refused too", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "set1", model.ObjectType_set)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "a", model.BlockContentWidget_Tree, 6), widgetEntry("w2", "space", "set1", model.BlockContentWidget_Link, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "set1").Return(apicore.ObjectRead{
			Snapshot: &model.SmartBlockSnapshotBase{Blocks: []*model.Block{{Id: "dataview", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{Id: "view-one"}},
			}}}}},
		}, nil).Maybe()
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{ViewId: strPtr("view-one")}, true)
		require.NoError(t, err)
		assert.Equal(t, "view-one", got.ViewId)
		_, err = fx.UpdateWidget(context.Background(), testSpaceId, "w2", "", v2model.UpdateWidgetRequest{Before: strPtr("w1"), Position: strPtr("first")}, true)
		assert.Equal(t, v2model.CodeAmbiguousInput, v2Err(t, err).Code)
	})

	t.Run("an existing target is classified on PATCH the way a new one is", func(t *testing.T) {
		for name, tc := range map[string]struct {
			layout  model.ObjectTypeLayout
			allowed string
		}{"file": {model.ObjectType_file, "link"}, "date": {model.ObjectType_date, "link"}, "participant": {model.ObjectType_participant, "link"}, "tag": {model.ObjectType_tag, "tree, link"}} {
			fx := newV2Fixture(t)
			fx.addWidgetTarget(t, "t1", tc.layout)
			fx.allowWidgets(apicore.WidgetScopeSpace, true)
			fx.holdWidgets(apicore.WidgetScopePersonal)
			fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "t1", model.BlockContentWidget_Link, 6))
			got, err := fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{Layout: strPtr("view")}, true)
			require.NoError(t, err, name)
			assert.Equal(t, strings.Split(tc.allowed, ", ")[0], got.Layout, name)
			assert.Contains(t, got.Warnings[0].Message, "It takes "+tc.allowed, name)
		}
	})

	t.Run("an empty patch is refused", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, err := fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{}, true)
		require.Equal(t, http.StatusBadRequest, v2Err(t, err).Status)
	})

	t.Run("a target held in both scopes needs the scope said", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addWidgetTarget(t, "page1", model.ObjectType_basic)
		fx.allowWidgets(apicore.WidgetScopePersonal, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal, widgetEntry("p1", "personal", "page1", model.BlockContentWidget_Tree, 6))

		// when
		_, err := fx.UpdateWidget(context.Background(), testSpaceId, "page1", "", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, true)

		// then
		e := v2Err(t, err)
		require.Equal(t, v2model.CodeAmbiguousInput, e.Code)
		require.Len(t, e.Issues, 1)
		assert.Contains(t, e.Issues[0].Message, "w1 (space), p1 (personal)")
		require.Len(t, e.Issues[0].SeeAlso, 2)
		assert.Equal(t, map[string]string{"scope": "space"}, e.Issues[0].SeeAlso[0].Query)
		assert.Equal(t, map[string]string{"scope": "personal"}, e.Issues[0].SeeAlso[1].Query)
		assert.Empty(t, e.Issues[0].SeeAlso[0].Op, "a resend names no operation")

		// and with the scope said, it resolves — as a dry run
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "page1", "personal", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, true)
		require.NoError(t, err)
		assert.Equal(t, "p1", got.Id)
		assert.Equal(t, 10, got.Limit)
		assert.True(t, got.DryRun)
	})

	t.Run("an unknown widget is a 404 with the list steer", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		fx.holdWidgets(apicore.WidgetScopePersonal)
		_, err := fx.UpdateWidget(context.Background(), testSpaceId, "nope", "", v2model.UpdateWidgetRequest{Limit: intPtr(10)}, true)
		e := v2Err(t, err)
		require.Equal(t, http.StatusNotFound, e.Status)
		require.Len(t, e.Issues[0].SeeAlso, 1)
		assert.Equal(t, v2model.OpListWidgets, e.Issues[0].SeeAlso[0].Op)
	})

	t.Run("an existing listing widget keeps the app's layout set", func(t *testing.T) {
		// given: favorite takes compact_list, list, tree — never link
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "favorite", model.BlockContentWidget_CompactList, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)

		// when
		got, err := fx.UpdateWidget(context.Background(), testSpaceId, "_favorite", "", v2model.UpdateWidgetRequest{Layout: strPtr("link")}, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, "compact_list", got.Layout)
		require.Len(t, got.Warnings, 1)
		assert.Contains(t, got.Warnings[0].Message, "compact_list, list, tree")
	})

	t.Run("a widget cannot be placed relative to itself", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "favorite", model.BlockContentWidget_CompactList, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		_, err := fx.UpdateWidget(context.Background(), testSpaceId, "w1", "", v2model.UpdateWidgetRequest{After: strPtr("w1")}, true)
		assert.Contains(t, v2Err(t, err).Message, "relative to itself")
	})
}

func TestDeleteWidget(t *testing.T) {
	t.Run("removes by id and answers the removed row", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopePersonal, true)
		fx.holdWidgets(apicore.WidgetScopeSpace)
		fx.holdWidgets(apicore.WidgetScopePersonal, widgetEntry("p1", "personal", "page1", model.BlockContentWidget_Tree, 6))
		fx.widgetsMock.EXPECT().DeleteWidget(mock.Anything, testSpaceId, apicore.WidgetScopePersonal, "p1", "page1").
			Return(widgetEntry("p1", "personal", "page1", model.BlockContentWidget_Tree, 6), nil)
		want := &v2model.WidgetResult{WidgetRow: v2model.WidgetRow{Id: "p1", Scope: "personal", Target: "page1", Layout: "tree", Limit: 6}, Removed: true}

		// when
		got, err := fx.DeleteWidget(context.Background(), testSpaceId, "p1", "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("dry run removes nothing", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "bin", model.BlockContentWidget_Link, 6))
		got, err := fx.DeleteWidget(context.Background(), testSpaceId, "_bin", "space", true)
		require.NoError(t, err)
		assert.True(t, got.DryRun)
		assert.False(t, got.Removed)
		assert.Equal(t, "_bin", got.Target)
	})

	t.Run("a widget that vanished between resolve and delete is a 404", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.widgetsMock.EXPECT().DeleteWidget(mock.Anything, testSpaceId, apicore.WidgetScopeSpace, "w1", "page1").Return(apicore.WidgetEntry{}, apicore.ErrWidgetNotFound)
		_, err := fx.DeleteWidget(context.Background(), testSpaceId, "w1", "", false)
		require.Equal(t, http.StatusNotFound, v2Err(t, err).Status)
	})

	t.Run("a widget retargeted under the lock is not removed under its old name", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal)
		fx.widgetsMock.EXPECT().DeleteWidget(mock.Anything, testSpaceId, apicore.WidgetScopeSpace, "w1", "page1").Return(apicore.WidgetEntry{}, apicore.ErrWidgetRetargeted)
		_, err := fx.DeleteWidget(context.Background(), testSpaceId, "page1", "", false)
		require.Equal(t, http.StatusConflict, v2Err(t, err).Status)
	})

	t.Run("a target in both scopes is ambiguous on DELETE too, and scope resolves it", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, true)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal, widgetEntry("p1", "personal", "page1", model.BlockContentWidget_Tree, 6))
		_, err := fx.DeleteWidget(context.Background(), testSpaceId, "page1", "", true)
		assert.Equal(t, v2model.CodeAmbiguousInput, v2Err(t, err).Code)
		got, err := fx.DeleteWidget(context.Background(), testSpaceId, "page1", "space", true)
		require.NoError(t, err)
		assert.Equal(t, "w1", got.Id)
	})

	t.Run("the permission gate holds on DELETE for both scopes", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, false)
		fx.allowWidgets(apicore.WidgetScopePersonal, false)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal, widgetEntry("p1", "personal", "page2", model.BlockContentWidget_Tree, 6))
		for _, id := range []string{"w1", "p1"} {
			_, err := fx.DeleteWidget(context.Background(), testSpaceId, id, "", false)
			require.Equal(t, http.StatusForbidden, v2Err(t, err).Status, id)
		}
	})

	t.Run("a denied delete preview is a 403 in either scope", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.allowWidgets(apicore.WidgetScopeSpace, false)
		fx.allowWidgets(apicore.WidgetScopePersonal, false)
		fx.holdWidgets(apicore.WidgetScopeSpace, widgetEntry("w1", "space", "page1", model.BlockContentWidget_Tree, 6))
		fx.holdWidgets(apicore.WidgetScopePersonal, widgetEntry("p1", "personal", "page2", model.BlockContentWidget_Tree, 6))
		for _, id := range []string{"w1", "p1"} {
			_, err := fx.DeleteWidget(context.Background(), testSpaceId, id, "", true)
			require.Equal(t, http.StatusForbidden, v2Err(t, err).Status, id)
		}
	})

	t.Run("a read-only key is refused before anything resolves", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, err := fx.DeleteWidget(grantCtx(util.GrantPermsRead, testSpaceId), testSpaceId, "w1", "", false)
		assert.Equal(t, v2model.CodeWriteNotGranted, v2Err(t, err).Code)
	})

	t.Run("without the port every call fails closed", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.Service.widgets = nil
		_, _, _, _, err := fx.ListWidgets(context.Background(), testSpaceId, "", 0, 25)
		var v2e *v2model.Error
		require.True(t, errors.As(err, &v2e))
		assert.Equal(t, http.StatusInternalServerError, v2e.Status)
	})
}

func int32Ptr(v int32) *int32 { return &v }
