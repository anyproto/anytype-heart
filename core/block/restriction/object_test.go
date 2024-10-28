package restriction

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestService_ObjectRestrictionsById(t *testing.T) {
	rs := service{}

	t.Run("anytype profile should have all restrictions", func(t *testing.T) {
		assert.ErrorIs(t, rs.GetRestrictions(givenRestrictionHolder(coresb.SmartBlockTypeAnytypeProfile, bundle.TypeKeyProfile)).Object.Check(
			objRestrictAll...,
		), ErrRestricted)
	})

	t.Run("sets and collections should have edit restrictions", func(t *testing.T) {
		collection := givenRestrictionHolder(coresb.SmartBlockTypePage, bundle.TypeKeyCollection)
		assert.ErrorIs(t, rs.GetRestrictions(collection).Object.Check(objRestrictEdit...), ErrRestricted)
		set := givenRestrictionHolder(coresb.SmartBlockTypePage, bundle.TypeKeySet)
		assert.ErrorIs(t, rs.GetRestrictions(set).Object.Check(objRestrictEdit...), ErrRestricted)
	})

	t.Run("plain pages should not have any restrictions", func(t *testing.T) {
		page := givenRestrictionHolder(coresb.SmartBlockTypePage, bundle.TypeKeyPage)
		for _, restriction := range objRestrictAll {
			assert.NoError(t, rs.GetRestrictions(page).Object.Check(restriction))
		}
	})

	t.Run("system type", func(t *testing.T) {
		assert.ErrorIs(t, rs.GetRestrictions(givenObjectType(bundle.TypeKeyObjectType)).Object.Check(
			model.Restrictions_Details,
			model.Restrictions_Delete,
		), ErrRestricted)
	})

	t.Run("system type restricted creation", func(t *testing.T) {
		assert.ErrorIs(t, rs.GetRestrictions(givenObjectType(bundle.TypeKeyParticipant)).Object.Check(
			model.Restrictions_CreateObjectOfThisType,
		), ErrRestricted)
	})

	t.Run("ordinary type", func(t *testing.T) {
		assert.NoError(t, rs.GetRestrictions(givenObjectType(bundle.TypeKeyDiaryEntry)).Object.Check(
			model.Restrictions_Details,
			model.Restrictions_Delete,
			model.Restrictions_CreateObjectOfThisType,
		))
	})

	t.Run("ordinary type has basic restrictions", func(t *testing.T) {
		assert.ErrorIs(t, rs.GetRestrictions(givenObjectType(bundle.TypeKeyDiaryEntry)).Object.Check(
			model.Restrictions_Blocks,
			model.Restrictions_LayoutChange,
		), ErrRestricted)
	})

	t.Run("ordinary relation has basic restrictions", func(t *testing.T) {
		assert.ErrorIs(t, rs.GetRestrictions(givenObjectType(bundle.TypeKeyDiaryEntry)).Object.Check(
			model.Restrictions_TypeChange,
		), ErrRestricted)
	})

	t.Run("bundled object types should have all restrictions", func(t *testing.T) {
		bundledType := givenRestrictionHolder(coresb.SmartBlockTypeBundledObjectType, bundle.TypeKeyObjectType)
		assert.ErrorIs(t, rs.GetRestrictions(bundledType).Object.Check(objRestrictAll...), ErrRestricted)
	})

	t.Run("ordinary relation", func(t *testing.T) {
		assert.NoError(t, rs.GetRestrictions(givenRelation(bundle.RelationKeyAudioLyrics)).Object.Check(
			model.Restrictions_Delete,
			model.Restrictions_Relations,
			model.Restrictions_Details,
		))
	})

	t.Run("system relation", func(t *testing.T) {
		assert.ErrorIs(t, rs.GetRestrictions(givenRelation(bundle.RelationKeyId)).Object.Check(
			model.Restrictions_Delete,
			model.Restrictions_Relations,
			model.Restrictions_Details,
		), ErrRestricted)
	})

	t.Run("bundled object types should have all restrictions", func(t *testing.T) {
		bundledRelation := givenRestrictionHolder(coresb.SmartBlockTypeBundledRelation, bundle.TypeKeyRelation)
		assert.ErrorIs(t, rs.GetRestrictions(bundledRelation).Object.Check(objRestrictAll...), ErrRestricted)
	})

	t.Run("chat should have edit and duplication restrictions", func(t *testing.T) {
		assert.ErrorIs(t, rs.GetRestrictions(givenRestrictionHolder(coresb.SmartBlockTypeChatObject, bundle.TypeKeyChat)).Object.Check(
			objRestrictEditDup...,
		), ErrRestricted)
	})
}

func TestTemplateRestriction(t *testing.T) {
	rs := service{}

	t.Run("cannot make template from Template smartblock type", func(t *testing.T) {
		template := givenRestrictionHolder(coresb.SmartBlockTypeTemplate, bundle.TypeKeyTemplate)
		assert.ErrorIs(t, rs.GetRestrictions(template).Object.Check(model.Restrictions_Template), ErrRestricted)
	})

	t.Run("cannot make template from set or collection layout", func(t *testing.T) {
		collection := givenRestrictionHolder(coresb.SmartBlockTypePage, bundle.TypeKeyCollection)
		assert.ErrorIs(t, rs.GetRestrictions(collection).Object.Check(model.Restrictions_Template), ErrRestricted)
	})

	t.Run("cannot make template from space layout", func(t *testing.T) {
		space := givenRestrictionHolder(coresb.SmartBlockTypePage, bundle.TypeKeySpace)
		assert.ErrorIs(t, rs.GetRestrictions(space).Object.Check(model.Restrictions_Template), ErrRestricted)
	})

	t.Run("make template from object with objectType added to space", func(t *testing.T) {
		book := givenRestrictionHolder(coresb.SmartBlockTypePage, bundle.TypeKeyBook)
		assert.NoError(t, rs.GetRestrictions(book).Object.Check(model.Restrictions_Template))
	})
}
