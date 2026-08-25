package anyblockjson

// wirenames.go — the wire spellings of the members the pre-freeze key/spelling
// split renamed (§2, §2e, §3), each defined exactly once.
//
// The word `key` used to mean two different things in one format: the envelope
// member held a STORED internal key (a bson id the app mints, or a bundled
// camelCase key), while a property definition's member held a document-facing
// SPELLING (`due_date`) — one word, two concepts, which is the §15 #14
// disease. The split gives each concept its own name:
//
//   - `internal_key` is the ONLY thing called a key that is a stored id — the
//     envelope identity of a definition document, and the optional stored-id
//     member of a property definition (export fidelity; an author never needs
//     to write one, because the app mints internal keys).
//   - `property` is the spelling a property definition states — the same
//     document-facing label every other key slot writes.
//   - the two legends say what their VALUES are: `property_internal_keys` and
//     `type_internal_keys` map a document's spellings to stored internal keys.
//
// Struct tags cannot reference constants, so the decoder tags in import.go,
// typeproperties.go and cmd/internal/anyblockbatch state the same strings;
// the schema files are the third statement. Tests pin all three against each
// other.
const (
	// memberInternalKey is the envelope's stored identity key on definition
	// documents (§2) and a property definition's optional stored-id member
	// (§2e). Minted by the app, written by export, never required from an
	// author.
	memberInternalKey = "internal_key"
	// memberDefinitionProperty is a property definition's spelling member
	// (§2e) — a document-facing spelling, not a stored key.
	memberDefinitionProperty = "property"
	// memberPropertyInternalKeys is the property legend: document spelling →
	// stored internal key (§3).
	memberPropertyInternalKeys = "property_internal_keys"
	// memberTypeInternalKeys is the same legend on the type namespace (§3).
	memberTypeInternalKeys = "type_internal_keys"
	// memberPropertySettings is a property document's definition group (§2d)
	// — the group that was born `relation_settings`. The v0.38 rename is the
	// same disease cured one word later: the product calls these things
	// properties, the format called the definition kind `relation`, and one
	// document said both (`featured_properties` the block type beside
	// `featured_relations` the key). One concept, one spelling (§15 #14).
	memberPropertySettings = "property_settings"
)
