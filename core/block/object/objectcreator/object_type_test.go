package objectcreator

import (
	"context"
	"fmt"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestFillRecommendedRelations(t *testing.T) {
	spc := mock_clientspace.NewMockSpace(t)
	spc.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, key domain.UniqueKey) (string, error) {
		return domain.RelationKey(key.InternalKey()).URL(), nil
	}).Maybe()
	spc.EXPECT().IsReadOnly().Return(true).Maybe()
	defaultRecFeatRelIds := buildRelationIds(defaultRecommendedFeaturedRelationKeys)
	defaultRecRelIds := buildRelationIds(defaultRecommendedRelationKeys)

	for _, tc := range []struct {
		name     string
		given    []string
		expected []string
	}{
		{
			"empty", []string{}, defaultRecRelIds,
		},
		{
			"no intersect",
			[]string{bundle.RelationKeyAssignee.BundledURL(), bundle.RelationKeyDone.BundledURL()},
			append([]string{bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL()}, defaultRecRelIds...),
		},
		{
			"fully intersect with system",
			[]string{bundle.RelationKeyLinks.BundledURL(), bundle.RelationKeyCreator.BundledURL()},
			lo.Uniq(append([]string{bundle.RelationKeyLinks.URL(), bundle.RelationKeyCreator.URL()}, defaultRecRelIds...)),
		},
		{
			"partially intersect with system",
			[]string{bundle.RelationKeyLinks.BundledURL(), bundle.RelationKeyDone.BundledURL()},
			lo.Uniq(append([]string{bundle.RelationKeyLinks.URL(), bundle.RelationKeyDone.URL()}, defaultRecRelIds...)),
		},
		{
			"intersect with featured",
			[]string{bundle.RelationKeyType.BundledURL(), bundle.RelationKeyTag.BundledURL(), bundle.RelationKeyIconOption.BundledURL()},
			append([]string{bundle.RelationKeyIconOption.URL()}, defaultRecRelIds...),
		},
		{
			"intersect both with featured and system",
			[]string{bundle.RelationKeyBacklinks.BundledURL(), bundle.RelationKeyAddedDate.BundledURL(), bundle.RelationKeyCreatedDate.BundledURL()},
			lo.Uniq(append([]string{bundle.RelationKeyAddedDate.URL(), bundle.RelationKeyCreatedDate.URL()}, defaultRecRelIds...)),
		},
	} {
		t.Run(fmt.Sprintf("from source: %s", tc.name), func(t *testing.T) {
			// given
			details := &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList(tc.given),
			}}

			// when
			keys, isAlreadyFilled, err := FillRecommendedRelations(nil, spc, details)

			// then
			assert.NoError(t, err)
			assert.False(t, isAlreadyFilled)
			assert.Equal(t, tc.expected, pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String()))
			assert.Equal(t, defaultRecFeatRelIds, pbtypes.GetStringList(details, bundle.RelationKeyRecommendedFeaturedRelations.String()))
			assert.Len(t, keys, len(tc.expected)+3)
		})
	}

	t.Run("recommendedRelations are already filled", func(t *testing.T) {
		// given
		details := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList([]string{
				"createdBy", "createdDate", "backlinks",
			}),
		}}

		// when
		keys, isAlreadyFilled, err := FillRecommendedRelations(nil, spc, details)

		// then
		assert.NoError(t, err)
		assert.True(t, isAlreadyFilled)
		assert.Empty(t, keys)
	})

	for _, tc := range []struct {
		name     string
		layout   int64
		expected []string
	}{
		{
			"empty", int64(0), defaultRecRelIds,
		},
		{
			"basic", int64(model.ObjectType_basic), defaultRecRelIds,
		},
		{
			"set", int64(model.ObjectType_set), append([]string{bundle.RelationKeySetOf.URL()}, defaultRecRelIds...),
		},
		{
			"to do", int64(model.ObjectType_todo), append([]string{bundle.RelationKeyDone.URL()}, defaultRecRelIds...),
		},
		{
			"note", int64(model.ObjectType_note), defaultRecRelIds,
		},
		{
			"collection", int64(model.ObjectType_collection), defaultRecRelIds,
		},
	} {
		t.Run(fmt.Sprintf("from layout: %s", tc.name), func(t *testing.T) {
			// given
			details := &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyRecommendedLayout.String(): pbtypes.Int64(tc.layout),
			}}

			// when
			keys, isAlreadyFilled, err := FillRecommendedRelations(nil, spc, details)

			// then
			assert.NoError(t, err)
			assert.False(t, isAlreadyFilled)
			assert.Equal(t, tc.expected, pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String()))
			assert.Equal(t, defaultRecFeatRelIds, pbtypes.GetStringList(details, bundle.RelationKeyRecommendedFeaturedRelations.String()))
			assert.Len(t, keys, len(tc.expected)+3)
		})
	}
}

func buildRelationIds(keys []domain.RelationKey) []string {
	ids := make([]string, len(keys))
	for i, rel := range keys {
		ids[i] = rel.URL()
	}
	return ids
}
