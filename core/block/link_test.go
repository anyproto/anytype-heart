package block

import (
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_LinkWithPage(t *testing.T) {
	var targetId = "testTargetId"

	fx := newPageFixture(t)
	defer fx.ctrl.Finish()

	subscribed := make(chan struct{})
	mockBlock := &blockWrapper{MockBlock: testMock.NewMockBlock(fx.ctrl), blockMetaChanSubscribed: subscribed}
	mockBlock.EXPECT().GetCurrentVersionId()

	fx.serviceFx.anytype.EXPECT().GetBlock(targetId).Return(mockBlock, nil)

	newBlockId, err := fx.Create(pb.RpcBlockCreateRequest{
		ContextId: fx.pageId,
		TargetId:  "",
		Block: &model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: targetId,
					Style:         model.BlockContentLink_Page,
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, newBlockId)

	select {
	case <-time.After(time.Second):
		t.Errorf("subscribe timeout")
		return
	case <-subscribed:
	}

	newFields := &types.Struct{
		Fields: map[string]*types.Value{
			"test": testStringValue("test"),
		},
	}
	mockBlock.blockMetaChan <- &testMeta{fields: newFields}
	time.Sleep(time.Millisecond * 50)

	err = fx.Unlink(newBlockId)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 50)
	mockBlock.m.Lock()
	assert.True(t, mockBlock.cancelBlockMetaCalled)
	mockBlock.m.Unlock()

	require.NoError(t, fx.Close())
	fx.savedBlocks[newBlockId].GetLink().Fields.Equal(newFields)
}

type testMeta struct {
	id     string
	fields *types.Struct
}

func (t *testMeta) VersionId() string {
	return t.id
}

func (t *testMeta) Model() *model.BlockMetaOnly {
	return nil
}

func (t *testMeta) User() string {
	return ""
}

func (t *testMeta) Date() *types.Timestamp {
	return nil
}

func (t testMeta) ExternalFields() *types.Struct {
	return t.fields
}
