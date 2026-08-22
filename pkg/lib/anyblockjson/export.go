package anyblockjson

// export.go serializes a snapshot into canonical AnyBlock JSON (§2–§7,
// §9–§9a).

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// FormatResolver reports the format of a property key, when known. Bundle
// properties are resolved internally; the resolver covers custom keys (§3).
type FormatResolver func(key domain.RelationKey) (model.RelationFormat, bool)

// OptionResolver maps select/multiSelect option ids to names on export and
// names to ids on import (creating options is the import wiring's job, §3).
//
// OptionName has TWO duties, and the second one is not an export call:
//
//  1. export — what is this option id called? The name is what the document
//     writes for the value (§3), and the id it stood for rides along in the
//     `option_ids` legend (§9a).
//  2. import — is this id a live option of this relation HERE? That is the
//     liveness question every `option_ids` entry is checked against
//     (optionrefs.go): the legend is a hint, honoured only for an id the
//     target space still serves under that key, and OptionName answering is
//     precisely what "still serves" means. Nothing else asks it, so a
//     resolver's answer here is the whole of the check.
//
// A resolver that cannot answer OptionName gives up the legend entirely: it
// says "no id is live", so every entry fails step 1 of §3's chain and every
// value falls back to name resolution, exactly as it did before `option_ids`
// existed — including the two losses the legend was added to close, a name
// shared by two options of one property (the first one answers) and an option
// renamed since the export (nothing answers, and the wiring mints a second
// option under the stale name). That is a legitimate position for a resolver
// with no option store to consult, and returning false is then the honest
// answer; it is not a stub to leave in place unexamined, because it disables
// a feature for everything that reader imports, silently. `OptionId` without
// `OptionName` is the shape to look at twice.
type OptionResolver interface {
	OptionName(key domain.RelationKey, id string) (string, bool)
	OptionId(key domain.RelationKey, name string) (string, bool)
}

// Options configures Marshal and Unmarshal (§13).
type Options struct {
	ResolveFormat     FormatResolver   // optional; nil = bundle-only resolution (§3)
	ResolveOptions    OptionResolver   // optional; nil = option values pass through as ids
	ResolveProperties PropertyResolver // optional; nil = type documents keep raw recommended-relation ids (§2a)
	Keys              KeyVocabulary    // optional; nil = BundledKeyVocabulary (the derived table — keyvocab.go)
	// Legend is the enclosing document's three legends, for the FRAGMENT
	// entry points only (fragment.go, filters.go, BuildRecommendedLists).
	// Marshal and Unmarshal ignore it: a whole document carries its own.
	//
	// A fragment has no envelope, so it has no legend of its own — and
	// without one the §3 chain loses its first and highest step. A block cut
	// out of a document that said `{"priority": "6a32d485…"}` resolved
	// `priority` through the READER's vocabulary alone, which is the exact
	// misresolution the legend exists to prevent, reintroduced at the seam
	// that edits live objects. Hand the fragment the document's legend and
	// chain step 1 is back.
	Legend             Legend
	OmitIds            bool          // export only: drop every id (§9)
	CompactBlockLabels bool          // export only: relabel doc-local block/row/column/view ids to short suffixes (§9a; lossy, legend-less)
	CompactIds         bool          // export only: alias for CompactBlockLabels — object refs are never compacted (§9a)
	GenerateId         func() string // import only: id generator for missing ids; nil = random 24-hex
	NormalizeIndent    bool          // import only: clamp over-deep indents instead of rejecting (§4)
	OnWarning          func(Issue)   // optional sink for warning-grade issues, both directions (indent clamps, unrepresentable dates, …)
}

// Legend carries the three legends of the document a fragment was cut out
// of, so the fragment entry points can run the §3 chain from step 1 instead
// of starting at the reader's vocabulary. The field names and the semantics
// are the envelope's: `property_keys` and `type_keys` values are
// AUTHORITATIVE, an `option_ids` value is a liveness-checked hint (§3).
//
// The zero value is "no legend", which is what a caller that assembled the
// fragment itself has, and is the behaviour every fragment entry point had
// before this field existed.
type Legend struct {
	// PropertyKeys maps a property spelling to the stored relation key it
	// names (§3) — the enclosing document's `property_keys`.
	PropertyKeys map[string]string
	// TypeKeys is the same for the type namespace — the enclosing document's
	// `type_keys`.
	TypeKeys map[string]string
	// OptionIds maps {property spelling: {option name: option id}} — the
	// enclosing document's `option_ids` (§9a).
	OptionIds map[string]map[string]string
}

// empty reports whether the legend says nothing.
func (l Legend) empty() bool {
	return len(l.PropertyKeys) == 0 && len(l.TypeKeys) == 0 && len(l.OptionIds) == 0
}

// fragmentDoc is the synthetic envelope a fragment entry point resolves
// against: no blocks, no properties, just the legend the caller handed over.
// Every fragment importer used to build `&jsonDoc{}` here, and an empty
// legend makes chain step 1 unconditionally silent.
func (o Options) fragmentDoc() *jsonDoc {
	return &jsonDoc{
		PropertyKeys: o.Legend.PropertyKeys,
		TypeKeys:     o.Legend.TypeKeys,
		OptionIds:    o.Legend.OptionIds,
	}
}

// compactBlockLabels reports whether doc-local id relabeling is on.
func (o Options) compactBlockLabels() bool { return o.CompactBlockLabels || o.CompactIds }

const (
	compactIdMinLen = 5

	// well-known internal keys that get lifted into the envelope
	detailKeyId   = "id"
	detailKeyType = "type"
	// typeKeyTemplate is the type key `kind: "template"` names. Nothing
	// RESOLVES to it any more: `kind` is the sole authority on whether a
	// document is a template (§2), so the spelling `template` is an ordinary
	// type term that a legend or a vocabulary may bind wherever it likes.
	//
	// Two raw string comparisons survive, and neither consults a vocabulary:
	// buildDoc keeps `kind` explicit when the term it is about to write is
	// literally `template` (so a Page never emits a document that trips the
	// legacy refusal), and validate.go refuses a document with no `kind`
	// whose `type` is literally `template` (the pre-v0.22 spelling, §10).
	typeKeyTemplate = "template"
	storeKeyItems   = "objects"
	// codeLangField is the internal fields key holding a code block's
	// language (§5.1)
	codeLangField = "lang"
)

// propertiesKeptOnExport are the internal properties the importer
// meaningfully preserves; everything else in LocalAndDerivedRelationKeys is
// stripped (§3).
var propertiesKeptOnExport = map[string]bool{
	"createdDate":      true,
	"lastModifiedDate": true,
	"creator":          true,
	"isFavorite":       true,
	"isArchived":       true,
	"resolvedLayout":   true,
}

// wellKnownPropertyOrder puts the §3 magic keys first in the properties
// object; all remaining keys follow alphabetically (canonical order
// decision).
var wellKnownPropertyOrder = []string{"name", "description", "iconEmoji", "iconImage"}

// MarshalPropertyValue converts one property value to its JSON form under
// the §3 rules (dates → RFC 3339, select options → names, object/file →
// id lists, scalars wrap into lists for list-shaped formats). It is the
// row-level building block for API list surfaces that carry requested
// property values (APIV2.md C5) without a full document export. The result
// marshals with encoding/json.
//
// The second return is this value's share of the `option_ids` legend —
// {option name: option id} for THIS key (§9a) — and it is not optional
// bookkeeping. A select value is written as its option NAME, and a name is
// not an address: two options of one property may share it, and an option
// renamed between this call and the read makes the wiring mint a second
// option under the stale name. The whole-document export has carried the ids
// since §9a; this entry point computed them and threw them away, so every
// caller of the row-level surface silently had the pre-legend behaviour.
// Hand the map back to a value-level reader through Options.Legend.OptionIds
// under this key's spelling, or drop it and accept name resolution knowingly.
func MarshalPropertyValue(key string, v *types.Value, opts Options) (any, map[string]string) {
	e := &exporter{opts: opts}
	out := e.propertyValue(key, v)
	return out, e.optionIdsFor(key)
}

// Marshal serializes a snapshot into canonical AnyBlock JSON (§13).
func Marshal(sbType model.SmartBlockType, snapshot *model.SmartBlockSnapshotBase, opts Options) ([]byte, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("nil snapshot")
	}
	e := &exporter{opts: opts, snapshot: snapshot, sbType: sbType, blocks: map[string]*model.Block{}, visited: map[string]bool{}}
	e.indexBlocks()
	if opts.compactBlockLabels() {
		e.buildLabelPlan()
	}
	doc, err := e.buildDoc(sbType)
	if err != nil {
		return nil, fmt.Errorf("build document: %w", err)
	}
	return marshalCanonical(doc)
}

type exporter struct {
	opts     Options
	snapshot *model.SmartBlockSnapshotBase
	sbType   model.SmartBlockType
	blocks   map[string]*model.Block
	rootId   string
	visited  map[string]bool // emitted block ids: breaks ChildrenIds cycles, dedupes shared children

	// emitted, when non-nil, records the STORED doc-local id of everything
	// this run actually writes — blocks, table rows/columns, dataview views.
	// It is the id census's population (buildLabelPlan): only the probe run
	// sets it, so a normal export pays nothing for it.
	emitted map[string]bool

	localIds map[string]string // block/row/column/view id -> short label (§9a)

	// optionRefs is the second `refs` population: the option id behind every
	// name export wrote for a select value (optionrefs.go). Recorded against
	// the STORED property key and rendered into `<name>#<slug>` keys at
	// envelope-assembly time, when the term ledger has settled.
	optionRefs map[optionRefPair]string

	// idLabels maps a stored block/row/column id to the id written for it, and
	// idsUsed is every id this document has written. One set for every id
	// surface, because they share one uniqueness domain (§4): a sanitized
	// column id, a compact label and a verbatim block id all land in the same
	// document, and any two of them colliding is a document Validate rejects.
	idLabels map[string]string
	idsUsed  map[string]struct{}

	// propertyKeys is the §3 legend: the slug→stored-key entries this document
	// must carry to be invertible by a reader that cannot ask the space.
	propertyKeys map[string]string

	// termOwner / termByKey / namedKeys are the §3 term ledger — the property
	// keys' analogue of idLabels/idsUsed: one claim domain for every key slot,
	// because the legend that inverts the terms is one document-wide map.
	termOwner map[string]string // term -> the stored key it denotes
	termByKey map[string]string // stored key -> the term written for it
	namedKeys map[string]bool   // census: every stored key any slot may name

	// typeKeys is the §3 legend for the TYPE namespace, and typeTermOwner /
	// typeTermByKey / typeNamedKeys its term ledger. One ledger and one
	// legend PER NAMESPACE, deliberately: a property slug and a type slug may
	// coincide without conflict (§3 — `object_type` the type key coexists
	// with `objectType` the layout value, and a space can slug a relation and
	// a type onto the same term), so a shared claim domain would back a key
	// off a slug the other namespace owns — a spurious conflict, and one
	// legend map could not carry both meanings of the shared term at all.
	typeKeys      map[string]string
	typeTermOwner map[string]string
	typeTermByKey map[string]string
	typeNamedKeys map[string]bool
}

