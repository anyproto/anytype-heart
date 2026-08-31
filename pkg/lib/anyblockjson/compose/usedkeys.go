package compose

// usedkeys.go — the used-property-key census, at byte level. The dictionary
// is used-only (§2f): it names every property the bundle's documents
// actually reference, so somebody has to read every emitted document and say
// which keys those are. The cmd tools did this by re-reading written files
// (cmd/internal/anyblockbatch.UsedPropertyKeys); production cannot — a zip
// export has no read path to its own entries before Close — so the scan runs
// on the marshalled bytes BEFORE the write, and this byte-level form is the
// single implementation both sides share (design §1.1).

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// usedKeysDoc is the slice of a document the census reads: the legend, the
// properties object, and the §2a property definitions (which moved off the
// document root into `type_settings.property_definitions` — a
// scanner still reading the root would silently see no declarations).
type usedKeysDoc struct {
	PropertyKeys map[string]string          `json:"property_internal_keys"`
	Properties   map[string]json.RawMessage `json:"properties"`
	TypeSettings *struct {
		PropertyDefinitions []usedKeysDef `json:"property_definitions"`
	} `json:"type_settings"`
}

// usedKeysDef is one §2e entry's identity: its `property` spelling, else
// its `internal_key`. A name-only entry states no identity and is skipped —
// the codec derives the spelling from the name at import.
type usedKeysDef struct {
	Property    string `json:"property"`
	InternalKey string `json:"internal_key"`
}

// UsedPropertyKeysFromBytes reports every STORED property key one document
// references — its contribution to the population the dictionary's
// `properties` list names (§2f, used-only). Two slots count as a reference,
// resolved through the same chain every scan runs (the document's own
// `property_internal_keys` legend, then the bundled table, then verbatim):
// a `properties` member, and a
// `type_settings.property_definitions[].property`. A dataview's column list
// is deliberately NOT one — it is a per-view cache carrying its own inline
// format (§6.2), so a key that appears there and nowhere else gives a
// reader nothing to look up.
func UsedPropertyKeysFromBytes(doc []byte) (map[string]bool, error) {
	var d usedKeysDoc
	if err := json.Unmarshal(doc, &d); err != nil {
		return nil, fmt.Errorf("parse document: %w", err)
	}
	out := map[string]bool{}
	for k := range d.Properties {
		// id and type are envelope facts, skipped on the SPELLING the way
		// the codec skips them (importer.build)
		if k == "id" || k == "type" {
			continue
		}
		out[resolveUsedTerm(d.PropertyKeys, k)] = true
	}
	if d.TypeSettings != nil {
		for _, def := range d.TypeSettings.PropertyDefinitions {
			switch {
			case def.Property != "":
				out[resolveUsedTerm(d.PropertyKeys, def.Property)] = true
			case def.InternalKey != "":
				// a stated internal key IS the stored key and skips the
				// ladder — a stored id is its own address (§2e)
				out[def.InternalKey] = true
			}
		}
	}
	return out, nil
}

// resolveUsedTerm binds one property term to the stored key it names,
// running the §3 chain: the document's own legend, then the bundled derived
// table, then verbatim (BundledKeyVocabulary's pass-through IS chain step 4).
func resolveUsedTerm(legend map[string]string, term string) string {
	if key, ok := legend[term]; ok && key != "" {
		return key
	}
	key, _ := anyblockjson.BundledKeyVocabulary{}.PropertyKey(term)
	return key
}
