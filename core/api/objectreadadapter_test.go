package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// fakeObjectGetter hands the adapter a prepared smartblock.
type fakeObjectGetter struct{ sb smartblock.SmartBlock }

func (f fakeObjectGetter) GetObject(context.Context, string) (smartblock.SmartBlock, error) {
	return f.sb, nil
}

func (f fakeObjectGetter) GetObjectByFullID(context.Context, domain.FullID) (smartblock.SmartBlock, error) {
	return f.sb, nil
}

// TestObjectReadAdapter covers the load-bearing C7/§8 invariant: the snapshot
// and the heads (from which the etag derives) are captured from the same
// smartblock under one lock, so the returned etag and content are consistent.
func TestObjectReadAdapter(t *testing.T) {
	sb := smarttest.New("obj1")
	sb.AddBlock(simple.New(&model.Block{Id: "obj1", ChildrenIds: []string{"p1"}}))
	sb.AddBlock(simple.New(&model.Block{Id: "p1",
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "hello"}}}))

	adapter := newObjectReadAdapter(fakeObjectGetter{sb: sb})
	read, err := adapter.ReadObject(context.Background(), "space1", "obj1")
	require.NoError(t, err)

	// the heads returned are exactly the smartblock's heads at read time
	assert.Equal(t, sb.GetDocInfo().Heads, read.Heads)
	require.NotNil(t, read.Snapshot)

	// the snapshot carries the state's content (proves it was read, not empty)
	var texts []string
	for _, b := range read.Snapshot.Blocks {
		if txt := b.GetText(); txt != nil {
			texts = append(texts, txt.Text)
		}
	}
	assert.Contains(t, texts, "hello")
}
