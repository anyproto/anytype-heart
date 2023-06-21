package restriction

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestService_ObjectRestrictionsById(t *testing.T) {
	rest := New(nil, nil)
	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		id: "",
		tp: model.SmartBlockType_AnytypeProfile,
	}).Object.Check(
		model.Restrictions_Blocks,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Delete,
		model.Restrictions_Duplicate,
	),
		ErrRestricted,
	)

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		id:     "",
		tp:     model.SmartBlockType_Page,
		layout: model.ObjectType_collection,
	}).Object.Check(model.Restrictions_Blocks),
		ErrRestricted,
	)

	assert.NoError(t, rest.GetRestrictions(&restrictionHolder{
		id: "",
		tp: model.SmartBlockType_Page,
	}).Object.Check(model.Restrictions_Blocks))

	assert.NoError(t, rest.GetRestrictions(&restrictionHolder{
		id:     bundle.TypeKeyDailyPlan.URL(),
		tp:     model.SmartBlockType_SubObject,
		layout: model.ObjectType_objectType,
	}).Object.Check(
		model.Restrictions_Details,
		model.Restrictions_Delete,
	))

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		id:     bundle.TypeKeyPage.URL(),
		tp:     model.SmartBlockType_SubObject,
		layout: model.ObjectType_objectType,
	}).Object.Check(
		model.Restrictions_Details,
		model.Restrictions_Delete,
	), ErrRestricted)

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		id:     bundle.TypeKeyBookmark.BundledURL(),
		tp:     model.SmartBlockType_BundledObjectType,
		layout: model.ObjectType_objectType,
	}).Object.Check(
		model.Restrictions_Duplicate,
		model.Restrictions_Relations,
	), ErrRestricted)

	assert.NoError(t, rest.GetRestrictions(&restrictionHolder{
		id:     bundle.RelationKeyImdbRating.String(),
		tp:     model.SmartBlockType_SubObject,
		layout: model.ObjectType_relation,
	}).Object.Check(
		model.Restrictions_Delete,
		model.Restrictions_Relations,
		model.Restrictions_Details,
	))

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		id:     bundle.RelationKeyName.URL(),
		tp:     model.SmartBlockType_SubObject,
		layout: model.ObjectType_relation,
	}).Object.Check(
		model.Restrictions_Delete,
		model.Restrictions_Relations,
		model.Restrictions_Details,
	), ErrRestricted)

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		id:     bundle.RelationKeyId.BundledURL(),
		tp:     model.SmartBlockType_BundledRelation,
		layout: model.ObjectType_relation,
	}).Object.Check(
		model.Restrictions_Duplicate,
	), ErrRestricted)
}
