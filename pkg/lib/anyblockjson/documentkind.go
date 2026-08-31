package anyblockjson

// documentkind.go — which of the format's three grammars a document follows,
// and what to say when it is handed to the wrong reader (§2c, §2f, §13).
//
// A bundle holds three species of file: object documents (including types,
// relations and templates), one bundle index, and one property dictionary.
// They are different grammars, not variants of one, and until this file
// existed nothing in a document reliably told them apart:
//
//   - `version` is `const: 1` in all three schemas, so it discriminates
//     nothing;
//   - `$schema` is written by all three writers but is not required, so a
//     hand-authored document may carry none — 9 of 9 bundle files written by
//     small models against the schemas carried none;
//   - which left the FILENAME, and a filename is not part of the format. A
//     bundle unpacked flat, renamed, or streamed over an API has lost it.
//
// The cost was not theoretical. Handed to the object reader, a perfectly good
// index reported `/name: property "name" is not allowed` — on the field whose
// whole job is to name the space — and a dictionary reported `/properties:
// got array, want object` and `/installed: property "installed" is not
// allowed`, on its headline field. Each of those sends an author to repair a
// file that was already correct. This package's own eval harness hit it too.
//
// Requiring `$schema` would settle it, and is deliberately NOT what this
// does: the format is meant to be hand-authorable, and demanding a 60-byte
// URL as the price of a three-line document is a worse trade than reading the
// document's shape. So `$schema` stays optional — but when present it must
// name one of the three real schemas, which is what catches the invented
// `.../relation.schema.json` an author reached for.

import (
	"encoding/json"
	"strings"
)

// DocumentKind names which of the format's three grammars data follows.
//
// It reads the declared `$schema` first — that is the author's own statement
// and outranks any inference — and falls back to shape for a document that
// declares none. It never fails: anything it cannot place is KindObject, the
// grammar that covers every document a space actually contains, so a reader
// dispatching on this behaves exactly as it did before for ordinary
// documents.
func DocumentKind(data []byte) string {
	kind, _ := documentKindOf(data)
	return kind
}

// documentKindOf is DocumentKind with the part that matters to a reader
// deciding whether to OVERRIDE its caller: decided reports whether the
// document carried evidence at all. A document that declares no `$schema`
// and holds no member unique to one grammar is not evidence of anything —
// `{"version": 2}` is a legal start to all three — so a reader must take the
// caller's word for it rather than infer.
func documentKindOf(data []byte) (kind string, decided bool) {
	var probe struct {
		Schema     string          `json:"$schema"`
		Installed  json.RawMessage `json:"installed"`
		Properties json.RawMessage `json:"properties"`
		Manifest   json.RawMessage `json:"manifest"`
		Widgets    json.RawMessage `json:"widgets"`
		Entrypoint json.RawMessage `json:"entrypoint"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return KindObject, false
	}
	switch {
	case strings.HasSuffix(probe.Schema, "/index.schema.json"):
		return KindIndex, true
	case strings.HasSuffix(probe.Schema, "/properties.schema.json"):
		return KindPropertyDictionary, true
	case strings.HasSuffix(probe.Schema, "/object.schema.json"):
		return KindObject, true
	}
	// No declaration: infer from members no other grammar has. `properties`
	// is the one member two grammars share, and they disagree about its TYPE
	// — an object maps keys to values, a dictionary lists definitions — so
	// the array spelling is itself a discriminator.
	if len(probe.Installed) > 0 || isJSONArray(probe.Properties) {
		return KindPropertyDictionary, true
	}
	if len(probe.Manifest) > 0 || len(probe.Widgets) > 0 || len(probe.Entrypoint) > 0 {
		return KindIndex, true
	}
	return KindObject, false
}

// The three grammars DocumentKind names.
const (
	KindObject             = "object"
	KindIndex              = "index"
	KindPropertyDictionary = "property_dictionary"
)

// misroutedIssues reports a document handed to the reader for a different
// grammar. want is the grammar this reader implements.
//
// It runs BEFORE schema validation, so the author is told what the document
// is instead of being walked through the ways it fails to be something it
// never claimed to be.
func misroutedIssues(data []byte, want string) []Issue {
	got, decided := documentKindOf(data)
	if !decided || got == want {
		return nil
	}
	return []Issue{{Message: "this is " + articleFor(got) + ", not " +
		articleFor(want) + " — read it with " + readerFor(got) +
		". " + evidenceFor(data, got)}}
}

func articleFor(kind string) string {
	switch kind {
	case KindIndex:
		return "a bundle index"
	case KindPropertyDictionary:
		return "a property dictionary"
	default:
		return "an object document"
	}
}

func readerFor(kind string) string {
	switch kind {
	case KindIndex:
		return "UnmarshalIndex"
	case KindPropertyDictionary:
		return "UnmarshalPropertyDictionary"
	default:
		return "Unmarshal or Validate"
	}
}

// evidenceFor names what the verdict was read from, so an author who
// disagrees with it can see which member decided and fix that member rather
// than guessing.
func evidenceFor(data []byte, kind string) string {
	if _, schemaURL, ok := DetectFormat(data); ok && schemaURL != "" {
		return "It declares `$schema: " + schemaURL + "`."
	}
	switch kind {
	case KindIndex:
		return "It declares no `$schema`, and carries members only an index has."
	case KindPropertyDictionary:
		return "It declares no `$schema`, and carries `installed` or a `properties` ARRAY, " +
			"which an object document — where `properties` maps keys to values — cannot."
	default:
		return "It declares no `$schema`."
	}
}

func isJSONArray(raw json.RawMessage) bool {
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '[':
			return true
		default:
			return false
		}
	}
	return false
}
