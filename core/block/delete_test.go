package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

func TestServiceDeleteObjectByFullID(t *testing.T) {
	id := domain.FullID{
		ObjectID: "obj",
		SpaceID:  "spc",
	}

	t.Run("close event should be sent on page deletion", func(t *testing.T) {
		// given
		fx := newFixture(t)
		spc := mock_clientspace.NewMockSpace(t)
		spc.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(func(id string, apply func(smartblock.SmartBlock) error) error {
			sb := smarttest.New(id)
			sb.SetType(coresb.SmartBlockTypePage)
			sb.SetRestrictions(restriction.Restrictions{Object: make(restriction.ObjectRestrictions, 0)})
			return apply(sb)
		})
		spc.EXPECT().DeleteTree(mock.Anything, mock.Anything).Return(nil).Once()

		fx.spaceService.EXPECT().Get(mock.Anything, mock.Anything).Return(spc, nil).Once()
		fx.eventSender.EXPECT().Broadcast(mock.Anything).RunAndReturn(func(e *pb.Event) {
			assert.Len(t, e.Messages, 1)
			msg := e.Messages[0].GetObjectClose()
			assert.NotNil(t, msg)
			assert.Equal(t, pb.EventObjectClose_Middle, msg.Closer)
			assert.Equal(t, id.ObjectID, msg.Id)
		})

		// when
		err := fx.DeleteObjectByFullID(id)

		// then
		assert.NoError(t, err)
	})
}
