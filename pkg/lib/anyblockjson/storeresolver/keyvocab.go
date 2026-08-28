package storeresolver

// keyvocab.go — the space-backed key vocabulary.
//
// The package default (anyblockjson.BundledKeyVocabulary) knows only the
// bundled name table, which is all an offline reader can know. Inside a
// node the space itself is the second authority: every non-bundled type and
// property has a display NAME, and §3 says the document spells THAT — NFC,
// verbatim — not the opaque BSON the store binds and not the api slug the
// API surface mints (`apiObjectKey` is never read by the format; it stays
// the API's affair).
//
// Shape, exactly as §3 prescribes: **one bounded query per kind per
// resolver instance** (i.e. per export/import operation), never one point
// query per reference. The listing that primes it is a DETAILS query rather
// than ListAllRelations, because the vocabulary needs only names, keys and
// flags. Precedence follows the §3 chain, and the ordering is load-bearing:
// an exact live STORED key always wins over the name layer (step 2 before
// any table), so a document naming a relation whose stored key happens to
// be spelled like someone else's name still binds to the relation it named.
// The vocabulary is lazy: a document with no key slots pays nothing.
//
// **Names are not unique, and the vocabulary does not pretend they are.**
// Two live properties may share one name; both spell it, because collisions
// are resolved per DOCUMENT (the exporter's term ledger disambiguates the
// 0.21% of documents where two claimants actually co-occur), not per space.
// The accept side therefore answers a shared spelling with NO single key —
// PropertyKey refuses to guess — and exposes the full candidate list
// through the ScopedKeyVocabulary capability, where the importer resolves
// within the declared type or raises a loud error asking for the legend.
// The one spelling an entity can NEVER take, in any document, is a string
// that is some other live entity's stored key: stored keys resolve
// verbatim-first at every reader, so that claimant degrades through the
// same ladder the document ledger uses — its stored key when that is
// readable, else `<name> (<tail6>)` with the stored key's last six hex,
// else the stored key regardless.

