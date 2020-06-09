package core

import (
	"strings"

	"github.com/anytypeio/go-anytype-library/schema"
	"github.com/santhosh-tekuri/jsonschema/v2"
)

const schemaURLPrefix = "https://anytype.io/schemas/"

type SmartBlockSchema interface {
	String() string // this method may cache the string representation so you don't need to marshal it every time
	URL() string
	Schema() *jsonschema.Schema
}

type smartBlockBaseSchema SmartBlockType

var jsonSchemaCompiler *jsonschema.Compiler

func init() {
	jsonSchemaCompiler = jsonschema.NewCompiler()
	jsonSchemaCompiler.ExtractAnnotations = true

	// compile page first because others depends on it
	var keys = []string{"https://anytype.io/schemas/relation", "https://anytype.io/schemas/page"}
loop:
	for schemaURL, _ := range schema.SchemaByURL {
		for _, key := range keys {
			if schemaURL == key {
				continue loop
			}
		}
		keys = append(keys, schemaURL)
	}

	for _, schemaURL := range keys {
		err := jsonSchemaCompiler.AddResource(schemaURL, strings.NewReader(schema.SchemaByURL[schemaURL]))
		if err != nil {
			log.Fatalf("failed to compile %s: %s", schemaURL, err.Error())
		}
	}
}

func (s smartBlockBaseSchema) URL() string {
	switch SmartBlockType(s) {
	case SmartBlockTypePage:
		return schemaURLPrefix + "page"
	case SmartBlockTypeProfilePage:
		return schemaURLPrefix + "person"
	default:
		return ""
	}
}

func (s smartBlockBaseSchema) String() string {
	return schema.SchemaByURL[s.URL()]
}

func (s smartBlockBaseSchema) Schema() *jsonschema.Schema {
	// it is cached inside the jsonschema package
	return jsonschema.MustCompile(s.URL())
}
