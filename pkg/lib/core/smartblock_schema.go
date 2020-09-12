package core

import (
	"github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-library/schema"
	"github.com/santhosh-tekuri/jsonschema/v2"
)

const schemaURLPrefix = "https://anytype.io/schemas/"

type SmartBlockSchema interface {
	String() string // this method may cache the string representation so you don't need to marshal it every time
	URL() string
	Schema() *jsonschema.Schema
}

type smartBlockBaseSchema smartblock.SmartBlockType

func (s smartBlockBaseSchema) URL() string {
	switch smartblock.SmartBlockType(s) {
	case smartblock.SmartBlockTypePage:
		return schemaURLPrefix + "page"
	case smartblock.SmartBlockTypeProfilePage:
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
