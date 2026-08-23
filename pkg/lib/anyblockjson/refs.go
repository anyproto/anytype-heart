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

import (
	"strings"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/domain"
)

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

// refNameSep splits an object reference from its informative name suffix.
// The FIRST occurrence splits (§9): the id half can never contain one, and
// the name half never does either once normalized, so first-vs-last is not a
// choice between behaviours — it is the same answer stated defensively.
const refNameSep = "#"

// maxRefNameLen bounds the suffix. The suffix is a glanceable hint, not an
// address, so a name that normalizes past the bound is truncated rather than
// dropped — truncation invents nothing here, unlike a key label (label.go),
// which IS an address and refuses instead.
const maxRefNameLen = 64

// splitRefName splits a reference at the first `#` into the id and the
// informative name. A reference with no `#`, and the degenerate `#…` whose
// id half would be empty, split into themselves and no name: import never
// invents an empty id out of a malformed reference.
func splitRefName(ref string) (id, name string) {
	if i := strings.Index(ref, refNameSep); i > 0 {
		return ref[:i], ref[i+1:]
	}
	return ref, ""
}

// trimRefName is the import half of the suffix: the id, with the informative
// name dropped unread (§9).
func trimRefName(ref string) string {
	id, _ := splitRefName(ref)
	return id
}

// refNameLabel normalizes a display name into the suffix grammar: the same
// identifier normalization key labels go through (label.go — letters of any
// script, digits, `_`, combining marks), bounded by maxRefNameLen. An empty
// answer means no suffix. The grammar admits no `#`, which is the writer's
// half of the split guarantee: a raw display name here would break the split
// from both ends.
func refNameLabel(name string) string {
	label := normalizeKeyLabel(name)
	if runes := []rune(label); len(runes) > maxRefNameLen {
		label = strings.TrimRight(string(runes[:maxRefNameLen]), "_")
	}
	return label
}

// isAccountIdentity reports whether s is a member's account identity — the
// base58 strkey form with its version byte and crc16 checksum intact
// (any-sync util/crypto). The checksum is what makes this a CLASSIFIER
// rather than a heuristic: no CID, bson id, or `_`-prefixed derived id can
// decode as one, so a bare identity in a reference slot is unambiguous.
func isAccountIdentity(s string) bool {
	if len(s) < 40 || len(s) > 64 {
		return false // cheap gate: real identities are 48 characters
	}
	_, err := crypto.DecodeAccountAddress(s)
	return err == nil
}

// foldParticipantRef is the export half of the participant fold (§9):
// `_participant_<SpaceId>_<identity>` becomes the bare identity. It folds
// ONLY what unfoldParticipantRef provably rebuilds — the space embedded in
// the id must be this run's own SpaceId (a cross-space participant ref would
// otherwise silently re-home on import), the identity must classify as one,
// and the composite must round-trip through domain.NewParticipantId
// byte-identically. With no SpaceId the fold is off in both directions:
// folding on export without the paired import being able to rebuild would
// land a bare identity where a composite belongs.
func (o Options) foldParticipantRef(id string) string {
	if o.SpaceId == "" || !strings.HasPrefix(id, domain.ParticipantPrefix) {
		return id
	}
	spaceId, identity, err := domain.ParseParticipantId(id)
	if err != nil || spaceId != o.SpaceId || !isAccountIdentity(identity) {
		return id
	}
	if domain.NewParticipantId(o.SpaceId, identity) != id {
		return id
	}
	return identity
}

// unfoldParticipantRef is the import half: a bare identity in an object
// reference slot rebuilds this space's participant id. Gated on the exact
// classifier the fold used, so unfold(fold(x)) == x and fold(unfold(y)) == y
// for every id either side touches.
func (o Options) unfoldParticipantRef(id string) string {
	if o.SpaceId == "" || !isAccountIdentity(id) {
		return id
	}
	return domain.NewParticipantId(o.SpaceId, id)
}

// objectRef renders one object reference for a document slot (§9): the
// participant fold first, then the informative `#name` suffix when the
// shape asks for it (Options.RefNames) and a resolver names the target. The
// resolver is asked about the STORED id — the composite participant id, not
// the folded identity — because that is the id the space indexes. With no
// resolver, or no name, the reference is written bare — never with a
// partial or invented suffix.
func (e *exporter) objectRef(id string) string {
	out := e.opts.foldParticipantRef(id)
	if !e.opts.RefNames || e.opts.ResolveObjectNames == nil || id == "" {
		return out
	}
	if !suffixableRef(id) {
		return out
	}
	name, ok := e.opts.ResolveObjectNames.ObjectName(id)
	if !ok {
		return out
	}
	if label := refNameLabel(name); label != "" {
		return out + refNameSep + label
	}
	return out
}

// suffixableRef reports the ids a name suffix belongs on. A date id and the
// missing-object sentinel already say everything they mean, and a dynamic
// filter placeholder (§6.2) is not an object id at all — a suffix on any of
// them would be decoration on a value some other layer must read verbatim.
//
// An id that already carries a `#` is excluded for a different reason: the
// suffix is only written where it is REVERSIBLE. No id this format writes
// contains one, but a snapshot is untrusted (§11) and may hold anything, and
// `x#y` + `#name` reads back as `x` — a different id from the one exported.
// Worse where the id half is empty: `#name` refuses to split at index 0
// (splitRefName), so import returns it whole and the next export appends
// again, one name per generation without bound. Writing such an id bare
// costs a caption on a reference that could not resolve anyway, and buys
// back §11 guarantee 2.
func suffixableRef(id string) bool {
	return !strings.HasPrefix(id, dateIdPrefix) &&
		id != missingObjectId &&
		!isFilterTemplate(id) &&
		!strings.Contains(id, refNameSep)
}

// dateIdPrefix marks a virtual date object id (pkg/lib/localstore/addr).
const dateIdPrefix = "_date_"

// missingObjectId is the dangling-reference sentinel stored details carry
// (pkg/lib/localstore/addr.MissingObject).
const missingObjectId = "_missing_object"

// objectRef reads one object reference back (§9): the informative suffix is
// trimmed at the first `#`, unread, and a bare identity unfolds into this
// space's participant id. Everything else passes verbatim, exactly as
// before the suffix existed — which is what keeps a bare id and a suffixed
// id importing identically.
func (imp *importer) objectRef(ref string) string {
	id := trimRefName(ref)
	// A bare account identity in a reference slot is the folded half of a
	// participant id (§9), and only a space can rebuild it. A reader that
	// names none would store the identity where the composite belongs — a
	// reference to an object that does not exist, in silence. The classifier
	// is exact (a strkey checksum), so the reader KNOWS this has happened
	// and says so, once, in build. It may not refuse: Validate never sees
	// Options, so refusing here would put the two surfaces into
	// disagreement over one document (§12 I2).
	if imp.opts.SpaceId == "" && isAccountIdentity(id) {
		imp.foldedUnrebuilt = true
		return id
	}
	return imp.opts.unfoldParticipantRef(id)
}
