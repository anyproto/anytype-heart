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
//     display name normalized into an identifier grammar (refNameNormalize:
//     letters, digits, `_`, combining marks, nothing else). Key spellings
//     stopped being normalized when raw naming landed; the suffix still is,
//     because its grammar is what keeps the `#` split safe.
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
// base32 `[a-z2-7]`, participant ids base32+base58, `_ot`/`_br` ids are
// `[a-zA-Z0-9_]` across all 223 bundled keys, `_date_…`/`_missing_object`
// are fixed shapes; measured over 37,429 production documents: zero
// id-shaped values contain `#`) — and the name half is normalized through a
// grammar that admits no `#` either.

import (
	"strings"
	"unicode"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"
	"golang.org/x/text/unicode/norm"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

// ObjectExistenceResolver answers whether the space's store holds an object
// under an id — the question behind the missing-reference rule (§9): a
// reference to an object that does not exist in the SPACE is not written as
// if it did. It is an optional capability of Options.ResolveObjectNames,
// discovered by type assertion (the TypeResolver pattern, §2d): the resolver
// that can NAME an object — one point lookup on the space index — is the one
// that can also say whether the row is there at all, and a caller without it
// keeps a well-defined degradation: nothing is rewritten and nothing is
// dropped, because the absence of an answer is not evidence of absence.
//
// ObjectName is NOT this question and must never stand in for it: its ok is
// `name != ""`, so it answers "no" for an object that exists UNTITLED — and
// untitled objects are common. An export that conflated the two would
// rewrite live references to `_missing_object`.
//
// known=false means the resolver could not ask (a store failure): the caller
// treats the reference exactly as if the capability were absent. exists is
// a statement about the store's rows, tombstones included — a deleted
// object keeps an index row, so a reference to it is NOT missing: the id
// still means something in this space.
type ObjectExistenceResolver interface {
	ObjectExists(id string) (exists, known bool)
}

// ObjectDeletionResolver answers whether an id names an object the space
// DELETED — a tombstone: the index keeps a row stripped to its bookkeeping
// (`{id, spaceId, isDeleted, sync*}`) and nothing else.
//
// It is deliberately separate from ObjectExists, which counts a tombstone as
// existing and says so: "a deleted object keeps an index row, so a reference
// to it is NOT missing: the id still means something in this space". That
// rule stands for every reference slot but one. An ICON is the exception,
// because an icon is OPTIONAL: a link or a mention block must have a target,
// so a dangling one is rewritten to the sentinel rather than dropped, but an
// object with no icon is an ordinary object. Measured over a 77-space
// export, 134 bookmark documents shipped an icon pointing at a favicon whose
// file object had been deleted — every one of the 134 confirmed a tombstone
// in its own space's store.
//
// known=false means the resolver could not ask; the caller then treats the
// reference as live, so a store failure never removes an icon.
type ObjectDeletionResolver interface {
	ObjectDeleted(id string) (deleted, known bool)
}

// DroppedDeletedIconRef reports that an icon image reference names an object
// the space deleted, so export drops the icon and falls through to whatever
// channel is left (§2b) — the same fall-through an image that is not an
// object id already takes.
//
// Exported because snapshotdiff must apply the SAME predicate: `iconImage`
// is a DETAIL, so without this the comparator reads every dropped icon as
// data loss — the drift class that once produced 1,344 false failures in a
// single sweep (§11).
func DroppedDeletedIconRef(opts Options, id string) bool {
	if !isObjectIdShaped(id) {
		return false
	}
	res, ok := opts.ResolveObjectNames.(ObjectDeletionResolver)
	if !ok {
		return false
	}
	deleted, known := res.ObjectDeleted(id)
	return known && deleted
}

// isObjectIdShaped reports whether s parses as a content id (CID) — the
// shape of every object and file id a space actually mints. It is the gate
// that keeps the existence question OFF everything that is not a space
// store row's address: derived ids (`_date_…` is virtual, `_ot…`/`_br…`
// bundled urls and cross-space participant composites resolve against other
// authorities than this space's index), account identities, type and
// property keys, doc-local block ids — none of these parse as a CID, so
// none can be declared missing by a store that was never their authority.
// The cheap length gate mirrors isAccountIdentity's: no CID is shorter than
// 46 characters, and nearly every non-id fails there.
func isObjectIdShaped(s string) bool {
	if len(s) < 46 {
		return false
	}
	_, err := cid.Decode(s)
	return err == nil
}

// missingFromSpace reports that id names an object the wired store says the
// space does not hold — the only fact that may rewrite or drop a reference
// (§9). Three gates, each fail-safe toward "not missing": the id must be
// object-id-shaped (isObjectIdShaped — an id the space index was never the
// authority for cannot be missing from it), the existence capability must be
// wired (a package-only export has no store to ask, and "missing from this
// EXPORT" is not "missing from the space"), and the store must actually
// answer (known) — a store failure leaves the reference untouched.
func missingFromSpace(opts Options, id string) bool {
	if !isObjectIdShaped(id) {
		return false
	}
	res, ok := opts.ResolveObjectNames.(ObjectExistenceResolver)
	if !ok {
		return false
	}
	exists, known := res.ObjectExists(id)
	return known && !exists
}

// DroppedMissingObjectRef reports whether export drops entry from a
// LIST-valued reference slot — an objects/files property value (§3), a
// property document's `object_types` (§2d): the stored `_missing_object`
// sentinel, or an object id the wired store says the space does not hold.
// A list expresses absence by being shorter; singular slots rewrite to the
// sentinel instead (§9) and are not this predicate's business.
//
// Exported because snapshotdiff — the comparator behind the corpus sweep —
// must apply the SAME predicate to both sides, or every dropped-by-design
// entry reports as data loss (the drift class that once produced 1,344
// false failures in one sweep, §11). With no capability wired it drops
// nothing, sentinel included: a package-only export passes every entry
// through verbatim.
func DroppedMissingObjectRef(opts Options, entry string) bool {
	if entry == missingObjectId {
		_, ok := opts.ResolveObjectNames.(ObjectExistenceResolver)
		return ok
	}
	return missingFromSpace(opts, entry)
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

// refNameLabel normalizes a display name into the suffix grammar
// (refNameNormalize below), bounded by maxRefNameLen. An empty answer means
// no suffix. The grammar admits no `#`, which is the writer's half of the
// split guarantee: a raw display name here would break the split from both
// ends.
func refNameLabel(name string) string {
	label := refNameNormalize(name)
	if runes := []rune(label); len(runes) > maxRefNameLen {
		label = strings.TrimRight(string(runes[:maxRefNameLen]), "_")
	}
	return label
}

// refNameNormalize turns a display name into the `#name` suffix grammar —
// letters of any script, digits, `_`, combining marks — or "" when nothing
// is left to name.
//
// This is the identifier normalization that used to mint KEY labels
// (label.go), surviving here for its one remaining surface. Key spellings
// are raw names now and need no normalization at all; the ref suffix still
// does, because its grammar is what makes the `#` split safe — a raw
// display name may contain `#`, and the suffix must not. The rules are
// unchanged from the key-label era on purpose: the suffix is informative
// and trimmed unread, so nothing depends on its exact shape, and keeping
// the bytes stable keeps every already-written reference identical on its
// next export.
//
// Three decisions worth keeping stated, because each has a plausible
// alternative:
//
//   - **NFC, lowercase, separators collapse to `_`.** Two visually
//     identical names must not suffix differently between exports.
//   - **Combining marks are kept with their letter.** In Devanagari, Thai,
//     Bengali, Tamil, Khmer and Myanmar the vowels ARE marks; dropping them
//     does not shorten a word, it changes it — मिल/मूल/मल/मैल would all
//     become मल.
//   - **A leading `_` run is content, not a gap** — integrations namespace
//     themselves `__amemory_…` in their names — while interior runs
//     collapse and a trailing run trims; and a result that starts with a
//     digit or is a filter-grammar keyword takes a leading `_`, the escape
//     the suffix inherited from the key grammar and keeps for byte
//     stability.
func refNameNormalize(s string) string {
	if s == "" {
		return ""
	}
	lead := 0
	for _, r := range s {
		if r != '_' {
			break
		}
		lead++
	}
	var b strings.Builder
	gap := false // a separator run is pending, emitted only before the next letter
	for _, r := range norm.NFC.String(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if gap && b.Len() > 0 {
				b.WriteRune('_')
			}
			gap = false
			b.WriteRune(unicode.ToLower(r))
		case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r):
			// a mark cannot start a token, and one arriving with a pending
			// separator is malformed input, not a word
			if b.Len() > 0 && !gap {
				b.WriteRune(r)
			}
		default:
			gap = true // `_` included: runs collapse and edges trim
		}
	}
	label := strings.Repeat("_", lead) + b.String()
	if label == "" || strings.Trim(label, "_") == "" {
		return ""
	}
	if !filterstring.IsBareKey(label) {
		label = "_" + label
	}
	if !filterstring.IsBareKey(label) {
		// unreachable by construction — every rune is already an identPart,
		// so the only faults are a leading digit and a keyword, both cured
		// above. It is a guard rather than a path: IsBareKey is another
		// package's rule and may grow one, and the honest degradation is no
		// suffix at all.
		return ""
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

// FoldParticipantId is the exported form of the participant fold, for
// callers that must agree with the envelope id Marshal writes WITHOUT
// marshalling: the exporter's path plan names a document file by its
// envelope id (EXPORTER_DESIGN.md §1.3), and a participant document's
// envelope id is its folded bare identity. Same gates as the internal fold
// — no spaceId, a foreign space, a non-identity tail, or a composite that
// does not round-trip all decline and return id unchanged — which is
// exactly when Marshal keeps the composite as the envelope id, so the plan
// and the envelope cannot disagree.
func FoldParticipantId(spaceId, id string) string {
	return Options{SpaceId: spaceId}.foldParticipantRef(id)
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

// singularObjectRef renders a SINGULAR reference slot — a block's
// `object_id` (link, bookmark, file kinds, dataview) — under the
// missing-reference rule (§9): a target the space does not hold is written
// as the `_missing_object` sentinel, because omission cannot express "no
// target" here — only deleting the block could, and that would lose the
// fact that a link existed. A target the store DOES hold, the store cannot
// speak for (missingFromSpace's gates), or that already IS the sentinel
// passes to the ordinary objectRef untouched.
//
// The rewrite warns, naming the id: unlike the sentinel — which says
// nothing beyond "gone" — the id is real information, and the warning is
// its last appearance anywhere. After one round trip the slot is a
// fixpoint: the sentinel is kept as-is, so re-exports are byte-stable.
func (e *exporter) singularObjectRef(path, slot, id string) string {
	if missingFromSpace(e.opts, id) {
		e.warn(path, "%s %q names no object in this space and is written as %q — "+
			"the slot cannot say \"no target\" without deleting the block, and the sentinel "+
			"keeps the fact that a reference existed", slot, id, missingObjectId)
		return e.objectRef(missingObjectId)
	}
	return e.objectRef(id)
}

// droppedMissingListEntry is the LIST half of the missing-reference rule
// (§9): an objects/files property value entry, or an `object_types` entry,
// that the space does not hold is dropped — a list expresses absence by
// being shorter. The predicate is the exported DroppedMissingObjectRef, so
// the comparator applies exactly what export applied.
//
// Only a REAL id warns. A stored `_missing_object` sentinel drops silently:
// it carries nothing — which object it was is already gone — and the corpus
// holds ~990 of them in property values alone, which would triple a warning
// channel that was just cut down to what is worth reading (§12).
func (e *exporter) droppedMissingListEntry(path, id string) bool {
	if !DroppedMissingObjectRef(e.opts, id) {
		return false
	}
	if id != missingObjectId {
		e.warn(path, "%q names no object in this space and is dropped — "+
			"a list expresses absence by being shorter", id)
	}
	return true
}

// exportMarks applies the missing-reference rule to inline markup (§8, §9):
// a `<mention object_id="…">` whose target the space does not hold is
// rewritten to the `_missing_object` sentinel — a mention is a singular
// slot; dropping the mark would lose the fact that a mention existed while
// its text stayed. Copy-on-write: the snapshot's own marks are caller-owned
// state and are never mutated, and the common case — nothing missing —
// returns the input slice untouched. Object-link marks (`[label](anytype://…)`)
// keep their ids verbatim, as §9 states for them.
func (e *exporter) exportMarks(path string, marks []*model.BlockContentTextMark) []*model.BlockContentTextMark {
	out := marks
	copied := false
	for i, m := range marks {
		if m == nil || m.Type != model.BlockContentTextMark_Mention || !missingFromSpace(e.opts, m.Param) {
			continue
		}
		e.warn(path, "mention target %q names no object in this space and is written as %q — "+
			"the mention's own text stays; only its address is gone", m.Param, missingObjectId)
		if !copied {
			out = append([]*model.BlockContentTextMark(nil), marks...)
			copied = true
		}
		clone := *m
		clone.Param = missingObjectId
		out[i] = &clone
	}
	return out
}

// dateIdPrefix marks a virtual date object id (pkg/lib/localstore/addr).
const dateIdPrefix = "_date_"

// missingObjectId is the dangling-reference sentinel stored details carry
// (pkg/lib/localstore/addr.MissingObject).
const missingObjectId = "_missing_object"

// MissingObjectId is missingObjectId for the round-trip comparator, which
// lives in its own package and must apply the very sentinel export applies —
// the two cannot be allowed to spell it differently.
const MissingObjectId = missingObjectId

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