// propertySlug renders a stored property key for output and records what the
// document owes a reader who cannot ask the space (§3). It is the term
// ledger's claim step, and the ONLY way a key slot may spell a key: a stored
// key always keeps its own term (§3 verbatim-first — the census reserved it),
// a slug goes to its first claimant, and a later key whose slug is already
// claimed — by an earlier holder or by a stored key the document names —
// falls back to its stored key, which is always its own address. The answer
// is remembered, so one key spells the same way in every slot; without the
// ledger, a block slot's blind recordPropertyKey could rebind a term that
// /properties already owns, silently moving that property's value onto a
// different relation.
func (e *exporter) propertySlug(key string) string {
	if key == "" {
		return key
	}
	if e.termOwner == nil {
		e.seedTermLedger()
	}
	if term, done := e.termByKey[key]; done {
		return term
	}
	term := e.writableSlug(key)
	if term != key {
		if _, claimed := e.termOwner[term]; claimed || e.namedKeys[term] {
			term = key
		}
	}
	e.termOwner[term] = key
	e.termByKey[key] = term
	e.recordPropertyKey(term, key)
	return term
}

// seedTermLedger runs the property-key census: every stored key any slot of
// this document may name. Verbatim-first (§3) makes each of those keys its
// own address, so no OTHER key's slug may take one as a spelling — the same
// avoid-set discipline seedIdLabels applies to ids. The walk mirrors the emit
// sites (buildProperties, buildTypeProperties, blockToJSON, dataviewToJSON);
// a key from a block the emit later drops only over-reserves, which degrades
// somebody's slug to their stored key — always correct, merely less compact,
// and it costs one thing worth naming: an over-reservation that the next
// generation does not repeat makes export stop being a fixpoint for that
// object (modelledTypeKeys says the same in the type namespace, where the
// gap was computable and is now closed). Which blocks survive is decided
// during buildBlocks, so this walk cannot know; the type-namespace shapes
// were reachable through ordinary data, this one needs a block export drops.
func (e *exporter) seedTermLedger() {
	e.termOwner = map[string]string{}
	e.termByKey = map[string]string{}
	e.namedKeys = map[string]bool{}
	name := func(key string) {
		if key != "" {
			e.namedKeys[key] = true
		}
	}
	if e.snapshot == nil {
		return
	}
	if e.snapshot.Details != nil {
		stripped := strippedDetailKeys()
		lifted := e.typePropDetailKeys()
		for k := range e.snapshot.Details.Fields {
			if !stripped[k] && !lifted[k] && isWritablePropertyKey(k) {
				name(k)
			}
		}
	}
	if e.typePropsActive() {
		for _, l := range recommendedListKeys {
			for _, id := range valueStringList(e.detail(l.detailKey)) {
				if def, ok := e.resolveTypeProperty(id); ok {
					name(string(def.Key))
				}
			}
		}
	}
	for _, b := range e.snapshot.Blocks {
		if b == nil {
			continue
		}
		switch c := b.Content.(type) {
		case *model.BlockContentOfRelation:
			name(orEmpty(c.Relation).Key)
		case *model.BlockContentOfLink:
			for _, k := range orEmpty(c.Link).Relations {
				name(k)
			}
		case *model.BlockContentOfDataview:
			dv := orEmpty(c.Dataview)
			for _, rl := range dv.RelationLinks {
				if rl != nil {
					name(rl.Key)
				}
			}
			for _, v := range dv.Views {
				if v == nil {
					continue
				}
				name(v.GroupRelationKey)
				name(v.CoverRelationKey)
				name(v.EndRelationKey)
				for _, r := range v.Relations {
					if r != nil {
						name(r.Key)
					}
				}
				for _, s := range v.Sorts {
					if s != nil {
						name(s.RelationKey)
					}
				}
				for _, f := range flattenFilters(v.Filters) {
					name(f.RelationKey)
				}
			}
		}
	}
}

// propertySlugs is the list form, and it lives here rather than on Options for
// the same reason the singular one does: a key list (a link block's shown
// properties) is a key slot like any other, and a slug written without the
// legend entry that inverts it is a slug that reads back as a different
// relation. Options carries the vocabulary; only the exporter can record what
// the document owes for using it.
func (e *exporter) propertySlugs(keys []string) []string {
	if len(keys) == 0 {
		return keys
	}
	out := make([]string, len(keys))
	for i, key := range keys {
		out[i] = e.propertySlug(key)
	}
	return out
}

// writableSlug is the vocabulary's spelling for a stored key when that
// spelling can actually be written, and the stored key itself otherwise. The
// slug is the string that becomes a JSON property name (buildProperties) or a
// legend entry's name, and slugs come from apiObjectKey — user-supplied or
// strcase-derived with no length bound — so nothing upstream guarantees the
// shape §3 requires of a spelling. Checking the stored key and then emitting
// the slug unchecked is how Marshal produced a document its own Validate
// rejects (maxLength 192 vs 128, on /properties and /property_keys at once).
// The stored-key arm covers the mirror case: a slug for a key that cannot be a
// legend VALUE has no invertible spelling but its own, so the verbatim key —
// always its own address (§3 verbatim-first) — is the one honest rendering.
//
// The same fate for a slug validation refuses on other grounds than shape:
// "id" and "type" are refused as SPELLINGS before any resolution (§2 — the
// legend cannot re-purpose them, so it cannot rescue them either), and a
// DENIED key never takes a slug at all, because the slug's legend entry would
// carry a value the §3 deny rule refuses. Both used to make Marshal emit what
// its own Validate rejects — or, for the denied key, emit a legend a
// pre-admission reader would happily resolve.
func (e *exporter) writableSlug(key string) string {
	slug := e.opts.propertySlug(key)
	if slug == key {
		return slug
	}
	if !isWritablePropertyKey(slug) || !isWritablePropertyKey(key) {
		e.warn("/property_keys",
			"the vocabulary spells %q as %q, which cannot be a property spelling in this format; the stored key is written instead",
			key, slug)
		return key
	}
	if slug == detailKeyId || slug == detailKeyType {
		e.warn("/property_keys",
			"the vocabulary spells %q as %q, a spelling this format refuses before any resolution (§2); the stored key is written instead",
			key, slug)
		return key
	}
	if _, denied := deniedPropertyKey(key); denied {
		e.warn("/property_keys",
			"%q cannot be a legend value (§3 deny rule), so its slug %q is not written; the stored key is its own address",
			key, slug)
		return key
	}
	return slug
}

// recordPropertyKey writes the legend entry a term owes, or nothing when
// every reader's own chain already answers it correctly. One condition,
// two halves, and they ask DIFFERENT questions:
//
//  1. the **bundled table BINDS this spelling to this very key** — it ships
//     with every reader, so `due_date` → `dueDate` needs no entry; and
//  2. the **vocabulary in force INVERTS it** — a reader may bind a spelling
//     the bundled table binds correctly, and the writer's own space is the
//     reader most likely to read the document back.
//
// The asymmetry is the point, and it is what makes the rule EXHAUSTIVE. A
// term that is a stored key written verbatim trivially "inverts" through any
// table, because a table that does not know a term answers the term itself
// (chain step 4) — so asking half 1 as an inversion let every custom key
// pass with no entry at all, and the document said nothing about the one
// population no reader can resolve without it. That silence is the corpse
// hole: the key is live and unambiguous the day it is written, and the
// moment a relation is UI-deleted its stored key stops being live while the
// freed spelling becomes some other relation's api key. Every document
// already written then re-points, offline, with nothing in it to say
// otherwise. Asking half 1 as a BINDING closes it: a spelling the bundled
// table does not bind to this key owes an entry, verbatim or not.
//
// Half 2 stays an inversion, and stays. Dropping it — "one table, not two" —
// loses the SHADOWING-WRITER entries, where the bundled table binds the
// spelling correctly and the vocabulary in force binds it elsewhere:
// measured, it drops `{"task": "task"}` and a template comes back pointing
// at an unrelated custom type, and drops `{"due_date": "dueDate"}` and
// dueDate's value lands on the custom relation that wanted the spelling.
// Both are silent losses of user data, both are pinned by tests.
//
// So identity entries stop being the exception and become the common line:
// every custom key names itself in the legend. That is the byte cost of the
// rule — ~2% on the golden documents — and it buys the document the ability
// to say what its own spellings mean without asking anyone.
//
// An entry the LEGEND cannot hold is not written — see legendEntryRefusal.
func (e *exporter) recordPropertyKey(term, key string) {
	if term == "" {
		return
	}
	if bundledBinds(term, key, (BundledKeyVocabulary{}).PropertyKey) &&
		termInverts(term, key, e.opts.keys().PropertyKey) {
		return
	}
	if reason, refused := legendEntryRefusal(term, key, true); refused {
		e.warn("/property_keys", "%s", reason)
		return
	}
	if e.propertyKeys == nil {
		e.propertyKeys = map[string]string{}
	}
	e.propertyKeys[term] = key
}

