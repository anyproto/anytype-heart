package block

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
)

const convertedMarker = "converted"

// convertingSmartTest stands in for an object whose blocks don't match its layout: its
// conversion marks the state. It records, in order, every apply that carries the marker (with
// its change type) and the session registration, so the test can see what reached the object
// and when.
type convertingSmartTest struct {
	*smarttest.SmartTest
	log          []string
	convertFlags []smartblock.ApplyFlag
}

func (c *convertingSmartTest) ConvertLayoutBlocks(s *state.State) bool {
	s.SetDetail(bundle.RelationKeyName, domain.String(convertedMarker))
	return true
}

func (c *convertingSmartTest) Apply(s *state.State, flags ...smartblock.ApplyFlag) error {
	introducesMarker := s.Details().GetString(bundle.RelationKeyName) == convertedMarker &&
		s.ParentState().Details().GetString(bundle.RelationKeyName) != convertedMarker
	if introducesMarker {
		c.log = append(c.log, "convert:"+s.GetChangeType().String())
		c.convertFlags = flags
	}
	return c.SmartTest.Apply(s, flags...)
}

func (c *convertingSmartTest) RegisterSession(ctx session.Context) {
	c.log = append(c.log, "register")
	c.SmartTest.RegisterSession(ctx)
}

func newOpenFixture(t *testing.T, spaceId, objectId string, readOnly bool) (*Service, *convertingSmartTest) {
	obj := &convertingSmartTest{SmartTest: smarttest.New(objectId)}
	spc := mock_clientspace.NewMockSpace(t)
	spc.EXPECT().IsReadOnly().Return(readOnly)
	spc.EXPECT().Do(objectId, mock.Anything).RunAndReturn(func(_ string, apply func(smartblock.SmartBlock) error) error {
		return apply(obj)
	})
	spaceSvc := mock_space.NewMockService(t)
	spaceSvc.EXPECT().Wait(mock.Anything, spaceId).Return(spc, nil)

	svc := New()
	svc.spaceService = spaceSvc
	svc.componentCtx = context.Background()
	return svc, obj
}

func TestService_OpenBlock_ConvertsLayoutBlocks(t *testing.T) {
	const (
		spaceId  = "space1"
		objectId = "obj1"
	)

	t.Run("member who can write -> conversion is applied as layout sync before the session registers", func(t *testing.T) {
		// given
		svc, obj := newOpenFixture(t, spaceId, objectId, false)
		want := []string{"convert:" + domain.ChangeTypeLayoutSync.String(), "register"}

		// when
		_, err := svc.OpenBlock(session.NewContext(), domain.FullID{SpaceID: spaceId, ObjectID: objectId}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, obj.log)
		assert.Equal(t, convertedMarker, obj.Details().GetString(bundle.RelationKeyName))
		// not undoable (the next undo must revert the user's edit, not this), clear of block
		// restrictions as every layout conversion, and with events for the sessions already
		// showing the object
		assert.Contains(t, obj.convertFlags, smartblock.NoHistory)
		assert.Contains(t, obj.convertFlags, smartblock.NoRestrictions)
		assert.NotContains(t, obj.convertFlags, smartblock.NoEvent)
	})

	t.Run("reader -> conversion is not applied", func(t *testing.T) {
		// given
		svc, obj := newOpenFixture(t, spaceId, objectId, true)
		want := []string{"register"}

		// when
		_, err := svc.OpenBlock(session.NewContext(), domain.FullID{SpaceID: spaceId, ObjectID: objectId}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, obj.log)
		assert.Empty(t, obj.Details().GetString(bundle.RelationKeyName))
	})
}
