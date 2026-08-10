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
	"title": true, "description": true, "featuredProperties": true,
}

// StructuralBlockType reports whether typ is a §7 structural block. Exported
// for the API v2 surfaces that publish an authorable vocabulary.
func StructuralBlockType(typ string) bool {
	return structuralBlockTypes[typ]
}

// BlockTypeNames lists every §5 block type the format reads, in schema order
// (input aliases — heading4/header4 for heading3, equation for embed —
// included: they are values a document may legitimately carry).
func BlockTypeNames() []string {
	return slices.Clone(blockTypeNames)
}

// AuthorableBlockTypeNames is the vocabulary a document BODY can be written
// in: BlockTypeNames minus the §7 structural types, which import absorbs into
// properties or drops. Order follows BlockTypeNames.
func AuthorableBlockTypeNames() []string {
	out := make([]string, 0, len(blockTypeNames))
	for _, name := range blockTypeNames {
		if !structuralBlockTypes[name] {
			out = append(out, name)
		}
	}
	return out
}
