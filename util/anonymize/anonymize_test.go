package anonymize

import (
	"testing"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
)

func TestText(t *testing.T) {
	in := "Some string with ютф. Symbols? http://123.com"
	out := Text(in)
	assert.Equal(t, utf8.RuneCountInString(in), utf8.RuneCountInString(out))
}

func TestStruct(t *testing.T) {
	in := &types.Struct{
		Fields: map[string]*types.Value{
			"string":      pbtypes.String("string value"),
			"string_list": pbtypes.StringList([]string{"one", "two"}),
			"number":      pbtypes.Int64(42),
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
		assert.False(t, in.Fields[k].Equal(out.Fields[k]))
	}
}
