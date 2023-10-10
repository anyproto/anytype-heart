package restriction

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TODO Use constructors instead for initializing restrictionHolder structures by hand. See givenObjectType and givenRelation
func TestService_ObjectRestrictionsById(t *testing.T) {
	rest := newFixture(t)

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		sbType:       coresb.SmartBlockTypeAnytypeProfile,
		objectTypeID: "",
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
		sbType:       coresb.SmartBlockTypePage,
		layout:       model.ObjectType_collection,
		objectTypeID: bundle.TypeKeyCollection.URL(),
	}).Object.Check(model.Restrictions_Blocks),
		ErrRestricted,
	)

	assert.NoError(t, rest.GetRestrictions(&restrictionHolder{
		sbType:       coresb.SmartBlockTypePage,
		objectTypeID: bundle.TypeKeyPage.URL(),
	}).Object.Check(model.Restrictions_Blocks))

	t.Run("system type", func(t *testing.T) {
		assert.ErrorIs(t, rest.GetRestrictions(givenObjectType(bundle.TypeKeyObjectType)).Object.Check(
			model.Restrictions_Details,
			model.Restrictions_Delete,
		), ErrRestricted)
	})

	t.Run("ordinary type", func(t *testing.T) {
		assert.NoError(t, rest.GetRestrictions(givenObjectType(bundle.TypeKeyDailyPlan)).Object.Check(
			model.Restrictions_Details,
			model.Restrictions_Delete,
		))
	})

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		sbType:       coresb.SmartBlockTypeBundledObjectType,
		layout:       model.ObjectType_objectType,
		objectTypeID: bundle.TypeKeyObjectType.URL(),
	}).Object.Check(
		model.Restrictions_Duplicate,
		model.Restrictions_Relations,
	), ErrRestricted)

	t.Run("ordinary relation", func(t *testing.T) {
		assert.NoError(t, rest.GetRestrictions(givenRelation(bundle.RelationKeyImdbRating)).Object.Check(
			model.Restrictions_Delete,
			model.Restrictions_Relations,
			model.Restrictions_Details,
		))
	})

	t.Run("system relation", func(t *testing.T) {
		assert.ErrorIs(t, rest.GetRestrictions(givenRelation(bundle.RelationKeyId)).Object.Check(
			model.Restrictions_Delete,
			model.Restrictions_Relations,
			model.Restrictions_Details,
		), ErrRestricted)
	})

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		sbType:       coresb.SmartBlockTypeBundledRelation,
		layout:       model.ObjectType_relation,
		objectTypeID: bundle.TypeKeyRelation.URL(),
	}).Object.Check(
		model.Restrictions_Duplicate,
	), ErrRestricted)
}

// TODO Use constructors instead for initializing restrictionHolder structures by hand. See givenObjectType and givenRelation
func TestTemplateRestriction(t *testing.T) {
	rs := newFixture(t)

	assert.ErrorIs(t, rs.GetRestrictions(&restrictionHolder{
		// id:         "cannot make template from Template smartblock type",
		sbType:       coresb.SmartBlockTypeTemplate,
		layout:       model.ObjectType_basic,
		objectTypeID: bundle.TypeKeyTemplate.URL(),
	}).Object.Check(
		model.Restrictions_Template,
	), ErrRestricted)

	assert.ErrorIs(t, rs.GetRestrictions(&restrictionHolder{
		// id:         "cannot make template from set or collection layout",
		sbType:       coresb.SmartBlockTypePage,
		layout:       model.ObjectType_collection,
		objectTypeID: bundle.TypeKeyCollection.URL(),
	}).Object.Check(
		model.Restrictions_Template,
	), ErrRestricted)

	assert.ErrorIs(t, rs.GetRestrictions(&restrictionHolder{
		// id:         "cannot make template from space layout",
		sbType:       coresb.SmartBlockTypePage,
		layout:       model.ObjectType_space,
		objectTypeID: bundle.TypeKeySpace.URL(),
	}).Object.Check(
		model.Restrictions_Template,
	), ErrRestricted)

	assert.NoError(t, rs.GetRestrictions(&restrictionHolder{
		// id:         "make template from object with objectType added to space",
		sbType:       coresb.SmartBlockTypePage,
		layout:       model.ObjectType_basic,
		objectTypeID: bundle.TypeKeyContact.URL(),
	}).Object.Check(
		model.Restrictions_Template,
	))
}