// legendEntryRefusal reports whether a `property_keys` / `type_keys` entry is
// one the format can actually carry, and why not. It is the recording site's
// share of I1 ("Marshal never emits what Validate rejects", §11): every other
// key slot admits before it writes, and the two legends did not — the ONLY
// admission on the way in was writableSlug/writableTypeSlug, which returns
// early when the vocabulary has no slug for a key, so the stored key reached
// the ledger unvetted. Marshal then wrote `{"a\nb": "a\nb"}` and its own
// Validate rejected the whole document; the object was unexportable and
// nothing said so. Reproduced first-hand at a dataview FILTER slot (which,
// unlike /properties, does not pre-filter unwritable keys) and at the
// envelope `type`, on `a\nb` and on a 140-character key, in both namespaces.
//
// The rule is the two Validate already states — a legend spelling and a
// legend stored key are both writable keys (propertyNameIssues), and a legend
// VALUE in the property namespace is a stored key the deny rule judges
// (§3 §4a) — asked here so the answers cannot drift.
//
// **Refusing the entry loses nothing the document was carrying.** The term is
// written verbatim either way (the ledger backed it off to the stored key
// long before this point), so the object still round-trips through any reader
// whose chain reaches step 4. What is lost is PORTABILITY for that one key:
// a reader whose vocabulary binds the spelling elsewhere has nothing to
// override it with. That is a strictly smaller loss than the alternative,
// which was the whole document, and the warning says which key it applies to.
func legendEntryRefusal(term, key string, deny bool) (string, bool) {
	if !isWritablePropertyKey(term) {
		return fmt.Sprintf("%s, so no legend entry is written for it; the term is "+
			"spelled verbatim and a reader that binds it elsewhere cannot be corrected",
			unwritableKeyReason("legend spelling", term)), true
	}
	if !isWritablePropertyKey(key) {
		return fmt.Sprintf("%s, so no legend entry is written for it; the term is "+
			"spelled verbatim and a reader that binds it elsewhere cannot be corrected",
			unwritableKeyReason("legend stored key", key)), true
	}
	if !deny {
		return "", false
	}
	if reason, denied := deniedPropertyKey(key); denied {
		return fmt.Sprintf("legend value: %s — so no legend entry is written for %q; "+
			"the term is spelled verbatim", reason, term), true
	}
	return "", false
}

// termInverts reports whether `term`, written for the stored key `key` with
// NO legend entry, reads back as `key` through one reader's table. It asks
// the table the same way the importer does — Options.propertyKey/typeKey
// take the answer and drop the ok flag, and a table that does not know a
// term answers the term itself (chain step 4, verbatim), which is what makes
// the two forms one question.
//
// Export asks it of TWO tables, and the second one is the fix for a defect
// that lost user data. The bundled table is the reader that always exists,
// so a spelling it binds elsewhere has always owed an entry. But the
// vocabulary this export runs under is a reader too — the writer's own
// space, the one most likely to read the document back — and it answers
// FIRST, before the bundled table (importer.propertyKey / typeKey run
// Options' vocabulary, which is the whole chain a node-backed reader has).
// Asking only the bundled table wrote a term with no legend that the
// writer's own vocabulary then bound to a different stored key:
//
//   - the type namespace lost data in silence. A type UI-deleted from the
//     space vacates the slug namespace (storeresolver's corpse policy), so
//     `initiative` stops being a live stored key while objects still carry
//     `ot-initiative`; the same listing binds the slug `initiative` to a
//     live type keyed `69bbfc…`. Export wrote `"type": "initiative"` with no
//     entry and import bound it to `69bbfc…` — the object came back typed as
//     a different type, no error anywhere.
//   - the property namespace broke I1 out loud. A vocabulary spelling
//     `alpha` as `beta`, over an object holding both `alpha` and `beta`,
//     wrote both keys verbatim (the ledger backs `alpha` off its contested
//     slug, correctly) — and then the reader's own vocabulary bound `beta`
//     to `alpha`, so two spellings addressed one property and Unmarshal
//     refused a document Marshal had just emitted.
//
// The entry fixes both for EVERY reader, not just for the one whose table
// prompted it: the legend is chain step 1, ahead of any vocabulary. What it
// cannot fix is a reader whose table shadows the bundled one in a way the
// writer never saw — that is precondition 2 on KeyVocabulary, and it stays a
// precondition for exactly this reason.
func termInverts(term, key string, table func(string) (string, bool)) bool {
	back, _ := table(term)
	return back == key
}

// bundledBinds is the stricter question recordPropertyKey/recordTypeKey ask
// of the BUNDLED table: does the table actually BIND this spelling to this
// key — `ok` and all — rather than merely fail to contradict it?
//
// termInverts cannot answer it. It drops the ok flag on purpose, because
// that is what the importer does, and for the VOCABULARY half that is the
// right question: "would this reader land on the right key?". For the
// bundled half it is the wrong one, because "the table has never heard of
// this term" and "the table binds this term to this key" are the same answer
// there — and they mean opposite things to a reader. The first is exactly
// the population that owes an entry: a key the bundled table cannot speak
// for, whose spelling is up for grabs the moment the key stops being live.
func bundledBinds(term, key string, table func(string) (string, bool)) bool {
	back, ok := table(term)
	return ok && back == key
}

