package restriction

import (
	"testing"

	"github.com/stretchr/testify/assert"

	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

func TestService_GetRestrictions(t *testing.T) {
	s := New()
	res := s.GetRestrictions(&restrictionHolder{sbType: coresb.SmartBlockTypeBundledObjectType})
	assert.NotEmpty(t, res.Object)
}
