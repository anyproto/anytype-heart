package subscription

import (
	"github.com/anytypeio/go-anytype-middleware/core/kanban"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func genTagEntries() []*entry {
	return []*entry{
		{id: "id_one", data: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyTag.String(): pbtypes.StringList([]string{"tag_1"}),
		}}},
		{id: "id_two", data: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyTag.String(): pbtypes.StringList([]string{"tag_2"}),
		}}},
		{id: "id_three", data: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyTag.String(): pbtypes.StringList([]string{"tag_1", "tag_2", "tag_3"}),
		}}},
	}
}

func tagEntriesToGroups(entries []*entry) []*model.BlockContentDataviewGroup {
	recs := make([]database.Record, len(entries))
	for _, e := range entries {
		recs = append(recs, database.Record{Details: e.data})
	}
	tags := kanban.GroupTag{Key: bundle.RelationKeyTag.String(), Records: recs}
	groups, err := tags.MakeDataViewGroups()
	if err != nil {
		panic(err)
	}

	return groups
}

func TestGroupTag(t *testing.T) {
	entries := genTagEntries()
	groups := tagEntriesToGroups(entries)

	q := database.Query{
	}

	f, err := database.NewFilters(q, nil, time.Now().Location())
	require.NoError(t, err)

	t.Run("change existing groups", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: bundle.RelationKeyTag.String(), f: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id: "id_three", data: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyTag.String(): pbtypes.StringList([]string{"tag_1", "tag_2"}),
			}}})
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 1, 1)
	})

	t.Run("add new group", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: bundle.RelationKeyTag.String(), f: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id: "id_four", data: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyTag.String(): pbtypes.StringList([]string{"tag_4"}),
			}}})
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 1, 0)
	})

	t.Run("remove existing group by setting tag null", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: bundle.RelationKeyTag.String(), f: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id: "id_one", data: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyTag.String(): pbtypes.StringList([]string{}),
			}}})
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 0, 1)
	})

	t.Run("remove existing group by removing", func(t *testing.T) {
		entries := genTagEntries()
		sub := groupSub{relKey: bundle.RelationKeyTag.String(), f: f, groups: groups, set: make(map[string]struct{}), cache: newCache()}

		require.NoError(t, sub.init(entries))

		ctx := &opCtx{c: sub.cache}
		ctx.entries = append(ctx.entries, &entry{
			id: "id_one", data: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyIsArchived.String(): pbtypes.Bool(true),
			}}})
		sub.onChange(ctx)

		assertCtxGroup(t, ctx, 0, 1)
	})
}
