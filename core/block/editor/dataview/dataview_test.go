package dataview

import (
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
	sch, err := schema.Get("https://anytype.io/schemas/page")
	require.NoError(t, err)

	relations := getDefaultRelations(sch)
	require.Len(t, relations, 3)

	require.Equal(t, relations[0].Id, "name")
	require.Equal(t, relations[0].IsVisible, true)
	require.Equal(t, relations[1].Id, "lastOpened")
	require.Equal(t, relations[1].IsVisible, true)

	require.Equal(t, relations[2].Id, "lastModified")
	require.Equal(t, relations[1].IsVisible, true)
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
				"id":   pbtypes.String("id1"),
				"name": pbtypes.String("name1_change"),
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
				"id":   pbtypes.String("id4"),
				"name": pbtypes.String("name4"),
			},
		}},
	}

	updated, removed, inserted := calculateEntriesDiff(a, b)

	require.Len(t, updated, 1)
	require.Len(t, removed, 1)
	require.Len(t, inserted, 1)

	require.Equal(t, b[0].Details, updated[0])
	require.Equal(t, "id3", removed[0])
	require.Equal(t, 2, inserted[0].position)
	require.Equal(t, []*types.Struct{b[2].Details}, inserted[0].entries)

}
