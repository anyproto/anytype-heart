package restriction

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func TestService_GetRestrictions(t *testing.T) {
	s := New(nil)
	res := s.GetRestrictions(&restrictionHolder{tp: model.SmartBlockType_BundledObjectType})
	assert.NotEmpty(t, res.Object)
}
