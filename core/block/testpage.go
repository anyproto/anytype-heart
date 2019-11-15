package block

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
)

const testPageId = "testpage"

func testStringValue(x string) *types.Value {
	return &types.Value{
		Kind: &types.Value_StringValue{StringValue: x},
	}
}

var testBlocks = []*model.Block{
	{
		Id: testPageId,
		Fields: &types.Struct{
			Fields: map[string]*types.Value{
				"name": testStringValue("Contacts"),
				"icon": testStringValue(":family:"),
			},
		},
		ChildrenIds: []string{"3", "4"},
		Content: &model.BlockContentOfPage{
			Page: &model.BlockContentPage{Style: model.BlockContentPage_EMPTY},
		},
	},
	{
		Id: "3",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "Contacts 3",
				Style: model.BlockContentText_p,
			},
		},
	},
	{
		Id: "4",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "Contacts 4",
				Style: model.BlockContentText_p,
			},
		},
	},
}

type testPage struct {
	s *service
}

func (t *testPage) Open(b anytype.Block) error {
	event := &pb.Event{
		Message: &pb.EventMessageOfBlockShowFullscreen{
			BlockShowFullscreen: &pb.EventBlockShowFullscreen{
				RootId: t.GetId(),
				Blocks: testBlocks,
			},
		},
	}
	t.s.sendEvent(event)
	return nil
}

func (t *testPage) GetId() string {
	return testPageId
}

func (t *testPage) Type() smartBlockType {
	return smartBlockTypePage
}

func (t *testPage) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	return "", fmt.Errorf("can't create block in the test page")
}

func (t *testPage) Close() error {
	return nil
}
