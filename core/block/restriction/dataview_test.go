package restriction

import (
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestService_DataviewRestrictions(t *testing.T) {
	rest := New()
	assert.True(t, rest.GetRestrictions(&restrictionHolder{
		id:     bundle.TypeKeyAudio.URL(),
		tp:     model.SmartBlockType_SubObject,
		layout: model.ObjectType_objectType,
	}).Dataview.Equal(DataviewRestrictions{
		model.RestrictionsDataviewRestrictions{
			BlockId:      DataviewBlockId,
			Restrictions: []model.RestrictionsDataviewRestriction{model.Restrictions_DVCreateObject},
		},
	}))

	assert.Nil(t, rest.GetRestrictions(&restrictionHolder{
		id:     bundle.TypeKeyContact.URL(),
		tp:     model.SmartBlockType_SubObject,
		layout: model.ObjectType_objectType,
	}).Dataview)

	assert.Nil(t, rest.GetRestrictions(&restrictionHolder{
		id:     bundle.RelationKeyType.URL(),
		tp:     model.SmartBlockType_SubObject,
		layout: model.ObjectType_relation,
	}).Dataview)

	assert.Nil(t, rest.GetRestrictions(&restrictionHolder{
		id:     bundle.RelationKeySizeInBytes.URL(),
		tp:     model.SmartBlockType_SubObject,
		layout: model.ObjectType_relation,
	}).Dataview)
}