import (
	"sort"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// keyMaps is one namespace's spelling tables plus the stored-key set that
// gives chain step 2 its precedence.
type keyMaps struct {
	// labelByKey is the granted document spelling per live visible entity —
	// the plain NFC name for nearly everyone, the disambiguated form where
	// the name was unusable (see grant). Absent means the entity spells its
	// stored key verbatim, which is always its own address.
	labelByKey map[string]string
	// keysByLabel is the reverse, MULTI-VALUED: a shared name lists every
	// claimant, in sorted-key order, and the accept side refuses to pick.
	keysByLabel map[string][]string
	// nameByKey is the raw NFC display name of every visible non-bundled
	// entity — the diagnostics surface for the glued-annotation warning,
	// kept separate from labelByKey because a degraded label is not the
	// name a writer would have copied.
	nameByKey map[string]string
	storedKey map[string]bool
	// keysByFold is chain step 4 — see fold.
	keysByFold map[string][]string
	// keyById is the stored key of each live entity by OBJECT ID — not part
	// of the resolution chain, which never sees an id, but the inverse the
	// store speaks in: a relation's `relationFormatObjectTypes` holds the
	// target types' object ids, and PropertyDefinition.ObjectTypes is
	// defined in stored type KEYS (§2a). Filled from the same one bounded
	// listing, so the mapping costs nothing extra. Hidden entities are
	// included: identity is not the name namespace.
	//
	// idByKey is its inverse, for the import half of the same translation
	// (anyblockjson.TypeResolver, §2d). First-wins on a duplicated key,
	// like keyById on a duplicated id.
	keyById map[string]string
	idByKey map[string]string
	// propertyIdsByKey exists on the TYPE namespace only: the four
	// recommended property lists of each live type, as the object ids the
	// store holds, read from the same one bounded listing. It is the
	// type-scoped resolution surface (TypePropertyKeys): the declared type
	// is what disambiguates a shared property name for a reader with no
	// legend.
	propertyIdsByKey map[string][]string

	bundledKey  func(spelling string) (string, bool)
	bundledFold func(input string) []string
	// bundled reports whether the bundled table speaks for a stored key.
	// Such a key takes its spelling from the code table in every space and
	// offline (§3), so the space's own row never derives one for it.
	bundled func(key string) bool
	// label is the namespace's half of the §3 label rule
	// (anyblockjson.PropertyLabel / TypeLabel): NFC(name), else nothing.
	label func(key, name string) string
}

// namespace is one key namespace's configuration: the listing it loads
// from, the two bundled tables it consults, and its label rule.
type namespace struct {
	layout      model.ObjectTypeLayout
	keyOf       func(*domain.Details) string
	label       func(key, name string) string
	bundled     func(key string) bool
	bundledKey  func(spelling string) (string, bool)
	bundledFold func(input string) []string
}

// entity is one row of the one bounded listing: everything the space stores
// that a spelling can be built from.
type entity struct {
	key    string
	name   string
	hidden bool
}

func newKeyMaps(ns namespace) *keyMaps {
	return &keyMaps{
		labelByKey:       map[string]string{},
		keysByLabel:      map[string][]string{},
		nameByKey:        map[string]string{},
		storedKey:        map[string]bool{},
		keysByFold:       map[string][]string{},
		keyById:          map[string]string{},
		idByKey:          map[string]string{},
		propertyIdsByKey: map[string][]string{},
		bundledKey:       ns.bundledKey,
		bundledFold:      ns.bundledFold,
		bundled:          ns.bundled,
		label:            ns.label,
	}
}

// add records one live entity.
//
// A HIDDEN entity keeps its stored key (chain step 2 — the stored key is
// always an address, and the emit side must still refuse to grant a
// spelling it owns) but does NOT enter the name namespace, exactly as v2's
// request namespace has it (core/api/v2/service/keys.go,
// propertyEntry.Hidden). A BUNDLED entity keeps its stored key too and
// contributes nothing else: its spelling is the code table's in every space
// and offline, and its folds are the bundled fold table's — letting a space
// row's name speak for it would let a renamed local copy move a spelling
// that ships with every reader.
func (m *keyMaps) add(row entity) {
	if row.key == "" {
		return
	}
	m.storedKey[row.key] = true
}

// grant runs the label pass for one visible non-bundled entity, after every
// stored key is known — the ordering is the whole reason granting is a
// second pass: the one hard refusal below needs the complete stored-key
// set, and the order rows arrive in must not decide anyone's spelling.
//
// A shared name is NOT refused: collisions are per-document (the exporter's
// ledger), so every claimant is granted the plain name and keysByLabel
// holds them all. The one spelling no entity may take is a string that is
// some other live entity's STORED KEY — verbatim-first outranks every
// table, so such a label could never resolve to its owner anywhere. That
// claimant degrades through the same ladder the document ledger uses:
//
//	(a) its stored key, by granting NO label, when the key is readable
//	    (not a minted 24-hex bson id);
//	(b) else `<name> (<tail6>)`, tail6 = the stored key's last six hex —
//	    deterministic, immutable, visibly synthetic;
//	(c) else no label, and the stored key is written regardless.
func (m *keyMaps) grant(row entity) {
	if row.key == "" || row.hidden || m.bundled(row.key) {
		return
	}
	name := m.label(row.key, row.name)
	if name != "" {
		m.nameByKey[row.key] = name
	}
	m.addFold(anyblockjson.FoldKeyTerm(row.key), row.key)
	if name == "" {
		return
	}
	m.addFold(anyblockjson.FoldKeyTerm(name), row.key)
	label := name
	if m.storedKey[label] { // necessarily someone else's: label==own key yields "" above
		label = anyblockjson.DisambiguatedKeySpelling(name, row.key)
		if label == "" || m.storedKey[label] {
			return // rung (a) or (c): the stored key is the spelling
		}
		m.addFold(anyblockjson.FoldKeyTerm(label), row.key)
	}
	m.labelByKey[row.key] = label
	m.keysByLabel[label] = append(m.keysByLabel[label], row.key)
}

func (m *keyMaps) addFold(fold, key string) {
	for _, existing := range m.keysByFold[fold] {
		if existing == key {
			return
		}
	}
	m.keysByFold[fold] = append(m.keysByFold[fold], key)
}

// candidates is the exact name layer's full answer for a term: every live
// visible claimant of the spelling, plus the bundled table's binding when
// it has one — sorted, deduplicated. It deliberately says nothing about
// stored keys: verbatim-first is the caller's step, asked before this one.
func (m *keyMaps) candidates(term string) []string {
	out := append([]string(nil), m.keysByLabel[term]...)
	if m.bundledKey != nil {
		if key, ok := m.bundledKey(term); ok {
			seen := false
			for _, k := range out {
				if k == key {
					seen = true
					break
				}
			}
			if !seen {
				out = append(out, key)
			}
		}
	}
	sort.Strings(out)
	return out
}

// key is the accept side's exact chain for one term: verbatim-first, then
// the name layer where EXACTLY ONE candidate holds the spelling. Several
// candidates are an ambiguity this method refuses to resolve — the caller
// with type context (the importer, through ScopedKeyVocabulary) may; a
// caller without one degrades to the verbatim term, never to a guess.
func (m *keyMaps) key(term string) (string, bool) {
	if m.storedKey[term] {
		return "", false // chain step 2: an exact stored key wins
	}
	if cands := m.candidates(term); len(cands) == 1 {
		return cands[0], true
	}
	return "", false
}

// fold is chain step 4, the forgiving layer: every exact lookup has already
// failed, so a SINGLE key whose stored key, name or granted label folds to
// the input's class is the intended forgiveness, and several are an
// ambiguity that degrades to the verbatim term — never a guess. Hidden
// holders do not participate, as at every other step.
func (m *keyMaps) fold(input string) (string, bool) {
	stored := m.keysByFold[anyblockjson.FoldKeyTerm(input)]
	candidates := append(make([]string, 0, len(stored)+1), stored...)
	if m.bundledFold != nil {
		seen := make(map[string]bool, len(candidates))
		for _, c := range candidates {
			seen[c] = true
		}
		for _, key := range m.bundledFold(input) {
			if !seen[key] {
				seen[key] = true
				candidates = append(candidates, key)
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return "", false
}

// extendsLiveName reports the live visible entity NAME that term extends
// with trailing text past a word boundary — the glued-annotation
// diagnostic. Longest name wins; equal lengths break lexicographically.
func (m *keyMaps) extendsLiveName(term string) string {
	var best string
	for _, name := range m.nameByKey {
		if !anyblockjson.KeyTermExtendsName(term, name) {
			continue
		}
		if len(name) > len(best) || (len(name) == len(best) && name < best) {
			best = name
		}
	}
	return best
}

func (r *Resolvers) relationKeyMaps() *keyMaps {
	if r.relVocab == nil {
		r.relVocab = r.loadKeyMaps(namespace{
			layout: model.ObjectType_relation,
			keyOf: func(d *domain.Details) string {
				return d.GetString(bundle.RelationKeyRelationKey)
			},
			label: anyblockjson.PropertyLabel,
			bundled: func(key string) bool {
				return bundle.HasRelation(domain.RelationKey(key))
			},
			// the bundled arms run through anyblockjson's name tables, not
			// pkg/lib/bundle's slug tables: bundled keys answer to their
			// display names (and to their key/name fold classes) — the same
			// chain the package-only reader runs, or the two disagree on
			// one spelling
			bundledKey:  anyblockjson.BundledPropertyKeyByName,
			bundledFold: anyblockjson.BundledPropertyKeysByFold,
		})
	}
	return r.relVocab
}

func (r *Resolvers) typeKeyMaps() *keyMaps {
	if r.typeVocab == nil {
		r.typeVocab = r.loadKeyMaps(namespace{
			layout: model.ObjectType_objectType,
			keyOf: func(d *domain.Details) string {
				key, err := domain.GetTypeKeyFromRawUniqueKey(d.GetString(bundle.RelationKeyUniqueKey))
				if err != nil {
					return ""
				}
				return string(key)
			},
			label: anyblockjson.TypeLabel,
			bundled: func(key string) bool {
				return bundle.HasObjectTypeByKey(domain.TypeKey(key))
			},
			bundledKey:  anyblockjson.BundledTypeKeyByName,
			bundledFold: anyblockjson.BundledTypeKeysByFold,
		})
	}
	return r.typeVocab
}

// recommendedListDetailKeys are the four stored lists a type declares its
// properties through — the same lists the exporter's §2a machinery reads,
// restated here because this package reads them off raw details rows.
var recommendedListDetailKeys = []domain.RelationKey{
	bundle.RelationKeyRecommendedFeaturedRelations,
	bundle.RelationKeyRecommendedRelations,
	bundle.RelationKeyRecommendedFileRelations,
	bundle.RelationKeyRecommendedHiddenRelations,
}

// loadKeyMaps runs the one bounded listing. A store error yields an EMPTY
// vocabulary, not a partial one: the caller then falls back to the bundled
// table, which is the offline-safe answer — never a stale or half-built
// map, which would resolve a write against the wrong property.
func (r *Resolvers) loadKeyMaps(ns namespace) *keyMaps {
	maps := newKeyMaps(ns)
	// ONE listing, TWO populations. The uninstalled entity is excluded from
	// the NAME namespace and included in the id→key naming, and the two
	// questions are genuinely different:
	//
	//   - "which entity answers to the spelling `Project`?" — a UI-deleted
	//     type must vacate it, or a new type minted under the freed name is
	//     shadowed by a corpse. That policy is why this listing filtered
	//     uninstalled entities out in the first place.
	//
	//   - "what does the id I am HOLDING point at?" — naming a corpse
	//     claims nothing. The store never removes the type, so the answer
	//     exists; refusing to look was what put raw object ids into
	//     `object_types`, where the slot's vocabulary is type KEYS.
	//
	// The filter therefore lives on the name half, not on the query.
	records, err := r.index.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(ns.layout)),
		},
	}})
	if err != nil {
		return maps
	}
	rows := make([]entity, 0, len(records))
	// live entities first, so that where a freed spelling HAS been retaken
	// the living owner claims id↔key before any corpse sharing its key
	for _, pass := range []bool{false, true} {
		for _, record := range records {
			uninstalled := record.Details.GetBool(bundle.RelationKeyIsUninstalled)
			if uninstalled != pass {
				continue
			}
			key := ns.keyOf(record.Details)
			if !uninstalled {
				rows = append(rows, entity{
					key:    key,
					name:   record.Details.GetString(bundle.RelationKeyName),
					hidden: record.Details.GetBool(bundle.RelationKeyIsHidden),
				})
				if ns.layout == model.ObjectType_objectType && key != "" {
					if _, taken := maps.propertyIdsByKey[key]; !taken {
						var ids []string
						for _, l := range recommendedListDetailKeys {
							ids = append(ids, record.Details.GetStringList(l)...)
						}
						maps.propertyIdsByKey[key] = ids
					}
				}
			}
			if id := record.Details.GetString(bundle.RelationKeyId); id != "" && key != "" {
				if _, taken := maps.keyById[id]; !taken {
					maps.keyById[id] = key
				}
				if _, taken := maps.idByKey[key]; !taken {
					maps.idByKey[key] = id
				}
			}
		}
	}
	// two passes, in this order: every stored key is registered first, and
	// only then are labels granted — the grant's one hard refusal (a name
	// that is someone else's stored key) needs the complete set, and rows
	// are sorted so nothing depends on store order.
	for _, row := range rows {
		maps.add(row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].key < rows[j].key })
	for _, row := range rows {
		maps.grant(row)
	}
	return maps
}

