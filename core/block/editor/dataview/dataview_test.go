package dataview

import (
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-library/database"
	"github.com/anytypeio/go-anytype-library/schema"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/santhosh-tekuri/jsonschema/v2"
	"github.com/stretchr/testify/require"
)

func Test_getDefaultRelations(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	compiler.ExtractAnnotations = true
	err := compiler.AddResource("https://anytype.io/schemas/relation", strings.NewReader(schema.SchemaByURL["https://anytype.io/schemas/relation"]))
	require.NoError(t, err)

	err = compiler.AddResource("https://anytype.io/schemas/page", strings.NewReader(schema.SchemaByURL["https://anytype.io/schemas/page"]))
	require.NoError(t, err)

	sch := compiler.MustCompile("https://anytype.io/schemas/page")
	require.NoError(t, err)

	relations := getDefaultRelations(sch)
	require.Len(t, relations, 2)

	require.Equal(t, relations[0].Id, "name")
	require.Equal(t, relations[0].Visible, true)
	require.Equal(t, relations[1].Id, "isArchived")
	require.Equal(t, relations[1].Visible, true)
}

func Test_calculateEntriesDiff(t *testing.T) {
	a := []database.Entry{
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id1"),
				"name": pbtypes.String("name1"),
			},
		}},
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id2"),
				"name": pbtypes.String("name2"),
			},
		}},
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id3"),
				"name": pbtypes.String("name3"),
			},
		}},
	}

	b := []database.Entry{
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id0"),
				"name": pbtypes.String("name0"),
			},
		}},
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id2"),
				"name": pbtypes.String("name2"),
			},
		}},
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("after_id2"),
				"name": pbtypes.String("name2_after"),
			},
		}},
		{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("id3"),
				"name": pbtypes.String("name3_changed"),
			},
		}},
	}

	updated, removed, inserted := calculateEntriesDiff(a, b)

	require.Len(t, updated, 1)
	require.Len(t, removed, 1)
	require.Len(t, inserted, 2)

	require.Equal(t, b[3].Details, updated[0])
	require.Equal(t, "id1", removed[0])
	require.Equal(t, 0, inserted[0].position)
	require.Equal(t, []*types.Struct{b[0].Details}, inserted[0].entries)

	require.Equal(t, 2, inserted[1].position)
	require.Equal(t, []*types.Struct{b[2].Details}, inserted[1].entries)

}
