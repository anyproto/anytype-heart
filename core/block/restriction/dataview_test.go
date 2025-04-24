package restriction

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestService_DataviewRestrictions(t *testing.T) {
	t.Run("internal types have restrictions", func(t *testing.T) {
		for _, typeKey := range bundle.InternalTypes {
			restrictions := GetRestrictions(givenObjectType(typeKey))
			assert.Equal(t,
				DataviewRestrictions{
					model.RestrictionsDataviewRestrictions{
						BlockId:      DataviewBlockId,
						Restrictions: []model.RestrictionsDataviewRestriction{model.Restrictions_DVCreateObject},
					},
				},
				restrictions.Dataview)
		}
	})

	t.Run("non-internal types have no restrictions", func(t *testing.T) {
		restrictions := GetRestrictions(givenObjectType(bundle.TypeKeyContact))
		assert.Nil(t, restrictions.Dataview)
	})

	t.Run("relations don't have restrictions", func(t *testing.T) {
		restrictions := GetRestrictions(givenRelation(bundle.RelationKeyId))
		assert.Nil(t, restrictions.Dataview)
	})

	t.Run("ordinary objects don't have restrictions", func(t *testing.T) {
		restrictions := GetRestrictions(
			&restrictionHolder{
				sbType:    smartblock.SmartBlockTypePage,
				uniqueKey: nil,
				layout:    model.ObjectType_basic,
			},
		)
		assert.Equal(t, dvRestrictNo, restrictions.Dataview)
	})
}
