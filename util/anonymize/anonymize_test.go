package anonymize

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/text"
)

func TestChange(t *testing.T) {
	changeUpdate := func(e *pb.EventMessage) *pb.ChangeContent {
		return &pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: []*pb.EventMessage{e},
				},
			},
		}
	}

	in := &pb.Change{
		Content: []*pb.ChangeContent{
			{
				Value: &pb.ChangeContentValueOfBlockCreate{
					BlockCreate: &pb.ChangeBlockCreate{
						Blocks: []*model.Block{
							{
								Id: "text",
								Content: &model.BlockContentOfText{
									Text: &model.BlockContentText{
										Text: "block create text",
										Marks: &model.BlockContentTextMarks{
											Marks: []*model.BlockContentTextMark{{
												Param: "https://randomsite.com/kosilica",
												Type:  model.BlockContentTextMark_Link,
											}},
										},
									},
								},
							},
						},
					},
				},
			},
			changeUpdate(event.NewMessage("", &pb.EventMessageValueOfBlockSetText{
				BlockSetText: &pb.EventBlockSetText{
					Id: "text",
					Text: &pb.EventBlockSetTextText{
						Value: "set text event",
					},
					Marks: &pb.EventBlockSetTextMarks{Value: &model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{{
							Param: "https://randomsite.com/kosilica",
							Type:  model.BlockContentTextMark_Link,
						}},
					}},
				},
			})),
		},
		Snapshot:  nil,
		FileKeys:  nil,
		Timestamp: 0,
	}

	out := Change(in)
	assert.NotEqual(
		t,
		in.Content[0].GetBlockCreate().Blocks[0].GetText().Text,
		out.Content[0].GetBlockCreate().Blocks[0].GetText().Text,
	)
	assert.NotEqual(
		t,
		in.Content[0].GetBlockCreate().Blocks[0].GetText().Marks.Marks[0].Param,
		out.Content[0].GetBlockCreate().Blocks[0].GetText().Marks.Marks[0].Param,
	)
	assert.NotEqual(
		t,
		in.Content[1].GetBlockUpdate().Events[0].GetBlockSetText().Text,
		out.Content[1].GetBlockUpdate().Events[0].GetBlockSetText().Text,
	)
	assert.NotEqual(
		t,
		in.Content[1].GetBlockUpdate().Events[0].GetBlockSetText().Marks.Value.Marks[0].Param,
		out.Content[1].GetBlockUpdate().Events[0].GetBlockSetText().Marks.Value.Marks[0].Param,
	)
}

func TestText(t *testing.T) {
	in := "Some string with ютф. Symbols? http://123.com"
	out := Text(in)
	assert.NotEqual(t, in, out)
	assert.Equal(t, text.UTF16RuneCountString(in), text.UTF16RuneCountString(out))
}

func TestStruct(t *testing.T) {
	in := &types.Struct{
		Fields: map[string]*types.Value{
			"string":      pbtypes.String("string value"),
			"string_list": pbtypes.StringList([]string{"onelistvalue", "twolistvalue"}),
			"number":      pbtypes.Int64(4242),
			"struct": &types.Value{Kind: &types.Value_StructValue{
				StructValue: &types.Struct{
					Fields: map[string]*types.Value{
						"v2": pbtypes.String("val in struct"),
					},
				},
			},
			},
		},
	}

	out := Struct(in)
	for k := range in.Fields {
		assert.False(t, in.Fields[k].Equal(out.Fields[k]), in.Fields[k], out.Fields[k])
	}
}