// PropertySlug implements anyblockjson.KeyVocabulary: bundled keys spell
// their display name from the code table (the authority in every space and
// offline — §3), the rest their granted label. Either arm still yields to a
// live stored key that owns the very string (verbatim-first): the granted
// labels were vetted against the stored-key set at build time, so only the
// bundled arm re-checks here.
func (r *Resolvers) PropertySlug(key string) string {
	maps := r.relationKeyMaps()
	if maps.bundled(key) {
		candidate := (anyblockjson.BundledKeyVocabulary{}).PropertySlug(key)
		if candidate != key && !maps.storedKey[candidate] {
			return candidate
		}
		return key
	}
	if label := maps.labelByKey[key]; label != "" {
		return label
	}
	return key
}

func (r *Resolvers) PropertyKey(spelling string) (string, bool) {
	maps := r.relationKeyMaps()
	if maps.storedKey[spelling] {
		return spelling, false // chain step 2 — verbatim, no table consulted
	}
	if key, ok := maps.key(spelling); ok {
		return key, true
	}
	if len(maps.candidates(spelling)) > 1 {
		return spelling, false // ambiguous: never resolved by guess here
	}
	if key, ok := maps.fold(spelling); ok {
		return key, true
	}
	return spelling, false
}

// TypeSlug is PropertySlug for the type namespace.
func (r *Resolvers) TypeSlug(key string) string {
	maps := r.typeKeyMaps()
	if maps.bundled(key) {
		candidate := (anyblockjson.BundledKeyVocabulary{}).TypeSlug(key)
		if candidate != key && !maps.storedKey[candidate] {
			return candidate
		}
		return key
	}
	if label := maps.labelByKey[key]; label != "" {
		return label
	}
	return key
}

