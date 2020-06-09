package dataview

import (
	"strings"
	"testing"

	"github.com/anytypeio/go-anytype-library/schema"
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
