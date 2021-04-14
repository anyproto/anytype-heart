package restriction

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
)

type testObj struct {
	id string
	tp pb.SmartBlockType
}

func (t testObj) Id() string {
	return t.id
}

func (t testObj) Type() pb.SmartBlockType {
	return t.tp
}

func TestService_ObjectRestrictionsById(t *testing.T) {
	rest := New()
	assert.Equal(t, ErrRestricted, rest.ObjectRestrictionsById(testObj{
		id: "",
		tp: pb.SmartBlockType_Breadcrumbs,
	}).Check(model.ObjectRestriction_CreateBlock))
	assert.NoError(t, rest.ObjectRestrictionsById(testObj{
		id: "",
		tp: pb.SmartBlockType_Page,
	}).Check(model.ObjectRestriction_CreateBlock))
}
