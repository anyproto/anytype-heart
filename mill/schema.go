package mill

import (
	"bytes"
	"encoding/json"

	"github.com/golang/protobuf/jsonpb"
	"github.com/textileio/go-textile/pb"
	"github.com/textileio/go-textile/schema"
	"github.com/xeipuuv/gojsonschema"
)

var pbMarshaler = jsonpb.Marshaler{
	OrigName: true,
}

type Schema struct{}

func (m *Schema) ID() string {
	return "/schema"
}

func (m *Schema) Encrypt() bool {
	return false
}

func (m *Schema) Pin() bool {
	return true
}

func (m *Schema) AcceptMedia(media string) error {
	return accepts([]string{"application/json"}, media)
}

func (m *Schema) Options(add map[string]interface{}) (string, error) {
	return hashOpts(make(map[string]string), add)
}

func (m *Schema) Mill(input []byte, name string) (*Result, error) {
	var node pb.Node
	if err := jsonpb.Unmarshal(bytes.NewReader(input), &node); err != nil {
		return nil, err
	}

	if node.Mill == "" {
		if len(node.Links) == 0 {
			return nil, schema.ErrEmptySchema
		}

		for _, link := range node.Links {
			if !schema.ValidateMill(link.Mill) {
				return nil, schema.ErrSchemaInvalidMill
			}

			// extra check for json
			if link.Mill == "/json" {
				if link.JsonSchema == nil {
					return nil, schema.ErrMissingJsonSchema
				}
				if err := validateJsonSchema(pb.ToMap(link.JsonSchema)); err != nil {
					return nil, err
				}
			}
		}

		// ensure link steps are solvable
		if _, err := schema.Steps(node.Links); err != nil {
			return nil, err
		}

	} else {
		if !schema.ValidateMill(node.Mill) {
			return nil, schema.ErrSchemaInvalidMill
		}

		// extra check for json
		if node.Mill == "/json" {
			if node.JsonSchema == nil {
				return nil, schema.ErrMissingJsonSchema
			}
			if err := validateJsonSchema(pb.ToMap(node.JsonSchema)); err != nil {
				return nil, err
			}
		}
	}

	data, err := pbMarshaler.MarshalToString(&node)
	if err != nil {
		return nil, err
	}

	return &Result{File: []byte(data)}, nil
}

func validateJsonSchema(jschema map[string]interface{}) error {
	data, err := json.Marshal(&jschema)
	if err != nil {
		return err
	}

	loader := gojsonschema.NewStringLoader(string(data))

	if _, err := gojsonschema.NewSchema(loader); err != nil {
		return schema.ErrBadJsonSchema
	}

	return nil
}
