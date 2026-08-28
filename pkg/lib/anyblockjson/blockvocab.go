package anyblockjson

// blockvocab.go exports the §5 block-type vocabulary the way viewvocab.go
// exports §6.2's: as the single list the API layer consumes, so a surface
// that publishes the types to a generator cannot drift from what the codec
// reads. Here the derivation is literal — the names are read out of the
// embedded JSON Schema's own enum, so there is no second list to keep in
// step at all.

import (
	"encoding/json"
	"fmt"
	"slices"
)

// blockTypeNames is the §5 inventory, read out of the embedded schema at
// startup. It is a must-decode for the same reason regexp.MustCompile is: the
// input is a build-time embedded asset, and a schema that does not carry its
// own block-type inventory is not one this package can read documents with.
var blockTypeNames = mustSchemaBlockTypes()

func mustSchemaBlockTypes() []string {
	var doc struct {
		Defs struct {
			BlockCore struct {
				Properties struct {
					Type struct {
						Enum []string `json:"enum"`
					} `json:"type"`
				} `json:"properties"`
			} `json:"blockCore"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schemaJSON, &doc); err != nil {
		panic(fmt.Sprintf("anyblockjson: decode embedded schema: %v", err))
	}
	names := doc.Defs.BlockCore.Properties.Type.Enum
	if len(names) == 0 {
		panic("anyblockjson: the embedded schema publishes no block-type enum")
	}
	return names
}

// structuralBlockTypes are the §7 structural blocks: derivable from
// properties, dropped on export, and — at indent 0 — absorbed or dropped on
// import (topLevelBlocks reads this map, so which types are structural is
// stated once). They are part of the format's vocabulary but not of a body a
// caller can author: whatever is written into one, the block does not survive
// the round trip.
var structuralBlockTypes = map[string]bool{
	"title": true, "description": true, "featured_properties": true,
}

// transparentBlockTypes are the §7a transparent containers: types that carry
// containment and nothing else, so export writes their children in their
// place and import lifts them back out. They are part of the format's
// vocabulary — Validate must keep accepting what Unmarshal accepts, so
// `group` stays in BlockTypeNames and in the schema's `blockCore.type` enum
// — but no export produces one and nothing a caller writes into one
// survives, so they are not part of an authorable body either.
//
// This is deliberately NOT merged with structuralBlockTypes, which reads
// nearly the same and means the opposite: topLevelBlocks drops a structural
// block TOGETHER WITH ITS SUBTREE, where a container's whole point is that
// its subtree stays and only it goes.
var transparentBlockTypes = map[string]bool{
	"group": true,
}

// StructuralBlockType reports whether typ is a §7 structural block. Exported
// for the API v2 surfaces that publish an authorable vocabulary.
func StructuralBlockType(typ string) bool {
	return structuralBlockTypes[typ]
}

// TransparentBlockType reports whether typ is a §7a transparent container —
// a type a document may carry that resolves to no block of its own. Exported
// for the same API v2 surfaces as StructuralBlockType: a generator shown
// this type would write a block that vanishes on the next read.
func TransparentBlockType(typ string) bool {
	return transparentBlockTypes[typ]
}

// BlockTypeNames lists every §5 block type the format reads, in schema order
// (input aliases — heading4/header4 for heading3, equation for embed —
// included: they are values a document may legitimately carry).
func BlockTypeNames() []string {
	return slices.Clone(blockTypeNames)
}

// AuthorableBlockTypeNames is the vocabulary a document BODY can be written
// in: BlockTypeNames minus the §7 structural types, which import absorbs into
// properties or drops, and minus the §7a transparent containers, which import
// lifts away. Order follows BlockTypeNames.
func AuthorableBlockTypeNames() []string {
	out := make([]string, 0, len(blockTypeNames))
	for _, name := range blockTypeNames {
		if !structuralBlockTypes[name] && !transparentBlockTypes[name] {
			out = append(out, name)
		}
	}
	return out
}

// blockPropertyNames is the §5 block ATTRIBUTE inventory, read out of the same
// embedded schema — the shared core plus every conditional per-type branch.
// Must-decode for the same reason blockTypeNames is.
var blockPropertyNames = mustSchemaBlockProperties()

func mustSchemaBlockProperties() map[string]bool {
	var doc struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(schemaJSON, &doc); err != nil {
		panic(fmt.Sprintf("anyblockjson: decode embedded schema: %v", err))
	}
	names := map[string]bool{}
	var collect func(raw json.RawMessage)
	collect = func(raw json.RawMessage) {
		var node map[string]json.RawMessage
		if err := json.Unmarshal(raw, &node); err != nil {
			// an array of subschemas (allOf/anyOf/oneOf) — recurse into each
			var list []json.RawMessage
			if json.Unmarshal(raw, &list) == nil {
				for _, item := range list {
					collect(item)
				}
			}
			return
		}
		if props, ok := node["properties"]; ok {
			var fields map[string]json.RawMessage
			if json.Unmarshal(props, &fields) == nil {
				for name := range fields {
					names[name] = true
				}
			}
		}
		// only the composition keywords are descended: a property's own value
		// is a schema for that property, not another property inventory
		for _, keyword := range []string{"allOf", "anyOf", "oneOf", "if", "then", "else"} {
			if sub, ok := node[keyword]; ok {
				collect(sub)
			}
		}
	}
	for _, def := range []string{"blockCore", "block", "cellBlock"} {
		if raw, ok := doc.Defs[def]; ok {
			collect(raw)
		}
	}
	if len(names) == 0 {
		panic("anyblockjson: the embedded schema publishes no block properties")
	}
	return names
}

// BlockPropertyNames lists every ATTRIBUTE name the format's block schema
// knows — the shared core plus the per-type conditional branches — sorted.
//
// Exported for the API v2 surfaces that re-publish a subset of the block shape
// (the PATCH op schemas): those defs are additionalProperties:false, so a name
// they publish that the format does not know is a field no document can ever
// carry, and a constrained decoder shown it emits a block the codec rejects.
// Block attribute names are NOT key slots (§3) — this is the
// build-enforced form of that exclusion.
func BlockPropertyNames() []string {
	out := make([]string, 0, len(blockPropertyNames))
	for name := range blockPropertyNames {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// KnownBlockProperty reports whether the format's block schema knows this
// attribute name.
func KnownBlockProperty(name string) bool {
	return blockPropertyNames[name]
}
