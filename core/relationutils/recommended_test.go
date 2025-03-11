package relationutils

import (
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type mockDeriver struct{}

func (d *mockDeriver) DeriveObjectID(ctx context.Context, key domain.UniqueKey) (string, error) {
	return domain.RelationKey(key.InternalKey()).URL(), nil
}

func TestFillRecommendedRelations(t *testing.T) {
	defaultRecFeatRelIds := buildRelationIds(defaultFeaturedRelationKeys)
	defaultRecRelIds := buildRelationIds(defaultRecommendedRelationKeys)
	defaultRecHiddenRelIds := buildRelationIds(defaultHiddenRelationKeys)

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
			lo.Uniq(append([]string{bundle.RelationKeyCreatedDate.URL()}, defaultRecRelIds...)),
		},
		{
			"exclude description",
			[]string{bundle.RelationKeyAssignee.BundledURL(), bundle.RelationKeyDescription.BundledURL()},
			append([]string{bundle.RelationKeyAssignee.URL()}, defaultRecRelIds...),
		},
	} {
		t.Run(fmt.Sprintf("from source: %s", tc.name), func(t *testing.T) {
			// given
			details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyRecommendedRelations: domain.StringList(tc.given),
			})

			// when
			keys, isAlreadyFilled, err := FillRecommendedRelations(nil, &mockDeriver{}, details, bundle.TypeKeyNote)

			// then
			assert.NoError(t, err)
			assert.False(t, isAlreadyFilled)
			assert.Equal(t, tc.expected, details.GetStringList(bundle.RelationKeyRecommendedRelations))
			assert.Equal(t, defaultRecFeatRelIds, details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))
			assert.Equal(t, defaultRecHiddenRelIds, details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations))
			assert.Len(t, keys, len(tc.expected)+3+4) // 3 featured and 4 hidden
		})
	}

	t.Run("recommendedRelations are already filled", func(t *testing.T) {
		// given
		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{
				"createdBy", "createdDate", "backlinks",
			}),
		})

		// when
		keys, isAlreadyFilled, err := FillRecommendedRelations(nil, &mockDeriver{}, details, bundle.TypeKeyPage)

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
			details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyRecommendedLayout: domain.Int64(tc.layout),
			})

			// when
			keys, isAlreadyFilled, err := FillRecommendedRelations(nil, &mockDeriver{}, details, bundle.TypeKeyTask)

			// then
			assert.NoError(t, err)
			assert.False(t, isAlreadyFilled)
			assert.Equal(t, tc.expected, details.GetStringList(bundle.RelationKeyRecommendedRelations))
			assert.Equal(t, defaultRecFeatRelIds, details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))
			assert.Equal(t, defaultRecHiddenRelIds, details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations))
			assert.Len(t, keys, len(tc.expected)+3+4) // 3 featured and 4 hidden
		})
	}

	t.Run("recommendedRelations of file types", func(t *testing.T) {
		// given
		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{
				bundle.RelationKeyFileExt.BundledURL(),
				bundle.RelationKeyAddedDate.BundledURL(),
				bundle.RelationKeyCameraIso.BundledURL(),
				bundle.RelationKeyAperture.BundledURL(),
			}),
			bundle.RelationKeyUniqueKey: domain.String(bundle.TypeKeyImage.URL()),
		})

		// when
		keys, isAlreadyFilled, err := FillRecommendedRelations(nil, &mockDeriver{}, details, bundle.TypeKeyProject)

		// then
		assert.NoError(t, err)
		assert.False(t, isAlreadyFilled)
		assert.Equal(t, buildRelationIds(defaultRecommendedRelationKeys), details.GetStringList(bundle.RelationKeyRecommendedRelations))
		assert.Equal(t, defaultRecFeatRelIds, details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))
		assert.Equal(t, defaultRecHiddenRelIds, details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations))
		assert.Equal(t, []string{
			bundle.RelationKeyFileExt.URL(),
			bundle.RelationKeyCameraIso.URL(),
			bundle.RelationKeyAperture.URL(),
		}, details.GetStringList(bundle.RelationKeyRecommendedFileRelations))
		assert.Len(t, keys, 13)
	})

	t.Run("recommendedRelations relations of Set", func(t *testing.T) {
		// given
		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{
				bundle.RelationKeySetOf.BundledURL(),
				bundle.RelationKeyType.BundledURL(),
				bundle.RelationKeyTag.BundledURL(),
				bundle.RelationKeyCreatedDate.BundledURL(),
			}),
		})

		// when
		keys, isAlreadyFilled, err := FillRecommendedRelations(nil, &mockDeriver{}, details, bundle.TypeKeySet)

		// then
		assert.NoError(t, err)
		assert.False(t, isAlreadyFilled)
		assert.Equal(t, buildRelationIds(defaultRecommendedRelationKeys), details.GetStringList(bundle.RelationKeyRecommendedRelations))
		assert.Equal(t, buildRelationIds(defaultSetFeaturedRelationKeys), details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))
		assert.Equal(t, defaultRecHiddenRelIds, details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations))
		assert.Len(t, keys, 4+3+4) // 4 featured + 3 sidebar + 4 hidden
	})
}

func buildRelationIds(keys []domain.RelationKey) []string {
	ids := make([]string, len(keys))
	for i, rel := range keys {
		ids[i] = rel.URL()
	}
	return ids
}
