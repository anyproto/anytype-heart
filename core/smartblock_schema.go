package core

import (
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

func init() {
	for schemaURL, schemaData := range schema.SchemaByURL {
		_, err := jsonschema.CompileString(schemaURL, schemaData)
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
	case SmartBlockTypeHome:
		return schemaURLPrefix + "home"
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
