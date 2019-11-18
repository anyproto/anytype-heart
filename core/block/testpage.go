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

func testFloatValue(x float64) *types.Value {
	return &types.Value{
		Kind: &types.Value_NumberValue{NumberValue: x},
	}
}

var testBlocks = []*model.Block{
	{
		Id: testPageId,
		Fields: &types.Struct{
			Fields: map[string]*types.Value{
				"name": testStringValue("Test page"),
				"icon": testStringValue(":deciduous_tree:"),
			},
		},
		ChildrenIds: []string{"2", "3", "4", "5", "7", "12", "13"},
		Content: &model.BlockContentOfPage{
			Page: &model.BlockContentPage{Style: model.BlockContentPage_Empty},
		},
	},

	{
		Id: "2",
		Content: &model.BlockContentOfIcon{
			Icon: &model.BlockContentIcon{
				Name: ":deciduous_tree:",
			},
		},
	},

	{
		Id: "3",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "Test page",
				Style: model.BlockContentText_Title,
			},
		},
	},
	{
		Id: "4",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:   "Why Anytype is better vs. Notion?",
				Style:  model.BlockContentText_P,
				Marker: model.BlockContentText_Bullet,
			},
		},
	},
	{
		Id:          "5",
		ChildrenIds: []string{"6"},
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:   "Better looking and more pleasant to use:",
				Style:  model.BlockContentText_P,
				Marker: model.BlockContentText_Bullet,
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{
							From: 0,
							To:   40,
						},
						Type: model.BlockContentTextMark_Bold,
					},
				},
			},
		},
	},
	{
		Id: "6",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "Why? Notion and Airtable use one standard design for all databases - one fits all approach. It works well for a generic case - pages. Anytype customizes the design of a database for each object: page, task, file, link, music file, video, etc. It makes Anytype's tools look native, like apps not spreadsheets.",
				Style: model.BlockContentText_P,
			},
		},
	},

	{
		Id:          "7",
		ChildrenIds: []string{"8", "10"},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Row,
			},
		},
	},
	{
		Id:          "8",
		ChildrenIds: []string{"9"},
		Fields: &types.Struct{
			Fields: map[string]*types.Value{
				"width": testFloatValue(0.5),
			},
		},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		},
	},
	{
		Id: "9",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "Anytype",
				Style: model.BlockContentText_P,
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{
							From: 0,
							To:   7,
						},
						Type: model.BlockContentTextMark_Italic,
					},
				},
			},
		},
	},
	{
		Id:          "10",
		ChildrenIds: []string{"11"},
		Fields: &types.Struct{
			Fields: map[string]*types.Value{
				"width": testFloatValue(0.5),
			},
		},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		},
	},
	{
		Id: "11",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "Notion",
				Style: model.BlockContentText_P,
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{
							From: 0,
							To:   6,
						},
						Type: model.BlockContentTextMark_Italic,
					},
				},
			},
		},
	},

	{
		Id:          "12",
		ChildrenIds: []string{"13"},
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:   "Faster, better mobile apps",
				Style:  model.BlockContentText_P,
				Marker: model.BlockContentText_Bullet,
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{
							From: 0,
							To:   26,
						},
						Type: model.BlockContentTextMark_Bold,
					},
				},
			},
		},
	},
	{
		Id: "13",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "We are building native mobile apps. Notion has cross-platform apps. We've analyzed all reviews - the biggest drawback of Notion currently is the quality of their mobile apps. You can check out yourself - here.",
				Style: model.BlockContentText_P,
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{
							From: 204,
							To:   208,
						},
						Type:  model.BlockContentTextMark_U,
						Param: "https://play.google.com/store/apps/details?id=notion.id&hl=en",
					},
				},
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