// buildPropertyKeys renders the legend in key order, or nil when the document
// needs none — which is every document a package-only reader wrote.
func (e *exporter) buildPropertyKeys() *omap {
	if len(e.propertyKeys) == 0 {
		return nil
	}
	slugs := make([]string, 0, len(e.propertyKeys))
	for slug := range e.propertyKeys {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	m := &omap{}
	for _, slug := range slugs {
		m.set(slug, e.propertyKeys[slug])
	}
	return m
}

// typeSlug renders a stored type key for output and records what the
// document owes a reader who cannot ask the space (§3) — the type
// namespace's claim step, propertySlug on a ledger of its own. The same
// discipline for the same reason: a stored type key named anywhere in the
// document always keeps its own term (verbatim-first), a slug goes to its
// first claimant, and a contested slug falls back to the stored key, which
// is always its own address.
func (e *exporter) typeSlug(key string) string {
	if key == "" {
		return key
	}
	if e.typeTermOwner == nil {
		e.seedTypeTermLedger()
	}
	if term, done := e.typeTermByKey[key]; done {
		return term
	}
	term := e.writableTypeSlug(key)
	if term != key {
		if _, claimed := e.typeTermOwner[term]; claimed || e.typeNamedKeys[term] {
			term = key
		}
	}
	e.typeTermOwner[term] = key
	e.typeTermByKey[key] = term
	e.recordTypeKey(term, key)
	return term
}

// typeSlugs is the list form (a type property's object_types, §2a).
func (e *exporter) typeSlugs(keys []string) []string {
	if len(keys) == 0 {
		return keys
	}
	out := make([]string, len(keys))
	for i, key := range keys {
		out[i] = e.typeSlug(key)
	}
	return out
}

// seedTypeTermLedger runs the type-key census: every stored type key any
// slot of this document may name — the snapshot's object types (envelope
// `type`/`template_for`) and the target types of the resolved type-property
// definitions (§2a object_types). Verbatim-first (§3) makes each its own
// address, so no other key's slug may take one as a spelling.
func (e *exporter) seedTypeTermLedger() {
	e.typeTermOwner = map[string]string{}
	e.typeTermByKey = map[string]string{}
	e.typeNamedKeys = map[string]bool{}
	if e.snapshot == nil {
		return
	}
	for _, key := range e.modelledTypeKeys(false) {
		e.typeNamedKeys[key] = true
	}
	if e.typePropsActive() {
		for _, l := range recommendedListKeys {
			for _, id := range valueStringList(e.detail(l.detailKey)) {
				def, ok := e.resolveTypeProperty(id)
				if !ok || !writableTypePropertyKey(def) {
					continue
				}
				for _, key := range def.ObjectTypes {
					if key != "" {
						e.typeNamedKeys[key] = true
					}
				}
			}
		}
	}
}

// writableTypeSlug is writableSlug for the type namespace: the vocabulary's
// spelling when it can actually be written and honored, the stored key
// itself otherwise. The shape rule is the same — a slug becomes a legend
// spelling and a stored key a legend value, both bounded by the schema. The
// reserved spelling differs: the type namespace has none. It used to refuse
// to move `template` in either direction, because the envelope's template
// semantics hung off the spelled term — export keyed template_for emission
// off it, validation gated /template_for on it, and import derived the
// smartblock kind from it, so a vocabulary moving the spelling dropped a
// template's target type and one landing another key on it handed that
// machinery to the wrong type. `kind` carries all three now (§2), the term is
// an ordinary type spelling, and the reservation deleted with the ambiguity
// it was protecting: a vocabulary may spell the template type `tmpl`, and the
// legend says so and inverts it.
func (e *exporter) writableTypeSlug(key string) string {
	slug := e.opts.typeSlug(key)
	if slug == key {
		return slug
	}
	if !isWritablePropertyKey(slug) || !isWritablePropertyKey(key) {
		e.warn("/type_keys",
			"the vocabulary spells type %q as %q, which cannot be a type spelling in this format; the stored key is written instead",
			key, slug)
		return key
	}
	return slug
}

// recordTypeKey writes the type legend entry a term owes, or nothing when
// every reader's own chain already inverts it — recordPropertyKey's rule
// through the type half of the two tables, identity entries included: a
// stored type key written verbatim whose spelling the bundled table binds to
// a DIFFERENT key (`object_type` the stored key beside bundled `objectType`)
// gets `{"object_type": "object_type"}`, the document's only way to tell a
// storeless reader the term is a stored key — and the same entry, for the
// same reason, when the vocabulary in force is the one that binds it
// elsewhere (`initiative` the stored key of a UI-deleted type, beside the
// live type whose api key is `initiative`). See termInverts.
//
// An entry the legend cannot hold is not written here either
// (legendEntryRefusal) — minus the deny rule, which is the property
// namespace's alone: `strippedDetailKeys` and the importer's resolution
// vectors are relation keys, and Validate states no deny rule over a
// `type_keys` value.
func (e *exporter) recordTypeKey(term, key string) {
	if term == "" {
		return
	}
	if bundledBinds(term, key, (BundledKeyVocabulary{}).TypeKey) &&
		termInverts(term, key, e.opts.keys().TypeKey) {
		return
	}
	if reason, refused := legendEntryRefusal(term, key, false); refused {
		e.warn("/type_keys", "%s", reason)
		return
	}
	if e.typeKeys == nil {
		e.typeKeys = map[string]string{}
	}
	e.typeKeys[term] = key
}

// buildTypeKeys renders the type legend in term order, or nil when the
// document needs none — which is every document that names only bundled and
// verbatim, unshadowed type keys.
// legendTypeTerm answers what this document's own type_keys legend binds a
// term to, falling back to the term itself. It is the emission-side twin of
// Validate's legacy-template gate: both read the document alone, resolve
// nothing beyond it, and must agree about which spellings mean the template
// type (§2, §10).
func (e *exporter) legendTypeTerm(term string) string {
	if key, ok := e.typeKeys[term]; ok && key != "" {
		return key
	}
	return term
}

func (e *exporter) buildTypeKeys() *omap {
	if len(e.typeKeys) == 0 {
		return nil
	}
	terms := make([]string, 0, len(e.typeKeys))
	for term := range e.typeKeys {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	m := &omap{}
	for _, term := range terms {
		m.set(term, e.typeKeys[term])
	}
	return m
}

// seedIdLabels reserves the id each block will be written with, before any
// sanitizing starts. Without it the first block to need sanitizing could take
// the name of a block that was going to be written verbatim — renaming a
// perfectly good authored id, or duplicating it.
//
// The order below is what makes export's reservations the same id domain
// validation checks (§4). Rows and columns come first, because the derived
// cell ids are built from their labels; then the grid those imply — every
// rowId-colId pair, written or not, since the table owns the id either way and
// the editor materializes the cell at exactly that id the first time it is
// filled (§6.1); then everything else, which yields to the grid, because "a
// non-table block id that collides with a derived cell id is a validation
// error" (§4) and the derived id is the one that cannot move.
//
// Only blocks the emit can REACH are reserved. A snapshot's block list is not
// its block tree: orphaned subtrees outlive the block that held them (state
// apply unlinks without deleting), and a table among them owns a whole grid of
// derived ids — reserving that grid renamed a perfectly good authored id on a
// block the document does contain, on the authority of one nobody can see. The
// walk over-approximates on purpose: it descends every ChildrenIds edge from
// the export's entry point, including the ones blockToJSON declines to follow,
// because under-reserving is the dangerous direction — a grid that IS emitted
// and not reserved is a document Marshal's own Validate rejects (I1).
func (e *exporter) seedIdLabels() {
	e.idLabels = map[string]string{}
	e.idsUsed = map[string]struct{}{}
	if e.snapshot == nil {
		return
	}
	reachable := e.reachableBlocks()

	// snapshot order, not map order: the reservations are order-dependent now,
	// so ranging over the id-keyed map would make the output nondeterministic.
	// wanted collects the label every block would take verbatim; it is an
	// avoid-set rather than a reservation, because sanitizing a row id must
	// not take the name of a block written as-is, and that block cannot be
	// reserved before the grid it may collide with.
	var order []*model.Block
	seen := map[string]bool{}
	wanted := map[string]struct{}{}
	for _, b := range e.snapshot.Blocks {
		if b == nil || b.Id == "" || seen[b.Id] || !reachable[b.Id] {
			continue
		}
		seen[b.Id] = true
		order = append(order, e.blocks[b.Id]) // the indexed block wins, as everywhere else
		wanted[e.localId(b.Id)] = struct{}{}
	}

	for _, b := range order {
		if !e.isTableInner(b) {
			continue
		}
		if want := e.localId(b.Id); isValidTableInnerId(want) {
			e.idLabels[b.Id] = want
			e.idsUsed[want] = struct{}{}
		}
	}
	for _, b := range order {
		if !e.isTableInner(b) {
			continue
		}
		if _, done := e.idLabels[b.Id]; done {
			continue
		}
		e.idLabels[b.Id] = e.reserveLabel(sanitizeTableInnerId(e.localId(b.Id)), wanted)
	}
	for _, id := range e.derivedCellIds(reachable) {
		e.idsUsed[id] = struct{}{}
	}
	for _, b := range order {
		if e.isTableInner(b) {
			continue
		}
		want := e.localId(b.Id)
		if !isBlockIdLabel(want) { // the blockId charset: [A-Za-z0-9_-]{1,64}
			continue
		}
		if _, taken := e.idsUsed[want]; taken {
			continue // a derived cell id holds the name; idLabel disambiguates
		}
		e.idLabels[b.Id] = want
		e.idsUsed[want] = struct{}{}
	}
}

// reserveLabel takes base, or the first _n form of it that no id has taken and
// no id is going to take verbatim.
func (e *exporter) reserveLabel(base string, wanted map[string]struct{}) string {
	label := base
	for n := 2; ; n++ {
		_, used := e.idsUsed[label]
		_, want := wanted[label]
		if !used && !want {
			e.idsUsed[label] = struct{}{}
			return label
		}
		suffix := "_" + strconv.Itoa(n)
		trimmed := base
		if len(trimmed)+len(suffix) > maxIdLen {
			trimmed = trimmed[:maxIdLen-len(suffix)]
		}
		label = trimmed + suffix
	}
}

// derivedCellIds lists the cell id every table the export can REACH implies:
// rowLabel + "-" + colLabel for the whole grid, materialized or not (§6.1).
// It runs after every row and column has its label, and mirrors the structure
// tableToJSON reads — wrappers by layout style, children by content type — so
// that the ids reserved are the ones the exported tables actually derive. A
// table no emit reaches derives nothing, so it reserves nothing: its grid is
// not in the document, and the ids of the blocks that are may not turn on it.
func (e *exporter) derivedCellIds(reachable map[string]bool) []string {
	var out []string
	for _, b := range e.snapshot.Blocks {
		if b == nil || b.Id == "" || e.blocks[b.Id] != b || !reachable[b.Id] {
			continue
		}
		if _, isTable := b.Content.(*model.BlockContentOfTable); !isTable {
			continue
		}
		var cols, rows []string
		for _, id := range b.ChildrenIds {
			wrapper := e.blocks[id]
			l, ok := wrapper.GetContent().(*model.BlockContentOfLayout)
			if !ok {
				continue
			}
			for _, innerId := range wrapper.ChildrenIds {
				inner := e.blocks[innerId]
				if inner == nil {
					continue
				}
				label := e.idLabels[innerId]
				if label == "" {
					continue
				}
				switch l.Layout.GetStyle() {
				case model.BlockContentLayout_TableColumns:
					if _, ok := inner.Content.(*model.BlockContentOfTableColumn); ok {
						cols = append(cols, label)
					}
				case model.BlockContentLayout_TableRows:
					if _, ok := inner.Content.(*model.BlockContentOfTableRow); ok {
						rows = append(rows, label)
					}
				}
			}
		}
		for _, row := range rows {
			for _, col := range cols {
				out = append(out, row+"-"+col)
			}
		}
	}
	return out
}

// reachableBlocks is the ChildrenIds closure of the export's entry point —
// every block the emit can arrive at, and therefore every block whose id the
// document may contain. Everything else is an orphan: present in the snapshot,
// absent from the output, and with no claim on the id domain (§4).
//
// It is deliberately coarser than the emit: blockToJSON stops descending into
// a bookmark, a link or a divider, and drops structural and content-less
// blocks entirely, but this walk follows those edges anyway. Reserving for a
// block the emit turns out to drop costs a disambiguation suffix; failing to
// reserve for one it keeps costs a document that fails its own Validate.
//
// The walk is iterative: a snapshot's block graph is untrusted, and a
// pathological chain is not the place to find out how deep the stack goes.
func (e *exporter) reachableBlocks() map[string]bool {
	out := map[string]bool{}
	if e.rootId == "" {
		return out
	}
	stack := []string{e.rootId}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if id == "" || out[id] {
			continue
		}
		out[id] = true
		if b := e.blocks[id]; b != nil {
			stack = append(stack, b.ChildrenIds...)
		}
	}
	return out
}

func (e *exporter) isTableInner(b *model.Block) bool {
	switch b.GetContent().(type) {
	case *model.BlockContentOfTableRow, *model.BlockContentOfTableColumn:
		return true
	}
	return false
}

// idLabel is the one place a stored id becomes the id written in the document.
// It sanitizes with the charset of the position, then disambiguates against
// every id the document has already written, and remembers its answer so the
// same stored id always renders the same way.
func (e *exporter) idLabel(stored string, sanitize func(string) string) string {
	if stored == "" {
		return ""
	}
	if e.idLabels == nil {
		e.seedIdLabels()
	}
	if got, ok := e.idLabels[stored]; ok {
		return got
	}
	base := sanitize(e.localId(stored))
	label := base
	for n := 2; ; n++ {
		if _, taken := e.idsUsed[label]; !taken {
			e.idsUsed[label] = struct{}{}
			e.idLabels[stored] = label
			return label
		}
		suffix := "_" + strconv.Itoa(n)
		trimmed := base
		if len(trimmed)+len(suffix) > maxIdLen {
			trimmed = trimmed[:maxIdLen-len(suffix)]
		}
		label = trimmed + suffix
	}
}

// blockLabel renders a block's stored id for output (§9). Stored ids are not
// guaranteed to match the schema's block charset: legacy accounts hold ids
// with dots and slashes, and Options.GenerateId belongs to the caller — the
// convert wiring derives ids from file paths. Writing one verbatim made
// Marshal emit a document its own Validate rejects, i.e. an archive that fails
// at import, discovered long after the export.
func (e *exporter) blockLabel(stored string) string {
	return e.idLabel(stored, sanitizeBlockId)
}

func (e *exporter) detail(key string) *types.Value {
	if e.snapshot.Details == nil {
		return nil
	}
	return e.snapshot.Details.Fields[key]
}

func (e *exporter) objectId() string {
	return e.detail(detailKeyId).GetStringValue()
}

func (e *exporter) indexBlocks() {
	children := map[string]bool{}
	for _, b := range e.snapshot.Blocks {
		if b == nil || b.Id == "" {
			continue
		}
		e.blocks[b.Id] = b
		for _, c := range b.ChildrenIds {
			children[c] = true
		}
	}
	// the root block's id equals the object id (§2); fall back to the first
	// block nobody references
	if _, ok := e.blocks[e.objectId()]; ok {
		e.rootId = e.objectId()
		return
	}
	for _, b := range e.snapshot.Blocks {
		if b != nil && b.Id != "" && !children[b.Id] {
			e.rootId = b.Id
			return
		}
	}
}

// typeKeyIdPrefix is the "ot-" prefix ObjectTypes entries carry.
var typeKeyIdPrefix = domain.TypeKey("").URL()

// envelopeTypeTerms are the spellings written for the snapshot's object
// types — the `type`/`template_for` slots — each claimed through the type
// term ledger so the legend it owes is recorded (§3).
//
// Two disciplines run here, and both are buildProperties' own:
//
//   - **A keyless entry is dropped WITH a warning, and the survivors close
//     ranks.** A stored `ot-` (or a bare "") carries no type key: typeSlug
//     answers "" for it and setNonEmpty then omits the slot, so a positional
//     write lost that entry AND everything behind it. A template stored as
//     ["ot-", "ot-task"] emitted no `type` at all, which made `template_for`
//     inexpressible too — so the perfectly good `ot-task` vanished beside its
//     bad neighbour, silently, and the document read back as no types at all.
//     Filtering first is what lets the good sibling survive; the warning is
//     what buildProperties already owes an unwritable *property* key, and
//     what the import seam refuses outright and path-addressed.
//   - **Only the slots actually WRITTEN claim a term.** typeSlug is the term
//     ledger's claim step, so slugging an entry no slot emits still records
//     the legend entry that spelling owes: a document then carried a
//     `type_keys` line naming a type it never mentions, publishing a space's
//     slug→key mapping for nothing. buildProperties cannot do this because it
//     filters before it slugs; the type side now does the same.
//
// The list still truncates to the positions §2 models — one type, plus the
// target type on a template. That is the format's shape, not a defect, and
// the census (seedTypeTermLedger) still reserves every stored type key the
// snapshot names, so a dropped entry's key can never be taken as another
// key's spelling.
func (e *exporter) envelopeTypeTerms() []string {
	keys := e.modelledTypeKeys(true)
	terms := make([]string, 0, len(keys))
	for _, key := range keys {
		terms = append(terms, e.typeSlug(key))
	}
	return terms
}

// modelledTypeKeys reduces the snapshot's object types to the stored keys the
// envelope will actually spell: keyless entries dropped, survivors closing
// ranks, then the positions §2 models — one type, plus the target type on a
// template. `warn` reports each keyless drop, and only the emitting call
// passes it, because the CENSUS runs this reduction too and must not report
// the same drop twice.
//
// The census has to see exactly this list rather than every object type,
// which is where it started. Reserving a key no slot spells makes export stop
// being a fixpoint: a snapshot whose truncated-away second type is the first
// one's slug backed that slug off, while the same object exported after one
// round trip — the second type gone, the census one key smaller — spelled it.
// Two documents, the same object, differing in the term and in a legend line;
// §9's "re-exports diff cleanly" is the promise that breaks. Nothing was
// protected by the wider reservation either: a key the document never names
// cannot be taken as another key's spelling by a reader that never sees it.
//
// The second slot exists exactly when the SMARTBLOCK TYPE is Template, which
// is the whole of §2's template rule since v0.22. It used to be "when the
// first surviving key is the template key", and that was the bug: a template
// whose object types are ["ot-task", "ot-extra"] — a real shape, since
// nothing in the model requires a template to carry the template key first —
// kept one slot and dropped its target type with a warning. Keyed off the
// smartblock type, the same snapshot writes `{"kind": "template", "type":
// "task", "template_for": "extra"}` and round-trips whole.
func (e *exporter) modelledTypeKeys(warn bool) []string {
	keys := make([]string, 0, len(e.snapshot.ObjectTypes))
	stood := make([]int, 0, len(e.snapshot.ObjectTypes)) // where each survivor stood
	for i, t := range e.snapshot.ObjectTypes {
		key := strings.TrimPrefix(t, typeKeyIdPrefix)
		if key == "" {
			if warn {
				e.warn("/type",
					"object type %d (%q) carries no type key and is dropped; the remaining types move up", i, t)
			}
			continue
		}
		keys = append(keys, key)
		stood = append(stood, i)
	}
	if len(keys) == 0 {
		return nil
	}
	kept := 1
	if e.sbType == model.SmartBlockType_Template && len(keys) > 1 {
		kept = 2
	}
	// §3 promises every drop is reported, and the positional one was the
	// silent half: a keyless entry warned, a perfectly good SECOND type just
	// vanished. It is not a defect — the envelope models these positions and
	// no others (§2) — but it is a loss the caller can neither see in the
	// document nor infer from it, and the caller is the one holding the
	// snapshot that still has the type.
	if warn {
		for j := kept; j < len(keys); j++ {
			e.warn("/type",
				"object type %d (%q) is dropped: the envelope carries one type, plus the target type on a template (§2), "+
					"and every position is already taken; it is not written anywhere in this document",
				stood[j], e.snapshot.ObjectTypes[stood[j]])
		}
	}
	return keys[:kept]
}

func (e *exporter) buildDoc(sbType model.SmartBlockType) (*omap, error) {
	doc := &omap{}
	doc.set("$schema", SchemaURL)
	doc.set("version", FormatVersion)

	typeTerms := e.envelopeTypeTerms()
	typeTerm := ""
	if len(typeTerms) > 0 {
		typeTerm = typeTerms[0]
	}

	// kind is omitted whenever derivable (§2), and since v0.22 only Page is
	// derivable: `kind` is the sole authority on template-ness, so a Template
	// always spells it. The term test that survives is an EMISSION rule and
	// resolves nothing — a Page whose type term is literally `template` keeps
	// its explicit kind, because `{"type": "template"}` with no kind is the
	// pre-v0.22 spelling of a template and this format's own Validate now
	// refuses it (§10). Emitting it would be Marshal writing what Validate
	// rejects, which is I1.
	// The term test reads what the document's own type_keys legend binds the
	// term to, not the raw spelling — Validate's legacy-template refusal reads
	// the same legend, and the two have to agree or Marshal writes what
	// Validate rejects. A page whose type term RESOLVES to the template key
	// (`{"type_keys": {"tmpl": "template"}, "type": "tmpl"}`) is the same
	// shape as one spelling it literally, and needs its kind just as much.
	// EITHER spelling forces the kind: the raw term (`{"type": "template"}` is
	// the pre-v0.22 spelling of a template, which Validate refuses) or the key
	// the document's own type_keys legend binds it to (`{"type_keys":
	// {"tmpl": "template"}, "type": "tmpl"}` is the same document said
	// differently, and Validate's legacy gate reads that legend too). Testing
	// only the raw term let a page whose term RESOLVES to the template key
	// export with no kind, which Validate then refused — Marshal writing what
	// Validate rejects (I1). Testing only the resolved key lets a page whose
	// legend rebinds `template` ELSEWHERE drop its kind, and the raw spelling
	// trips the same refusal.
	derivable := sbType == model.SmartBlockType_Page &&
		typeTerm != typeKeyTemplate && e.legendTypeTerm(typeTerm) != typeKeyTemplate
	if !derivable {
		name := kindNames.name(sbType)
		if name == "" {
			return nil, fmt.Errorf("smartblock type %v has no kind mapping", sbType)
		}
		doc.set("kind", name)
	}

	doc.setNonEmpty("id", e.objectId())
	doc.setNonEmpty("type", typeTerm)
	if sbType == model.SmartBlockType_Template && len(typeTerms) > 1 {
		doc.setNonEmpty("template_for", typeTerms[1])
	}
	doc.setNonEmpty("key", e.snapshot.Key)
	// every surface that spells a property or type key runs before the
	// envelope is assembled, so the legends they populate land in their
	// canonical positions rather than trailing the blocks that filled them
	properties := e.buildProperties()
	typeProperties := e.buildTypeProperties()
	blocks, err := e.buildBlocks()
	if err != nil {
		return nil, err
	}

	doc.setNonEmpty("properties", properties)
	if typeProperties != nil {
		doc.set("type_properties", typeProperties) // present even when empty (§2a)
	}

	doc.setNonEmpty("property_keys", e.buildPropertyKeys())
	doc.setNonEmpty("type_keys", e.buildTypeKeys())
	// option_ids last of the three legends: its outer keys are property
	// spellings, so the legend that inverts those precedes it (§2). Written
	// unconditionally — this is identity, not compaction — except under
	// OmitIds, where a legend of nothing but ids has no place (§9, §9a).
	if !e.opts.OmitIds {
		doc.setNonEmpty("option_ids", sortedNestedOmap(e.buildOptionIds()))
	}
	doc.setNonEmpty("blocks", blocks)

	items, store := e.buildStore()
	doc.setNonEmpty("items", items)
	doc.setNonEmpty("store", store)
	doc.setNonEmpty("root", e.buildRootEscape())
	return doc, nil
}

func (e *exporter) buildStore() ([]any, *omap) {
	coll := e.snapshot.Collections
	if coll == nil || len(coll.Fields) == 0 {
		return nil, nil
	}
	// the objects key lifts into items only when it is a list; any other
	// shape stays in store so nothing is silently dropped
	var items []any
	objectsLifted := false
	if v := coll.Fields[storeKeyItems]; v != nil {
		if lv, ok := v.GetKind().(*types.Value_ListValue); ok {
			objectsLifted = true
			for _, el := range lv.ListValue.GetValues() {
				if id := el.GetStringValue(); id != "" {
					items = append(items, id)
				}
			}
		}
	}
	store := &omap{}
	keys := make([]string, 0, len(coll.Fields))
	for k := range coll.Fields {
		if k != storeKeyItems || !objectsLifted {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		store.set(k, protoValueToJSON(coll.Fields[k]))
	}
	return items, store
}

func (e *exporter) buildRootEscape() *omap {
	root := e.blocks[e.rootId]
	if root == nil {
		return nil
	}
	m := &omap{}
	if root.Fields != nil && len(root.Fields.Fields) > 0 {
		m.set("fields", protoStructToJSON(root.Fields))
	}
	m.setNonEmpty("background_color", root.BackgroundColor)
	return m
}

//
// ---- properties ----
//

// strippedDetailKeys are the internal/derived properties export removes (§3).
// InternalPropertyKeys reports the property keys this format treats as
// internal: export strips them and import refuses them (§3). It is exported
// for tooling that has to agree with that set — a round-trip checker comparing
// a snapshot with its re-import has to know which keys are *expected* to be
// gone. Two copies of this list have now drifted: cmd/anyblockroundtrip's, and
// the one that moved with it into snapshotdiff. Both reported 10 378 false
// data-loss issues over a 36 808-object account the moment the package added
// the importer's provenance keys.
func InternalPropertyKeys() map[string]bool {
	return strippedDetailKeys()
}

// strippedDetailKeys is the internal-property list, and it is the single
// source of truth for both directions: export removes these keys, and import
// refuses them (§3, §4a — deniedPropertyKey reads this same set). Two lists
// would drift, which is how the import surface ended up strictly wider than
// the export surface.
func strippedDetailKeys() map[string]bool {
	stripped := map[string]bool{detailKeyId: true, detailKeyType: true}
	for _, k := range bundle.LocalAndDerivedRelationKeys {
		if !propertiesKeptOnExport[string(k)] {
			stripped[string(k)] = true
		}
	}
	// the importer's own resolution vectors are bundled relations but not
	// local/derived ones, so the list above does not cover them; they are
	// internal all the same
	for k := range neverWritableProperties {
		stripped[k] = true
	}
	// transient state describes the moment the object was written, not the
	// object; it means nothing on the other side of an import
	for k := range transientProperties {
		stripped[k] = true
	}
	return stripped
}

func (e *exporter) buildProperties() *omap {
	if e.snapshot.Details == nil {
		return nil
	}
	stripped := strippedDetailKeys()
	lifted := e.typePropDetailKeys()
	var keys []string
	for k := range e.snapshot.Details.Fields {
		if stripped[k] || lifted[k] {
			continue
		}
		// a stored detail key is not necessarily a property name: real data
		// holds an empty key and keys with control characters in them, and
		// there is no way to write those (§3). Dropping them is what keeps
		// Marshal's output valid — emitting one produced a document its own
		// Validate rejects, which is the invariant §11 states.
		if !isWritablePropertyKey(k) {
			e.warn("/properties", "property key %q cannot be written in this format and is dropped", k)
			continue
		}
		keys = append(keys, k)
	}
	// the document spells slugs (§7.5a), so the canonical alphabetical order
	// is over the SPELLINGS, not the stored keys — the reader sorts what it
	// sees. Values still resolve through the stored key.
	//
	// The claims below run over the STORED keys sorted, not over map order:
	// which holder keeps a contested spelling must not depend on Go's map
	// iteration, or the canonical form is not canonical and export∘import
	// byte-stability is a coin flip on exactly the spaces that need it most.
	// The collapse discipline itself — a spelling two stored keys agree on
	// would merge into one JSON key and lose a value — lives in the term
	// ledger (propertySlug), where every OTHER key slot claims through it too.
	sort.Strings(keys)
	type prop struct{ slug, key string }
	props := make([]prop, 0, len(keys))
	for _, k := range keys {
		props = append(props, prop{slug: e.propertySlug(k), key: k})
	}
	sort.Slice(props, func(i, j int) bool { return props[i].slug < props[j].slug })
	ordered := make([]prop, 0, len(props))
	seen := map[string]bool{}
	for _, wk := range wellKnownPropertyOrder {
		for _, p := range props {
			if p.key == wk {
				ordered = append(ordered, p)
				seen[p.key] = true
			}
		}
	}
	for _, p := range props {
		if !seen[p.key] {
			ordered = append(ordered, p)
		}
	}
	m := &omap{}
	for _, p := range ordered {
		// presence of a property key is meaningful — it records that the
		// property was set on the object — so values are written verbatim,
		// including empty and default ones (§3); the omit-empty canon applies
		// to block attributes and envelope fields only
		m.set(p.slug, e.propertyValue(p.key, e.snapshot.Details.Fields[p.key]))
	}
	return m
}

// warn reports a warning-grade issue through the caller's sink (§13): a thing
// export had to do that the author would want to know about, but that does not
// make the output invalid. Silent when no sink is wired.
func (e *exporter) warn(path, format string, args ...any) {
	if e.opts.OnWarning == nil {
		return
	}
	e.opts.OnWarning(Issue{Path: path, Message: fmt.Sprintf(format, args...)})
}

func (e *exporter) resolveFormat(key string) (model.RelationFormat, bool) {
	return resolveFormatWith(e.opts, key)
}

// resolveFormatWith applies the §3 resolution order: bundle first, then the
// caller's resolver.
func resolveFormatWith(opts Options, key string) (model.RelationFormat, bool) {
	if f, err := bundle.GetRelationFormat(domain.RelationKey(key)); err == nil {
		return f, true
	}
	if opts.ResolveFormat != nil {
		return opts.ResolveFormat(domain.RelationKey(key))
	}
	return 0, false
}

func (e *exporter) propertyValue(key string, v *types.Value) any {
	// layout is stored as a number and named in the format (§3); a number
	// outside the enum falls through and exports unchanged.
	if isLayoutKey(key) {
		if n, isNum := v.GetKind().(*types.Value_NumberValue); isNum {
			if name := layoutNames.name(model.ObjectTypeLayout(int32(n.NumberValue))); name != "" {
				return name
			}
		}
	}
	format, ok := e.resolveFormat(key)
	if !ok {
		return protoValueToJSON(v)
	}
	switch format {
	case model.RelationFormat_date:
		if n, isNum := v.GetKind().(*types.Value_NumberValue); isNum {
			if s, ok := formatDateValue(n.NumberValue); ok {
				return s
			}
			// no RFC 3339 form: emitting one anyway would write a string
			// parseDate cannot read, so the value would come back as a string
			// on a date property and stay that way (byte-stable, so nothing
			// corrects it). The raw number round-trips instead.
			e.warn("/properties/"+key,
				"date %v has no RFC 3339 form (outside years 0000-9999), so it is written as a raw number; "+
					"a value this large is usually milliseconds where seconds belong", n.NumberValue)
		}
	case model.RelationFormat_status, model.RelationFormat_tag:
		var out []any
		for _, id := range valueStringList(v) {
			out = append(out, e.optionName(key, id))
		}
		return out
	case model.RelationFormat_object, model.RelationFormat_file:
		var out []any
		for _, id := range valueStringList(v) {
			out = append(out, id)
		}
		return out
	}
	return protoValueToJSON(v)
}

// optionName is the one site where export substitutes a name for an option id
// (§3), in property values, filter values and custom orders alike — so it is
// also the one site that records what that name stood for (optionrefs.go).
// The legend entry rides with the substitution rather than behind a flag:
// identity is not compaction, and a document that spells a name without
// saying which option it was is the lossy half this legend exists to close.
func (e *exporter) optionName(key, id string) string {
	if e.opts.ResolveOptions != nil {
		if name, ok := e.opts.ResolveOptions.OptionName(domain.RelationKey(key), id); ok {
			e.recordOptionRef(key, name, id)
			return name
		}
	}
	return id
}

// valueStringList reads a value as a list of strings, accepting the single
// string form.
func valueStringList(v *types.Value) []string {
	if s := v.GetStringValue(); s != "" {
		return []string{s}
	}
	var out []string
	for _, el := range v.GetListValue().GetValues() {
		if s := el.GetStringValue(); s != "" {
			out = append(out, s)
		}
	}
	return out
}

//
// ---- blocks ----
//

// orEmpty substitutes an empty message for a nil one (proto semantics: a nil
// message equals its zero value).
func orEmpty[T any](p *T) *T {
	if p == nil {
		return new(T)
	}
	return p
}

// isStructural reports blocks that are derivable and dropped on export (§7).
func isStructural(b *model.Block) bool {
	switch c := b.Content.(type) {
	case *model.BlockContentOfLayout:
		return orEmpty(c.Layout).Style == model.BlockContentLayout_Header
	case *model.BlockContentOfText:
		style := orEmpty(c.Text).Style
		return style == model.BlockContentText_Title ||
			style == model.BlockContentText_Description
	case *model.BlockContentOfFeaturedRelations:
		return true
	}
	return false
}

// isTransparentContainer reports the §7a transparent containers: a block
// that contributes containment and NOTHING else, so the document spells its
// children and not it. Two shapes qualify — a `Layout/Div`, which is the
// editor's fan-out wrapper (state.wrapChildrenToDiv mints one whenever a
// parent exceeds 40 children: a rendering budget, not an authored block),
// and a block whose content oneof is unset, which is legacy data with no
// content to render at all.
//
// The test is on CONTENT, never on the `div-` id prefix the normalizer
// happens to mint: keying on a prefix would make id SPELLING semantically
// load-bearing — the worst thing to freeze — and would leave an authored
// `{"type": "group"}` round-tripping into a permanent wrapper.
func isTransparentContainer(b *model.Block) bool {
	switch c := b.Content.(type) {
	case nil:
		return true
	case *model.BlockContentOfLayout:
		return orEmpty(c.Layout).Style == model.BlockContentLayout_Div
	}
	return false
}

// warnLiftedAttributes reports a transparent container that carried block
// attributes, because the lift drops them with it (§7a) and the loss is
// otherwise invisible. Free on real data — all 7,303 wrappers in the
// production corpus carry none — and it turns a silent loss into a visible
// one for a document that authored a `group` with an attribute on it.
func (e *exporter) warnLiftedAttributes(b *model.Block) {
	if e.opts.OnWarning == nil {
		return
	}
	if b.Align == model.Block_AlignLeft && b.VerticalAlign == model.Block_VerticalAlignTop &&
		b.BackgroundColor == "" && len(b.Fields.GetFields()) == 0 {
		return
	}
	e.warn("/blocks", "block %s is a transparent container (§7a): it is lifted, and the attributes on it are dropped", b.Id)
}

func (e *exporter) buildBlocks() ([]any, error) {
	root := e.blocks[e.rootId]
	if root == nil {
		return nil, nil
	}
	var out []any
	if err := e.appendBlocksFlat(&out, root.ChildrenIds, 0, true); err != nil {
		return nil, err
	}
	return out, nil
}

// appendBlocksFlat walks a subtree in pre-order and appends each block to out
// with its depth as the indent field — the flat encoding (§4 F1–F2). A block
// dropped by blockToJSON (structural, visited, content-less leaf) drops its
// whole subtree, matching the nested encoding's semantics.
func (e *exporter) appendBlocksFlat(out *[]any, ids []string, depth int, topLevel bool) error {
	for _, id := range ids {
		b := e.blocks[id]
		if b == nil {
			continue
		}
		if topLevel && isStructural(b) {
			continue
		}
		// §7a: a transparent container is not a block. Emit nothing for it
		// and walk its children at ITS OWN depth, carrying its topLevel flag
		// — the children take the position it held.
		//
		// Before the depth check, so both the value compared against the
		// bound and the emitted indent are post-lift; and marking it visited
		// HERE, because the lift skips blockToJSON, which is where that mark
		// is set — a ChildrenIds cycle through a chain of containers would
		// otherwise recurse until the stack gives out.
		if isTransparentContainer(b) {
			if b.Id != "" {
				if e.visited[b.Id] {
					continue
				}
				e.visited[b.Id] = true
			}
			e.warnLiftedAttributes(b)
			if err := e.appendBlocksFlat(out, b.ChildrenIds, depth, topLevel); err != nil {
				return err
			}
			continue
		}
		emitDepth := depth
		if depth > maxBlockIndent {
			if e.opts.OnWarning == nil {
				return fmt.Errorf("block %s: nesting depth %d exceeds the format bound %d", id, depth, maxBlockIndent)
			}
			// read path (C11): degrade rather than fail — clamp the indent and
			// keep the content instead of erroring the whole document.
			e.opts.OnWarning(Issue{Path: "/blocks", Message: fmt.Sprintf("block %s: nesting depth %d exceeds the bound %d — indent clamped", id, depth, maxBlockIndent)})
			emitDepth = maxBlockIndent
		}
		m, withChildren, err := e.blockToJSON(b, emitDepth)
		if err != nil {
			return err
		}
		if m == nil {
			continue
		}
		e.recordEmitted(b.Id)
		*out = append(*out, m)
		if withChildren {
			if err := e.appendBlocksFlat(out, b.ChildrenIds, depth+1, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// recordEmitted notes that this run wrote the block/row/column/view stored
// under id. Only the census probe collects; every other run no-ops.
func (e *exporter) recordEmitted(id string) {
	if e.emitted != nil && id != "" {
		e.emitted[id] = true
	}
}

func (e *exporter) localId(id string) string {
	if e.opts.compactBlockLabels() {
		if short, ok := e.localIds[id]; ok {
			return short
		}
	}
	return id
}

// blockToJSON renders one block at the given depth (its indent, written
// first per the §4 canonical key order). The returned bool reports whether
// the caller should descend into the block's children.
func (e *exporter) blockToJSON(b *model.Block, depth int) (*omap, bool, error) {
	// a snapshot's block graph is untrusted: without this, a ChildrenIds
	// cycle recurses to an unrecoverable stack overflow, and a block shared
	// by two parents is emitted twice (duplicate ids fail validation)
	if b.Id != "" {
		if e.visited[b.Id] {
			return nil, false, nil
		}
		e.visited[b.Id] = true
	}
	m := &omap{}
	m.setNonEmpty("indent", depth)
	if !e.opts.OmitIds {
		m.setNonEmpty("id", e.blockLabel(b.Id))
	}
	liftedFields := map[string]bool{}
	withChildren := true

	// nil inner messages are proto-equivalent to empty ones — never panic
	switch c := b.Content.(type) {
	case nil:
		// legacy content-less blocks exist in old accounts: relation objects
		// carry a bare wrapper around their "used in" dataview, and pages can
		// hold orphaned empty leaves. Both are transparent containers (§7a),
		// lifted before this is ever reached from the document walk; the one
		// caller that does reach it is a CELL, which cannot lift because the
		// cell is the position (cellToJSON turns it into an empty cell).
		return nil, false, nil
	case *model.BlockContentOfText:
		if err := e.textToJSON(m, b, orEmpty(c.Text), liftedFields); err != nil {
			return nil, false, err
		}
	case *model.BlockContentOfFile:
		// file blocks are leaves in the editor, but legacy data holds real
		// text children under them — dropping those would be silent loss
		e.fileToJSON(m, orEmpty(c.File))
	case *model.BlockContentOfBookmark:
		bm := orEmpty(c.Bookmark)
		m.set("type", "bookmark")
		m.setNonEmpty("url", bm.Url)
		m.setNonEmpty("object_id", bm.TargetObjectId)
		withChildren = false
	case *model.BlockContentOfLink:
		l := orEmpty(c.Link)
		m.set("type", "link")
		m.setNonEmpty("object_id", l.TargetBlockId)
		if l.CardStyle != model.BlockContentLink_Text {
			m.setNonEmpty("card_style", cardStyleNames.name(l.CardStyle))
		}
		if l.IconSize != model.BlockContentLink_SizeNone {
			m.setNonEmpty("icon_size", iconSizeNames.name(l.IconSize))
		}
		if l.Description != model.BlockContentLink_None {
			m.setNonEmpty("description", linkDescriptionNames.name(l.Description))
		}
		m.setNonEmpty("properties", stringsToAny(e.propertySlugs(l.Relations)))
		withChildren = false
	case *model.BlockContentOfDiv:
		m.set("type", "divider")
		if style := orEmpty(c.Div).Style; style != model.BlockContentDiv_Line {
			m.setNonEmpty("style", divStyleNames.name(style))
		}
		withChildren = false
	case *model.BlockContentOfLayout:
		switch orEmpty(c.Layout).Style {
		case model.BlockContentLayout_Row:
			m.set("type", "row")
		case model.BlockContentLayout_Column:
			m.set("type", "column")
		default:
			// header and stray table wrappers are structural (§7); a Div is
			// a transparent container (§7a) and is lifted before this — a
			// cell that IS one lands here and renders as an empty cell
			return nil, false, nil
		}
	case *model.BlockContentOfTable:
		if err := e.tableToJSON(m, b); err != nil {
			return nil, false, err
		}
		withChildren = false
	case *model.BlockContentOfLatex:
		lx := orEmpty(c.Latex)
		m.set("type", "embed")
		if lx.Processor != model.BlockContentLatex_Latex {
			m.setNonEmpty("processor", processorNames.name(lx.Processor))
		}
		m.setNonEmpty("text", lx.Text)
		withChildren = false
	case *model.BlockContentOfTableOfContents:
		m.set("type", "table_of_contents")
		withChildren = false
	case *model.BlockContentOfRelation:
		// a property block IS a reference to a property, so one with no key
		// refers to nothing. It used to be emitted as `{"type": "property"}`
		// — a block the schema accepted, import stored with the empty key,
		// and the next export wrote again, forever. Dropped, like the
		// nameless sort and the nameless column (§6).
		if orEmpty(c.Relation).Key == "" {
			e.warn("", "a property block names no property and is dropped; "+
				"a key slot has to name something (§3)")
			return nil, false, nil
		}
		m.set("type", "property")
		m.setNonEmpty("key", e.propertySlug(orEmpty(c.Relation).Key))
		withChildren = false
	case *model.BlockContentOfDataview:
		if err := e.dataviewToJSON(m, orEmpty(c.Dataview)); err != nil {
			return nil, false, err
		}
		withChildren = false
	case *model.BlockContentOfWidget:
		w := orEmpty(c.Widget)
		m.set("type", "widget")
		if w.Layout != model.BlockContentWidget_Link {
			m.setNonEmpty("layout", widgetLayoutNames.name(w.Layout))
		}
		m.setNonEmpty("limit", w.Limit)
		m.setNonEmpty("view_id", w.ViewId)
		m.setNonEmpty("auto_added", w.AutoAdded)
	case *model.BlockContentOfChat:
		m.set("type", "chat")
		withChildren = false
	case *model.BlockContentOfFeaturedRelations:
		m.set("type", "featured_properties")
		withChildren = false
	case *model.BlockContentOfIcon:
		m.set("type", "icon")
		m.setNonEmpty("name", orEmpty(c.Icon).Name)
		withChildren = false
	case *model.BlockContentOfSmartblock:
		return nil, false, nil
	default:
		if e.opts.OnWarning != nil {
			// read path (C11): drop the unrepresentable block with a warning
			// instead of failing the whole read.
			e.opts.OnWarning(Issue{Path: "/blocks", Message: fmt.Sprintf("block %s: content type %T has no JSON mapping — dropped", b.Id, b.Content)})
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("block %s: content type %T has no JSON mapping", b.Id, b.Content)
	}

	e.finishBlockJSON(m, b, liftedFields)
	return m, withChildren, nil
}

// finishBlockJSON writes the common block tail: align, verticalAlign,
// backgroundColor, fields — in the §4 canonical order.
func (e *exporter) finishBlockJSON(m *omap, b *model.Block, liftedFields map[string]bool) {
	if b.Align != model.Block_AlignLeft {
		m.setNonEmpty("align", alignNames.name(b.Align))
	}
	if b.VerticalAlign != model.Block_VerticalAlignTop {
		m.setNonEmpty("vertical_align", verticalAlignNames.name(b.VerticalAlign))
	}
	m.setNonEmpty("background_color", b.BackgroundColor)
	m.setNonEmpty("fields", e.fieldsToJSON(b.Fields, liftedFields))
}

func (e *exporter) fieldsToJSON(fields *types.Struct, lifted map[string]bool) *omap {
	if fields == nil || len(fields.Fields) == 0 {
		return nil
	}
	m := &omap{}
	keys := make([]string, 0, len(fields.Fields))
	for k := range fields.Fields {
		if !lifted[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		m.set(k, protoValueToJSON(fields.Fields[k]))
	}
	return m
}

func (e *exporter) textToJSON(m *omap, b *model.Block, t *model.BlockContentText, liftedFields map[string]bool) error {
	style := t.Style
	// deprecated Header4 exports as heading3 (§5)
	if style == model.BlockContentText_Header4 {
		style = model.BlockContentText_Header3
	}
	typ := textStyleNames.name(style)
	if typ == "" {
		return fmt.Errorf("block %s: text style %v has no JSON mapping", b.Id, t.Style)
	}
	m.set("type", typ)

	if style == model.BlockContentText_Checkbox {
		m.setNonEmpty("checked", t.Checked)
	}
	if style == model.BlockContentText_Callout {
		m.setNonEmpty("icon_emoji", t.IconEmoji)
		m.setNonEmpty("icon_image", t.IconImage)
	}
	if style == model.BlockContentText_Code {
		if b.Fields != nil {
			if lang := b.Fields.Fields[codeLangField].GetStringValue(); lang != "" {
				m.set("language", lang)
				liftedFields[codeLangField] = true
			}
		}
		// literal text; stored marks and color dropped (§8.4, §11)
		m.setNonEmpty("text", t.Text)
		return nil
	}
	m.setNonEmpty("color", t.Color)
	m.setNonEmpty("text", renderInline(t.Text, t.Marks.GetMarks()))
	return nil
}

func (e *exporter) fileToJSON(m *omap, f *model.BlockContentFile) {
	typ := fileTypeNames.name(f.Type)
	if typ == "" {
		typ = "file" // Type_None (§5)
	}
	m.set("type", typ)
	objectId := f.TargetObjectId
	if objectId == "" {
		objectId = f.Hash // legacy content address migrates to objectId
	}
	m.setNonEmpty("object_id", objectId)
	m.setNonEmpty("name", f.Name)
	m.setNonEmpty("mime_type", f.Mime)
	m.setNonEmpty("size", f.Size_)
	if f.Style != model.BlockContentFile_Auto {
		m.setNonEmpty("style", fileStyleNames.name(f.Style))
	}
	if f.AddedAt != 0 {
		// addedAt is a string in the schema, so there is no number form to
		// fall back to (§5): an unrepresentable timestamp is dropped rather
		// than written as a string no reader can parse back
		if s, ok := formatDate(f.AddedAt); ok {
			m.set("added_at", s)
		} else {
			e.warn("", "file block: added_at %d has no RFC 3339 form (outside years 0000-9999), so it is omitted", f.AddedAt)
		}
	}
}

func stringsToAny(ss []string) []any {
	var out []any
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

//
// ---- compact ids (§9a) ----
//

// emittedLocalIds is the id census's population: the doc-local ids this
// export actually SERVES — every block it writes, plus the table rows,
// columns and dataview views inside them. It runs the block emit a second
// time on a throwaway exporter (own visited map, own ledgers, warnings
// swallowed so nothing is reported twice) rather than re-deriving the drop
// rules, because a second statement of "what export emits" would be a second
// thing to keep in step with blockToJSON, and the census is only correct
// while the two agree exactly (TestExport_CensusPopulationIsWhatExportEmits).
func (e *exporter) emittedLocalIds() map[string]bool {
	opts := e.opts
	if opts.OnWarning != nil {
		// keep the nil-ness — several drop-vs-error decisions read it — but
		// silence the probe: the real run reports the same issues.
		opts.OnWarning = func(Issue) {}
	}
	probe := &exporter{
		opts:     opts,
		snapshot: e.snapshot,
		sbType:   e.sbType,
		blocks:   e.blocks,
		rootId:   e.rootId,
		visited:  map[string]bool{},
		emitted:  map[string]bool{},
	}
	// an error here is the real run's error too, and it fails there with the
	// message the caller should see; the partial census costs nothing.
	_, _ = probe.buildBlocks()
	return probe.emitted
}

// buildLabelPlan works out which doc-local block/row/column/view ids may be
// relabeled to a short suffix (§9a). It walks BOTH id populations to do it:
// the doc-local ids that are the relabeling candidates, and every OBJECT id
// the document references — not because an object id is ever compacted (none
// is, §9a), but because every one of them is spelled verbatim in the output,
// so a label equal to one would make two different things answer to one name.
// mintedSuffixLabels' own census counts local ids only, so that avoid-set is
// the sole guard against it (TestExport_CompactLabelCannotTakeAServedId).
//
// **The local population is what export EMITS, not what the snapshot holds**
// (§9a, mirroring §3's term census). A block the document does not spell —
// a transparent container (§7a), a structural block (§7), a content-less
// leaf, a cell whose id is derived, anything unreachable — is gone from the
// snapshot the round trip rebuilds, so reserving its suffix slot makes
// `Export(S)` and `Export(Import(Export(S)))` disagree: the first read keeps
// a paragraph's id full because an invisible block shares its 5-char tail,
// the second compacts it. That is guarantee 3 (§11), broken on the API's
// default read shape. The protection lost is illusory anyway — a container
// the editor re-creates gets a FRESH id no census could have reserved
// against.
func (e *exporter) buildLabelPlan() {
	objects := map[string]bool{}
	locals := e.emittedLocalIds()
	addObject := func(id string) {
		if id != "" {
			objects[id] = true
		}
	}

	for _, b := range e.snapshot.Blocks {
		if b == nil {
			continue
		}
		switch c := b.Content.(type) {
		case *model.BlockContentOfText:
			t := orEmpty(c.Text)
			for _, mk := range t.Marks.GetMarks() {
				if mk == nil {
					continue
				}
				switch {
				case mk.Type == model.BlockContentTextMark_Mention || mk.Type == model.BlockContentTextMark_Object:
					addObject(mk.Param)
				case mk.Type == model.BlockContentTextMark_Link && isObjectLink(mk.Param):
					// normalizes to an Object mark on render (§8.3)
					id, _ := parseObjectLink(mk.Param)
					addObject(id)
				}
			}
			addObject(t.IconImage)
		case *model.BlockContentOfFile:
			f := orEmpty(c.File)
			if f.TargetObjectId != "" {
				addObject(f.TargetObjectId)
			} else {
				addObject(f.Hash)
			}
		case *model.BlockContentOfBookmark:
			addObject(orEmpty(c.Bookmark).TargetObjectId)
		case *model.BlockContentOfLink:
			addObject(orEmpty(c.Link).TargetBlockId)
		case *model.BlockContentOfDataview:
			dv := orEmpty(c.Dataview)
			addObject(dv.TargetObjectId)
			for _, v := range dv.Views {
				if v == nil {
					continue
				}
				addObject(v.DefaultTemplateId)
				addObject(v.DefaultObjectTypeId)
				for _, f := range flattenFilters(v.Filters) {
					if format, ok := e.dvFormat(dv, f.RelationKey); ok &&
						(format == model.RelationFormat_object || format == model.RelationFormat_file) {
						for _, id := range valueStringList(f.Value) {
							addObject(id)
						}
					}
				}
				for _, s := range v.Sorts {
					if s == nil {
						continue
					}
					if format, ok := e.dvFormat(dv, s.RelationKey); ok &&
						(format == model.RelationFormat_object || format == model.RelationFormat_file) {
						for _, cv := range s.CustomOrder {
							for _, id := range valueStringList(cv) {
								addObject(id)
							}
						}
					}
				}
			}
			for _, oo := range dv.ObjectOrders {
				if oo == nil {
					continue
				}
				for _, id := range oo.ObjectIds {
					addObject(id)
				}
			}
		}
	}

	if e.snapshot.Details != nil {
		stripped := strippedDetailKeys()
		lifted := e.typePropDetailKeys()
		for key, v := range e.snapshot.Details.Fields {
			if stripped[key] || lifted[key] {
				continue // stripped/lifted properties never appear as ids, so no legend entry
			}
			format, ok := e.resolveFormat(key)
			if ok && (format == model.RelationFormat_object || format == model.RelationFormat_file) {
				for _, id := range valueStringList(v) {
					addObject(id)
				}
			}
		}
	}
	if e.snapshot.Collections != nil {
		if v := e.snapshot.Collections.Fields[storeKeyItems]; v != nil {
			for _, el := range v.GetListValue().GetValues() {
				addObject(el.GetStringValue())
			}
		}
	}
	// the envelope id is never compacted (§9a)
	delete(objects, e.objectId())

	// no label may equal a full id present in the document (§9a); the avoid
	// set covers every id this export knows about, object ids included —
	// every one of those is written verbatim now, which makes the object
	// half of this set matter MORE than it did when they were compacted
	fullIds := map[string]bool{e.objectId(): true}
	for id := range objects {
		fullIds[id] = true
	}
	for id := range locals {
		fullIds[id] = true
	}
	// only machine-minted opaque ids relabel (isMintedLocalId); every id that
	// keeps its full spelling is reserved through the fullIds avoid-set, so no
	// label can alias a served id — and the census inside mintedSuffixLabels
	// runs over ALL local ids, so a label cannot be an ambiguous suffix of one
	// either. For the LOCAL population the census is the binding guard (it
	// counts a short id as itself); the avoid-set carries the OBJECT
	// population, which the census never sees. Labels stay dash-free as
	// before: '-' is the derived-cell-id separator and forbidden in row/column
	// ids (§6.1) — minted suffixes are hex, so the check is a backstop.
	e.localIds = mintedSuffixLabels(setToSlice(locals), compactIdMinLen, func(candidate string) bool {
		return fullIds[candidate] || isInvalidLocalLabel(candidate)
	})
}

// isBlockIdLabel reports whether s matches the block-id pattern
// ^[A-Za-z0-9_-]{1,64}$ — the charset §4 puts on a block, row or column id.
// Named for the deleted `refs` legend, whose plain keys carried the same
// charset; the id sanitizer is the caller that remains.
func isBlockIdLabel(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !isLabelRune(r) && r != '-' {
			return false
		}
	}
	return true
}

// isInvalidLocalLabel rejects relabel candidates for blocks/rows/columns/
// views: the row/column charset has no dash (§6.1), which also keeps labels
// clear of derived cell ids.
func isInvalidLocalLabel(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return true
	}
	for _, r := range s {
		if !isLabelRune(r) {
			return true
		}
	}
	return false
}

func isLabelRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func flattenFilters(filters []*model.BlockContentDataviewFilter) []*model.BlockContentDataviewFilter {
	var out []*model.BlockContentDataviewFilter
	var walk func([]*model.BlockContentDataviewFilter)
	walk = func(fs []*model.BlockContentDataviewFilter) {
		for _, f := range fs {
			if f == nil {
				continue
			}
			out = append(out, f)
			walk(f.NestedFilters)
		}
	}
	walk(filters)
	return out
}

func setToSlice(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
