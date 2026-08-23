package anyblockjson

// refs.go — object references (§9): the informative `#name` suffix and the
// participant fold.
//
// An object reference in this format is a full id, always (§9a deleted the
// compaction legend). Two amendments make one readable without ceasing to be
// an address:
//
//   - **The `#name` suffix.** A reference MAY carry `#<name>` after the id —
//     `bafyrei…#local_first_ux` — where the name is the referenced object's
//     display name normalized through the same identifier grammar key labels
//     use (label.go): letters, digits, `_`, combining marks, nothing else.
//     The suffix is INFORMATIVE ONLY: import trims it at the first `#` and
//     never resolves it, so a stale name costs nothing and two objects
//     sharing one name collide on nothing. It exists so a human or a model
//     reading a document sees what a reference points at instead of a
//     59-character CID. A bare id with no suffix is equally valid, and is
//     what a writer with no name in hand writes.
//
//   - **The participant fold.** `_participant_<spaceId>_<identity>` is a
//     derived id: the space id is the document's own space restated, and the
//     identity is the whole of the content. When Options.SpaceId names the
//     space, export folds the composite down to the bare identity and import
//     rebuilds the composite (domain.NewParticipantId) — 135 characters down
//     to 48, and the same member re-addresses correctly when a document
//     crosses spaces, because the reader rebuilds against ITS space.
//
// The split at `#` is unconditional and safe from both ends, verified rather
// than assumed: no id form this format writes can contain `#` (CIDs are
// base32 `[a-z2-7]`, participant ids base32+base58, `_ot`/`_br` slugs are
// `[a-zA-Z0-9_]` across all 223 bundled keys, `_date_…`/`_missing_object`
// are fixed shapes; measured over 37,429 production documents: zero
// id-shaped values contain `#`) — and the name half is normalized through a
// grammar that admits no `#` either.

// ObjectNameResolver names an object for the informative reference suffix
// (§9). It is the object-namespace sibling of ParticipantResolver, and it is
// export-only: import trims the suffix without ever asking anyone.
//
// A resolver that cannot name an id returns false and the reference is
// written bare — never with a partial or invented suffix. An empty or
// whitespace name is treated as no name at the seam (refNameLabel), the same
// discipline the participant seam applies, so an implementation answering
// ("", true) cannot put a dangling `#` on every reference in an export.
type ObjectNameResolver interface {
	ObjectName(id string) (string, bool)
}
