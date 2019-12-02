package block

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
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
		ChildrenIds: []string{"2", "3", "4", "4a", "5", "7", "12", "13", "16", "19", "21", "22", "23", "28", "29", "30", "31", "32", "37"},
		Content: &model.BlockCoreContentOfPage{
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
				Text:  "Why Anytype is better vs. Notion?",
				Style: model.BlockContentText_Header1,
			},
		},
	},
	{
		Id: "4a",
		Content: &model.BlockCore{Content: &model.BlockCoreContentOfText{
			Text: &model.BlockContentText{
				Text:  "Test break for numbering check",
				Style: model.BlockContentText_Paragraph,
			},
		},
		},
		{
			Id:          "5",
			ChildrenIds: []string{"6"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Better looking and more pleasant to use:",
					Style: model.BlockContentText_Header1,
					Marks: &model.BlockContentTextMarks{
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
		},
		{
			Id: "6",

			Fields: &types.Struct{
				Fields: map[string]*types.Value{
					"lang": testStringValue("js"),
				},
			},
			Content: &model.BlockCoreContentOfText{
				Text: &model.BlockContentText{
					Text:  "Why? Notion and Airtable use one standard design for all databases - one fits all approach. It works well for a generic case - pages. Anytype customizes the design of a database for each object: page, task, file, link, music file, video, etc. It makes Anytype's tools look native, like apps not spreadsheets.",
					Style: model.BlockContentText_Paragraph,
					Marks: &model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{
							{
								Range: &model.Range{
									From: 0,
									To:   7,
								},
								Type:  model.BlockContentTextMark_TextColor,
								Param: "#ff0000",
							},
							{
								Range: &model.Range{
									From: 0,
									To:   7,
								},
								Type:  model.BlockContentTextMark_BackgroundColor,
								Param: "#00ff00",
							},
						},
					},
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
			ChildrenIds: []string{"9", "14"},
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
					Style: model.BlockContentText_Paragraph,
					Marks: &model.BlockContentTextMarks{
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
		},
		{
			Id: "14",
			Content: &model.BlockContentOfImage{
				Image: &model.BlockContentImage{
					LocalFilePath: "/Users/andrewsimachev/Pictures/P03STgPliLQ.jpg",
				},
			},
		},
		{
			Id:          "10",
			ChildrenIds: []string{"11", "15"},
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
					Style: model.BlockContentText_Paragraph,
					Marks: &model.BlockContentTextMarks{
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
		},
		{
			Id: "15",
			Content: &model.BlockContentOfImage{
				Image: &model.BlockContentImage{
					LocalFilePath: "/Users/andrewsimachev/Pictures/32.jpg",
				},
			},
		},
		{
			Id:          "16",
			ChildrenIds: []string{"17", "18"},
			Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_Row,
				},
			},
		},
		{
			Id: "17",
			Fields: &types.Struct{
				Fields: map[string]*types.Value{
					"width": testFloatValue(0.5),
				},
			},
			Content: &model.BlockContentOfImage{
				Image: &model.BlockContentImage{
					LocalFilePath: "/Users/andrewsimachev/Pictures/anigI4urVRs.jpg",
				},
			},
		},
		{
			Id: "18",
			Fields: &types.Struct{
				Fields: map[string]*types.Value{
					"width": testFloatValue(0.5),
				},
			},
			Content: &model.BlockContentOfImage{
				Image: &model.BlockContentImage{
					LocalFilePath: "/Users/andrewsimachev/Pictures/Photo11.jpg",
				},
			},
		},
		{
			Id:          "19",
			ChildrenIds: []string{"20"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Free with no storage and upload limits",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Bullet,
					Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 0,
								To:   38,
							},
							Type: model.BlockContentTextMark_Bold,
						},
					}},
				},
			},
		},
		{
			Id: "20",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Notion gives 1000 blocks with 5MB per upload for free. Usually a user is over this limit in a week. Anytype is free with no storage and upload limits (and we don't spend resources to offer that). Notion charges more for each member - Anytype can be free for team of any size. Free products grow faster.",
					Style: model.BlockContentText_Paragraph,
				},
			},
		},
		{
			Id: "21",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Some cool shit here:",
					Style: model.BlockContentText_Header3,
				},
			},
		},
		{
			Id: "22",
			Content: &model.BlockContentOfDiv{
				Div: &model.BlockContentDiv{},
			},
		},
		{
			Id:          "23",
			ChildrenIds: []string{"24", "27"},
			Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_Row,
				},
			},
		},
		{
			Id:          "24",
			ChildrenIds: []string{"25", "26"},
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
			Id: "25",
			Fields: &types.Struct{
				Fields: map[string]*types.Value{
					"name": testStringValue("Test page"),
					"icon": testStringValue(":deciduous_tree:"),
				},
			},
			Content: &model.BlockContentOfPage{
				Page: &model.BlockContentPage{Style: model.BlockContentPage_Empty},
			},
		},
		{
			Id: "26",
			Fields: &types.Struct{
				Fields: map[string]*types.Value{
					"name": testStringValue("Test page"),
					"icon": testStringValue(":deciduous_tree:"),
				},
			},
			Content: &model.BlockContentOfPage{
				Page: &model.BlockContentPage{Style: model.BlockContentPage_Empty},
			},
		},
		{
			Id: "27",
			Fields: &types.Struct{
				Fields: map[string]*types.Value{
					"width": testFloatValue(0.5),
					"name":  testStringValue("Test page"),
					"icon":  testStringValue(":deciduous_tree:"),
				},
			},
			Content: &model.BlockContentOfPage{
				Page: &model.BlockContentPage{Style: model.BlockContentPage_Empty},
			},
		},
		{
			Id: "28",
			Content: &model.BlockContentOfDiv{
				Div: &model.BlockContentDiv{},
			},
		},
		{
			Id: "29",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "",
					Style: model.BlockContentText_Header3,
				},
			},
		},
		{
			Id: "30",
			Content: &model.BlockContentOfVideo{
				Video: &model.BlockContentVideo{},
			},
		},
		{
			Id: "31",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "The menu plays 2 roles:",
					Style: model.BlockContentText_Header3,
				},
			},
		},
		{
			Id:          "32",
			ChildrenIds: []string{"33"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Is used to add a new block",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Bullet,
				},
			},
		},
		{
			Id:          "33",
			ChildrenIds: []string{"34", "35", "36"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "How it works:",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Bullet,
				},
			},
		},
		{
			Id: "34",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "User hits \"+\" button",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Number,
				},
			},
		},
		{
			Id: "35",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Add block menu appears",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Number,
				},
			},
		},
		{
			Id: "36",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "User can visually click on one of the options and the block of the corresponding type will appear",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Number,
				},
			},
		},
		{
			Id:          "37",
			ChildrenIds: []string{"38", "47"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Is used as a Power Tool that allows to call for almost any action - change color, turn block into another, delete block",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Bullet,
				},
			},
		},
		{
			Id:          "38",
			ChildrenIds: []string{"39", "40", "41", "42", "46"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "How it works",
					Style: model.BlockContentText_Paragraph,
					// Marker:     model.BlockContentText_Bullet,
					// Toggleable: true,
				},
			},
		},
		{
			Id: "39",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "User hits \"+\" button",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Number,
				},
			},
		},
		{
			Id: "40",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Add block menu appears",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Number,
				},
			},
		},
		{
			Id: "41",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "User starts typing \"page\"",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Number,
				},
			},
		},
		{
			Id:          "42",
			ChildrenIds: []string{"43", "44", "45"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Items connected to page appear:",
					Style: model.BlockContentText_Paragraph,
					// Marker:     model.BlockContentText_Number,
					// Toggleable: true,
				},
			},
		},
		{
			Id: "43",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Add block",
					Style: model.BlockContentText_Paragraph,
					Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 0,
								To:   9,
							},
							Type: model.BlockContentTextMark_Bold,
						},
					}},
				},
			},
		},
		{
			Id: "44",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "new page",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Bullet,
				},
			},
		},
		{
			Id: "45",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "new page",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Bullet,
				},
			},
		},
		{
			Id: "46",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "User chooses one from the list",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Number,
				},
			},
		},
		{
			Id:          "47",
			ChildrenIds: []string{"48", "49", "50", "51"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Alternative example",
					Style: model.BlockContentText_Paragraph,
					// Marker:     model.BlockContentText_Bullet,
					// Toggleable: true,
				},
			},
		},
		{
			Id: "48",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "User hits \"+\" button",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Number,
				},
			},
		},
		{
			Id: "49",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Add block menu appears",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Number,
				},
			},
		},
		{
			Id: "50",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "User starts typing \"turn into\"",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Number,
				},
			},
		},
		{
			Id:          "51",
			ChildrenIds: []string{"52", "53", "54", "55", "56"},
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Items connected to turn into appear:",
					Style: model.BlockContentText_Paragraph,
					// Marker:     model.BlockContentText_Number,
					// Toggleable: true,
				},
			},
		},
		{
			Id: "52",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "Turn into",
					Style: model.BlockContentText_Paragraph,
					Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 0,
								To:   9,
							},
							Type: model.BlockContentTextMark_Bold,
						},
					}},
				},
			},
		},
		{
			Id: "53",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "text",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Bullet,
				},
			},
		},
		{
			Id: "54",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "page",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Bullet,
				},
			},
		},
		{
			Id: "55",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "heading",
					// Marker: model.BlockContentText_Bullet,
				},
			},
		},
		{
			Id: "56",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  "list",
					Style: model.BlockContentText_Paragraph,
					// Marker: model.BlockContentText_Bullet,
				},
			},
		},
	}}

type testPage struct {
	s *service
}

func (t *testPage) UpdateTextBlock(id string, apply func(t *text.Text) error) error {
	return fmt.Errorf("can't update block in the test page")
}

func (t *testPage) Open(b anytype.Block) error {
	return nil
}

func (t *testPage) Init() {
	event := &pb.Event{
		Messages: []*pb.EventMessage{{&pb.EventMessageValueOfBlockShow{
			BlockShow: &pb.EventBlockShow{
				RootId: t.GetId(),
				Blocks: testBlocks,
			},
		}}},
	}
	t.s.sendEvent(event)
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
