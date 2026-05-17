package space

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
)

func newMockTechSpaceViewExists(t *testing.T, spaceId string, exists bool, err error) *mock_techspace.MockTechSpace {
	ts := mock_techspace.NewMockTechSpace(t)
	ts.EXPECT().SpaceViewExists(mock.Anything, spaceId).Return(exists, err)
	return ts
}

func TestComputeLazyMode(t *testing.T) {
	newSvc := func(preferred, tech string) *service {
		return &service{preferredSpaceId: preferred, techSpaceId: tech}
	}

	t.Run("empty preferred -> eager", func(t *testing.T) {
		s := newSvc("", "techId")
		assert.False(t, s.computeLazyMode(context.Background(), nil))
	})

	t.Run("preferred is tech -> eager", func(t *testing.T) {
		s := newSvc("techId", "techId")
		assert.False(t, s.computeLazyMode(context.Background(), nil))
	})

	t.Run("preferred resolvable -> lazy", func(t *testing.T) {
		ts := newMockTechSpaceViewExists(t, "space.x", true, nil)
		s := newSvc("space.x", "techId")
		assert.True(t, s.computeLazyMode(context.Background(), &clientspace.TechSpace{TechSpace: ts}))
	})

	t.Run("preferred not resolvable -> eager", func(t *testing.T) {
		ts := newMockTechSpaceViewExists(t, "space.x", false, nil)
		s := newSvc("space.x", "techId")
		assert.False(t, s.computeLazyMode(context.Background(), &clientspace.TechSpace{TechSpace: ts}))
	})
}
