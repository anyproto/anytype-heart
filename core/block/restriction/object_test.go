package restriction

import (
	"testing"

	"go.uber.org/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/testMock"
)

func TestService_ObjectRestrictionsById(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := testMock.NewMockObjectStore(ctrl)
	store.EXPECT().GetObjectType(gomock.Any()).AnyTimes()
	rest := New(nil, store)

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		id:         "",
		tp:         model.SmartBlockType_AnytypeProfile,
		objectType: "",
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
		id:         "",
		tp:         model.SmartBlockType_Page,
		layout:     model.ObjectType_collection,
		objectType: bundle.TypeKeyCollection.URL(),
	}).Object.Check(model.Restrictions_Blocks),
		ErrRestricted,
	)

	assert.NoError(t, rest.GetRestrictions(&restrictionHolder{
		id:         "",
		tp:         model.SmartBlockType_Page,
		objectType: bundle.TypeKeyPage.URL(),
	}).Object.Check(model.Restrictions_Blocks))

	assert.NoError(t, rest.GetRestrictions(&restrictionHolder{
		id:         bundle.TypeKeyDailyPlan.URL(),
		tp:         model.SmartBlockType_SubObject,
		layout:     model.ObjectType_objectType,
		objectType: bundle.TypeKeyObjectType.URL(),
	}).Object.Check(
		model.Restrictions_Details,
		model.Restrictions_Delete,
	))

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		id:         bundle.TypeKeyPage.URL(),
		tp:         model.SmartBlockType_SubObject,
		layout:     model.ObjectType_objectType,
		objectType: bundle.TypeKeyObjectType.URL(),
	}).Object.Check(
		model.Restrictions_Details,
		model.Restrictions_Delete,
	), ErrRestricted)

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		id:         bundle.TypeKeyBookmark.BundledURL(),
		tp:         model.SmartBlockType_BundledObjectType,
		layout:     model.ObjectType_objectType,
		objectType: bundle.TypeKeyObjectType.URL(),
	}).Object.Check(
		model.Restrictions_Duplicate,
		model.Restrictions_Relations,
	), ErrRestricted)

	assert.NoError(t, rest.GetRestrictions(&restrictionHolder{
		id:         bundle.RelationKeyImdbRating.String(),
		tp:         model.SmartBlockType_SubObject,
		layout:     model.ObjectType_relation,
		objectType: bundle.TypeKeyRelation.URL(),
	}).Object.Check(
		model.Restrictions_Delete,
		model.Restrictions_Relations,
		model.Restrictions_Details,
	))

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		id:         bundle.RelationKeyName.URL(),
		tp:         model.SmartBlockType_SubObject,
		layout:     model.ObjectType_relation,
		objectType: bundle.TypeKeyRelation.URL(),
	}).Object.Check(
		model.Restrictions_Delete,
		model.Restrictions_Relations,
		model.Restrictions_Details,
	), ErrRestricted)

	assert.ErrorIs(t, rest.GetRestrictions(&restrictionHolder{
		id:         bundle.RelationKeyId.BundledURL(),
		tp:         model.SmartBlockType_BundledRelation,
		layout:     model.ObjectType_relation,
		objectType: bundle.TypeKeyRelation.URL(),
	}).Object.Check(
		model.Restrictions_Duplicate,
	), ErrRestricted)
}

func TestTemplateRestriction(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := testMock.NewMockObjectStore(ctrl)
	store.EXPECT().GetObjectType(bundle.TypeKeyPage.URL()).Return(nil, objectstore.ErrObjectNotFound)
	store.EXPECT().GetObjectType(bundle.TypeKeyContact.URL()).Return(&model.ObjectType{}, nil)
	rs := New(nil, store)

	assert.ErrorIs(t, rs.GetRestrictions(&restrictionHolder{
		id:         "cannot make template from Template smartblock type",
		tp:         model.SmartBlockType_Template,
		layout:     model.ObjectType_basic,
		objectType: bundle.TypeKeyTemplate.URL(),
	}).Object.Check(
		model.Restrictions_Template,
	), ErrRestricted)

	assert.ErrorIs(t, rs.GetRestrictions(&restrictionHolder{
		id:         "cannot make template from set or collection layout",
		tp:         model.SmartBlockType_Page,
		layout:     model.ObjectType_collection,
		objectType: bundle.TypeKeyCollection.URL(),
	}).Object.Check(
		model.Restrictions_Template,
	), ErrRestricted)

	assert.ErrorIs(t, rs.GetRestrictions(&restrictionHolder{
		id:         "cannot make template from space layout",
		tp:         model.SmartBlockType_Page,
		layout:     model.ObjectType_space,
		objectType: bundle.TypeKeySpace.URL(),
	}).Object.Check(
		model.Restrictions_Template,
	), ErrRestricted)

	assert.ErrorIs(t, rs.GetRestrictions(&restrictionHolder{
		id:         "cannot make template from object with objectType not added to space",
		tp:         model.SmartBlockType_Page,
		layout:     model.ObjectType_basic,
		objectType: bundle.TypeKeyPage.URL(),
	}).Object.Check(
		model.Restrictions_Template,
	), ErrRestricted)

	assert.NoError(t, rs.GetRestrictions(&restrictionHolder{
		id:         "make template from object with objectType added to space",
		tp:         model.SmartBlockType_Page,
		layout:     model.ObjectType_basic,
		objectType: bundle.TypeKeyContact.URL(),
	}).Object.Check(
		model.Restrictions_Template,
	))
}
