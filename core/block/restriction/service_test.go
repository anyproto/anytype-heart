package restriction

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
)

func TestService_RestrictionsByObj(t *testing.T) {
	s := New()
	res := s.RestrictionsByObj(&testObj{tp: model.SmartBlockType_MarketplaceRelation})
	assert.NotEmpty(t, res.Object)
	assert.NotEmpty(t, res.Dataview)
}
