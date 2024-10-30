package subscription

import (
	"testing"

	"github.com/anyproto/any-store/anyenc"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var kanbanKey = bundle.RelationKeyTag.String()

func genTagEntries() []*entry {
	return []*entry{
		makeTag("tag_1"),
		makeTag("tag_2"),
		makeTag("tag_3"),

		{id: "record_one", data: &types.Struct{Fields: map[string]*types.Value{
			kanbanKey: pbtypes.StringList([]string{"tag_1"}),
		}}},
		{id: "record_two", data: &types.Struct{Fields: map[string]*types.Value{
			kanbanKey: pbtypes.StringList([]string{"tag_2"}),
		}}},
		{id: "record_three", data: &types.Struct{Fields: map[string]*types.Value{
			kanbanKey: pbtypes.StringList([]string{"tag_1", "tag_2", "tag_3"}),
		}}},
	}
}

func tagEntriesToGroups(entries []*entry) []*model.BlockContentDataviewGroup {
	recs := make([]database.Record, len(entries))
	for _, e := range entries {
		recs = append(recs, database.Record{Details: e.data})
	}
	tags := kanban.GroupTag{Key: kanbanKey, Records: recs}
	groups, err := tags.MakeDataViewGroups()
	if err != nil {
		panic(err)
	}

	return groups
}

func makeTag(key string) *entry {
	return &entry{id: key, data: &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyId.String():          pbtypes.String(key),
		bundle.RelationKeyRelationKey.String(): pbtypes.String(kanbanKey),
		bundle.RelationKeyType.String():        pbtypes.String(bundle.TypeKeyRelationOption.URL()),
	}}}
}

func TestGroupTag(t *testing.T) {
	entries := genTagEntries()
	groups := tagEntriesToGroups(entries)

	q := database.Query{}

	f, err := database.NewFilters(q, spaceindex.NewStoreFixture(t), &anyenc.Arena{}, &collate.Buffer{})
	require.NoError(t, err)
	filterTag := database.FilterNot{Filter: database.FilterEmpty{Key: kanbanKey}}
	f.FilterObj = database.FiltersAnd{f.FilterObj, filterTag}
	f.FilterObj = database.FiltersOr{f.FilterObj, database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyRelationKey.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.String(kanbanKey),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyType.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.String(bundle.TypeKeyRelationOption.URL()),
		},
	}}

	t.Run("change_existing_groups", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: kanbanKey, filter: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id: "record_three", data: &types.Struct{Fields: map[string]*types.Value{
				kanbanKey: pbtypes.StringList([]string{"tag_1", "tag_2"}),
			}}})
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 1, 1)
	})

	t.Run("add_new_group_from_existing_tags", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: kanbanKey, filter: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id: "record_four", data: &types.Struct{Fields: map[string]*types.Value{
				kanbanKey: pbtypes.StringList([]string{"tag_1", "tag_2"}),
			}}})
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 1, 0)
	})

	t.Run("add_new_group_by_adding_new_tag", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: kanbanKey, filter: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, makeTag("tag_4"))
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 1, 0)
	})

	t.Run("remove_existing_group_by_setting_tag_null", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: kanbanKey, filter: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id: "record_three", data: &types.Struct{Fields: map[string]*types.Value{
				kanbanKey: pbtypes.StringList([]string{}),
			}}})
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 0, 1)
	})

	t.Run("remove_existing_group_by_removing_record", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: kanbanKey, filter: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id: "record_three", data: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyIsArchived.String(): pbtypes.Bool(true),
			}}})
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 0, 1)
	})

	t.Run("remove_from_group_with_single_tag", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: kanbanKey, filter: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id: "record_one", data: &types.Struct{Fields: map[string]*types.Value{
				kanbanKey: pbtypes.StringList([]string{}),
			}}})
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 0, 0)
	})

	t.Run("remove_tag_which_exist_in_two_groups", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: kanbanKey, filter: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id: "tag_1", data: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyIsArchived.String(): pbtypes.Bool(true),
			}}})
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 1, 2)
	})

	t.Run("add_new_tag", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: kanbanKey, filter: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, makeTag("tag_4"))
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 1, 0)
	})

	t.Run("add_new_tag_and_set_to_record", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: kanbanKey, filter: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries,
			makeTag("tag_4"),
			&entry{id: "record_one", data: &types.Struct{Fields: map[string]*types.Value{
				kanbanKey: pbtypes.StringList([]string{"tag_1", "tag_4"}),
			}}},
		)
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 2, 0)
	})
}
