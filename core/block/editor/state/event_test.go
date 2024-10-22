package state

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
)

func TestSortMessages(t *testing.T) {
	input := []*pb.EventMessage{
		{Value: &pb.EventMessageValueOfBlockSetText{}},
		{Value: &pb.EventMessageValueOfBlockDelete{}},
		{Value: &pb.EventMessageValueOfBlockSetVerticalAlign{}},
		{Value: &pb.EventMessageValueOfBlockAdd{}},
		{Value: &pb.EventMessageValueOfObjectDetailsUnset{}},
		{Value: &pb.EventMessageValueOfBlockSetBackgroundColor{}},
		{Value: &pb.EventMessageValueOfObjectDetailsAmend{}},
		{Value: &pb.EventMessageValueOfBlockSetChildrenIds{}},
		{Value: &pb.EventMessageValueOfBlockSetAlign{}},
		{Value: &pb.EventMessageValueOfObjectDetailsSet{}},
		{Value: &pb.EventMessageValueOfBlockSetChildrenIds{}},
		{Value: &pb.EventMessageValueOfBlockDataviewViewDelete{}},
		{Value: &pb.EventMessageValueOfBlockDataviewViewSet{}},
	}

	want := WrapEventMessages(false, []*pb.EventMessage{
		{Value: &pb.EventMessageValueOfBlockAdd{}},
		{Value: &pb.EventMessageValueOfBlockDelete{}},
		{Value: &pb.EventMessageValueOfBlockSetChildrenIds{}},
		{Value: &pb.EventMessageValueOfBlockSetChildrenIds{}},
		{Value: &pb.EventMessageValueOfObjectDetailsSet{}},
		{Value: &pb.EventMessageValueOfObjectDetailsAmend{}},
		{Value: &pb.EventMessageValueOfObjectDetailsUnset{}},
		{Value: &pb.EventMessageValueOfBlockDataviewViewSet{}},
		{Value: &pb.EventMessageValueOfBlockDataviewViewDelete{}},
		{Value: &pb.EventMessageValueOfBlockSetText{}},
		{Value: &pb.EventMessageValueOfBlockSetVerticalAlign{}},
		{Value: &pb.EventMessageValueOfBlockSetBackgroundColor{}},
		{Value: &pb.EventMessageValueOfBlockSetAlign{}},
	})

	got := WrapEventMessages(false, input)
	sortEventMessages(got)

	assert.Equal(t, want, got)
}