func (r *Resolvers) TypeKey(spelling string) (string, bool) {
	maps := r.typeKeyMaps()
	if maps.storedKey[spelling] {
		return spelling, false
	}
	if key, ok := maps.key(spelling); ok {
		return key, true
	}
	if len(maps.candidates(spelling)) > 1 {
		return spelling, false
	}
	if key, ok := maps.fold(spelling); ok {
		return key, true
	}
	return spelling, false
}

// PropertyKeyCandidates, TypeKeyCandidates, TypePropertyKeys,
// PropertyTermFacts and TypeTermFacts implement
// anyblockjson.ScopedKeyVocabulary — the capability the importer discovers
// by assertion to resolve a shared name within the declared type, and to
// diagnose a term it is about to store verbatim.

func (r *Resolvers) PropertyKeyCandidates(spelling string) []string {
	return r.relationKeyMaps().candidates(spelling)
}

func (r *Resolvers) TypeKeyCandidates(spelling string) []string {
	return r.typeKeyMaps().candidates(spelling)
}

// TypePropertyKeys returns the stored property keys the type declares
// through its four recommended lists — the space's own row where the type
// is installed, the bundled table's links otherwise. The store speaks in
// relation object ids, so the answer is translated through the relation
// namespace's id→key map, dropping ids the space cannot name (a dropped id
// only narrows the scope, which degrades toward the loud error rather than
// toward a wrong resolution).
func (r *Resolvers) TypePropertyKeys(typeKey string) []string {
	if typeKey == "" {
		return nil
	}
	tm := r.typeKeyMaps()
	if ids, ok := tm.propertyIdsByKey[typeKey]; ok {
		rm := r.relationKeyMaps()
		keys := make([]string, 0, len(ids))
		for _, id := range ids {
			if key, ok := rm.keyById[id]; ok && key != "" {
				keys = append(keys, key)
			} else if key, err := bundle.RelationKeyFromID(id); err == nil {
				keys = append(keys, string(key))
			}
		}
		return keys
	}
	if t, err := bundle.GetType(domain.TypeKey(typeKey)); err == nil {
		keys := make([]string, 0, len(t.RelationLinks))
		for _, l := range t.RelationLinks {
			if l != nil && l.Key != "" {
				keys = append(keys, l.Key)
			}
		}
		return keys
	}
	return nil
}

