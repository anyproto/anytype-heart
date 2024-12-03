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
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestService_fillRecommendedRelations(t *testing.T) {
	s := service{}
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
			err := s.fillRecommendedRelations(nil, spc, details, false)

			// then
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String()))
			assert.Equal(t, defaultRecFeatRelIds, pbtypes.GetStringList(details, bundle.RelationKeyRecommendedFeaturedRelations.String()))
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
