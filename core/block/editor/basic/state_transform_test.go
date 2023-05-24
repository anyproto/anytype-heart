package basic

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCutBlocks(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}}))
	sb.AddBlock(simple.New(&model.Block{Id: "1", ChildrenIds: []string{"1.1"}}))
	sb.AddBlock(simple.New(&model.Block{Id: "1.1", ChildrenIds: []string{"1.1.1"}}))
	sb.AddBlock(simple.New(&model.Block{Id: "1.1.1"}))

	s := sb.NewState()

	blockIds := []string{"1", "1.1", "1.1.1"}
	blocks := CutBlocks(s, blockIds)

	require.NoError(t, sb.Apply(s))

	var gotIds []string
	for _, b := range blocks {
		gotIds = append(gotIds, b.Model().Id)
	}
	assert.ElementsMatch(t, blockIds, gotIds)

	var restIds []string
	for _, b := range sb.Blocks() {
		restIds = append(restIds, b.Id)
	}
	assert.ElementsMatch(t, []string{"test"}, restIds)
}