func (r *Resolvers) PropertyTermFacts(term string) anyblockjson.KeyTermFacts {
	maps := r.relationKeyMaps()
	return anyblockjson.KeyTermFacts{
		LiveStoredKey: maps.storedKey[term],
		ExtendsName:   maps.extendsLiveName(term),
	}
}

func (r *Resolvers) TypeTermFacts(term string) anyblockjson.KeyTermFacts {
	maps := r.typeKeyMaps()
	return anyblockjson.KeyTermFacts{
		LiveStoredKey: maps.storedKey[term],
		ExtendsName:   maps.extendsLiveName(term),
	}
}

// TypeKeyById and TypeIdByKey implement anyblockjson.TypeResolver (§2d): the
// translation between the type object ids `relationFormatObjectTypes` stores
// and the stored type keys the format spells. It is targetTypeKeys' own
// mapping (keyById, filled from the one bounded type listing) surfaced as
// the capability the codec discovers by assertion, plus the bundled-url arm
// for legacy entries that were never rewritten to derived ids.
//
// Both answer false on a miss, deliberately: the codec's degradation for an
// unanswered entry is verbatim pass-through — its own address (§3) — and an
// invented answer here would translate one direction with nothing to invert
// it on the other.
func (r *Resolvers) TypeKeyById(id string) (string, bool) {
	if key, err := bundle.TypeKeyFromUrl(id); err == nil && key != "" {
		return string(key), true
	}
	if key := r.typeKeyMaps().keyById[id]; key != "" {
		return key, true
	}
	return "", false
}

func (r *Resolvers) TypeIdByKey(key string) (string, bool) {
	if id := r.typeKeyMaps().idByKey[key]; id != "" {
		return id, true
	}
	return "", false
}
