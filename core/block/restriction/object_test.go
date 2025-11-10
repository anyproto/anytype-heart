package restriction

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestService_ObjectRestrictionsById(t *testing.T) {
	all := lo.Keys(objRestrictAll)
	edit := lo.Keys(objRestrictEdit)
	dup := lo.Keys(objRestrictEditAndDuplicate)

	t.Run("anytype profile should have all restrictions", func(t *testing.T) {
		assert.ErrorIs(t, GetRestrictions(
			givenRestrictionHolder(smartblock.SmartBlockTypeAnytypeProfile, bundle.TypeKeyProfile),
		).Object.Check(all...), ErrRestricted)
	})

	t.Run("sets and collections should have edit restrictions", func(t *testing.T) {
		collection := givenRestrictionHolder(smartblock.SmartBlockTypePage, bundle.TypeKeyCollection)
		assert.ErrorIs(t, GetRestrictions(collection).Object.Check(edit...), ErrRestricted)
		set := givenRestrictionHolder(smartblock.SmartBlockTypePage, bundle.TypeKeySet)
		assert.ErrorIs(t, GetRestrictions(set).Object.Check(edit...), ErrRestricted)
	})

	t.Run("plain pages should not have any restrictions", func(t *testing.T) {
		page := givenRestrictionHolder(smartblock.SmartBlockTypePage, bundle.TypeKeyPage)
		for restriction := range objRestrictAll {
			assert.NoError(t, GetRestrictions(page).Object.Check(restriction))
		}
	})

	t.Run("system type", func(t *testing.T) {
		assert.ErrorIs(t, GetRestrictions(givenObjectType(bundle.TypeKeyObjectType)).Object.Check(
			model.Restrictions_Details,
			model.Restrictions_Delete,
		), ErrRestricted)
	})

	t.Run("system type restricted creation", func(t *testing.T) {
		assert.ErrorIs(t, GetRestrictions(givenObjectType(bundle.TypeKeyParticipant)).Object.Check(
			model.Restrictions_CreateObjectOfThisType,
		), ErrRestricted)
	})

	t.Run("ordinary type", func(t *testing.T) {
		assert.NoError(t, GetRestrictions(givenObjectType(bundle.TypeKeyDiaryEntry)).Object.Check(
			model.Restrictions_Details,
			model.Restrictions_Delete,
			model.Restrictions_CreateObjectOfThisType,
		))
	})

	t.Run("ordinary type has basic restrictions", func(t *testing.T) {
		assert.ErrorIs(t, GetRestrictions(givenObjectType(bundle.TypeKeyDiaryEntry)).Object.Check(
			model.Restrictions_Blocks,
			model.Restrictions_LayoutChange,
		), ErrRestricted)
	})

	t.Run("ordinary relation has basic restrictions", func(t *testing.T) {
		assert.ErrorIs(t, GetRestrictions(givenObjectType(bundle.TypeKeyDiaryEntry)).Object.Check(
			model.Restrictions_TypeChange,
		), ErrRestricted)
	})

	t.Run("bundled object types should have all restrictions", func(t *testing.T) {
		bundledType := givenRestrictionHolder(smartblock.SmartBlockTypeBundledObjectType, bundle.TypeKeyObjectType)
		assert.ErrorIs(t, GetRestrictions(bundledType).Object.Check(all...), ErrRestricted)
	})

	t.Run("ordinary relation", func(t *testing.T) {
		assert.NoError(t, GetRestrictions(givenRelation(bundle.RelationKeyAudioLyrics)).Object.Check(
			model.Restrictions_Delete,
			model.Restrictions_Relations,
			model.Restrictions_Details,
		))
	})

	t.Run("system relation", func(t *testing.T) {
		assert.ErrorIs(t, GetRestrictions(givenRelation(bundle.RelationKeyId)).Object.Check(
			model.Restrictions_Delete,
			model.Restrictions_Relations,
			model.Restrictions_Details,
		), ErrRestricted)
	})

	t.Run("bundled object types should have all restrictions", func(t *testing.T) {
		bundledRelation := givenRestrictionHolder(smartblock.SmartBlockTypeBundledRelation, bundle.TypeKeyRelation)
		assert.ErrorIs(t, GetRestrictions(bundledRelation).Object.Check(all...), ErrRestricted)
	})

	t.Run("chat should have edit and duplication restrictions", func(t *testing.T) {
		assert.ErrorIs(t, GetRestrictions(givenRestrictionHolder(smartblock.SmartBlockTypeChatDerivedObject, bundle.TypeKeyChatDerived)).Object.Check(
			dup...,
		), ErrRestricted)
	})
}

func TestTemplateRestriction(t *testing.T) {
	t.Run("cannot make template from Template smartblock type", func(t *testing.T) {
		template := givenRestrictionHolder(smartblock.SmartBlockTypeTemplate, bundle.TypeKeyTemplate)
		assert.ErrorIs(t, GetRestrictions(template).Object.Check(model.Restrictions_Template), ErrRestricted)
	})

	t.Run("we CAN make template from set or collection layout", func(t *testing.T) {
		collection := givenRestrictionHolder(smartblock.SmartBlockTypePage, bundle.TypeKeyCollection)
		set := givenRestrictionHolder(smartblock.SmartBlockTypePage, bundle.TypeKeySet)
		assert.NoError(t, GetRestrictions(collection).Object.Check(model.Restrictions_Template))
		assert.NoError(t, GetRestrictions(set).Object.Check(model.Restrictions_Template))
	})

	t.Run("cannot make template from space layout", func(t *testing.T) {
		space := givenRestrictionHolder(smartblock.SmartBlockTypePage, bundle.TypeKeySpace)
		assert.ErrorIs(t, GetRestrictions(space).Object.Check(model.Restrictions_Template), ErrRestricted)
	})

	t.Run("make template from object with objectType added to space", func(t *testing.T) {
		book := givenRestrictionHolder(smartblock.SmartBlockTypePage, bundle.TypeKeyBook)
		assert.NoError(t, GetRestrictions(book).Object.Check(model.Restrictions_Template))
	})
}

func TestArchivedObjectRestrictions(t *testing.T) {
	archived := &restrictionHolder{
		sbType: smartblock.SmartBlockTypePage,
		localDetails: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyIsArchived: domain.Bool(true),
		}),
	}

	rs := GetRestrictions(archived).Object
	for r := range objRestrictAll {
		if r == model.Restrictions_Delete {
			assert.NoError(t, rs.Check(r))
			continue
		}
		assert.Error(t, rs.Check(r))
	}
	assert.NoError(t, rs.Check(model.Restrictions_None))
	assert.NoError(t, rs.Check(model.Restrictions_CreateObjectOfThisType))
}
