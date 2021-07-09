package anonymize

import (
	"testing"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
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
									},
								},
							},
						},
					},
				},
			},
			changeUpdate(&pb.EventMessage{
				Value: &pb.EventMessageValueOfBlockSetText{
					BlockSetText: &pb.EventBlockSetText{Id: "text", Text: &pb.EventBlockSetTextText{
						Value: "set text event",
					}},
				},
			}),
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
		in.Content[1].GetBlockUpdate().Events[0].GetBlockSetText().Text,
		out.Content[1].GetBlockUpdate().Events[0].GetBlockSetText().Text,
	)
}

func TestText(t *testing.T) {
	in := "Some string with ютф. Symbols? http://123.com"
	out := Text(in)
	assert.Equal(t, utf8.RuneCountInString(in), utf8.RuneCountInString(out))
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
