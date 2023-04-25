package restriction

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type testObj struct {
	id     string
	tp     model.SmartBlockType
	layout model.ObjectTypeLayout
}

func (t testObj) Id() string {
	return t.id
}

func (t testObj) Type() model.SmartBlockType {
	return t.tp
}

func (t testObj) Layout() (model.ObjectTypeLayout, bool) {
	return t.layout, t.layout != -1
}

func TestService_ObjectRestrictionsById(t *testing.T) {
	rest := New(nil)
	assert.ErrorIs(t, rest.ObjectRestrictionsByObj(testObj{
		id: "",
		tp: model.SmartBlockType_AnytypeProfile,
	}).Check(model.Restrictions_Blocks),
		ErrRestricted,
	)

	assert.ErrorIs(t, rest.ObjectRestrictionsByObj(testObj{
		id:     "",
		tp:     model.SmartBlockType_Page,
		layout: model.ObjectType_collection,
	}).Check(model.Restrictions_Blocks),
		ErrRestricted,
	)

	assert.NoError(t, rest.ObjectRestrictionsByObj(testObj{
		id: "",
		tp: model.SmartBlockType_Page,
	}).Check(model.Restrictions_Blocks))
}
