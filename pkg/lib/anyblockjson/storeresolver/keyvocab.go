package storeresolver

// keyvocab.go — the space-backed key vocabulary.
//
// The package default (anyblockjson.BundledKeyVocabulary) knows only the
// bundled derived table, which is all an offline reader can know. Inside a
// node the space itself is the second authority: every non-bundled type and
// property carries a stored `apiObjectKey` slug, and §7.5a says the document
// spells THAT, not the opaque BSON the store binds.
//
// Shape, exactly as §7.5a-2 prescribes: **one bounded query per kind per
// resolver instance** (i.e. per export/import operation), never one point
// query per reference — `apiObjectKey` is an ordinary hidden detail with no
// dedicated index, so a lookup per reference would each pay a scan. The
// listing that primes it is a DETAILS query rather than ListAllRelations,
// because `model.Relation` carries no apiObjectKey.
//
// Precedence follows the §7.5a-5 chain, and the ordering is load-bearing:
// an exact live STORED key always wins over the slug layer (step 1 before
// step 2), so a document naming a relation whose stored key happens to be
// spelled like someone else's slug still binds to the relation it named.
// The vocabulary is lazy: a document with no key slots pays nothing.

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// keyMaps is one namespace's two directions plus the stored-key set that
// gives chain step 1 its precedence. bundledKey is that namespace's bundled
// reverse table — the third chain step, which the EMIT side has to consult
// too (see roundTrips).
type keyMaps struct {
	slugByKey  map[string]string
	keyBySlug  map[string]string
	storedKey  map[string]bool
	keysByFold map[string][]string // chain step 4 — see fold
	// keyById is the stored key of each live entity by OBJECT ID — not part
	// of the resolution chain, which never sees an id, but the inverse the
	// store speaks in: a relation's `relationFormatObjectTypes` holds the
	// target types' object ids, and PropertyDefinition.ObjectTypes is
	// defined in stored type KEYS (§2a). Filled from the same one bounded
	// listing, so the mapping costs nothing extra. Hidden entities are
	// included: identity is not the slug namespace.
	//
	// idByKey is its inverse, for the import half of the same translation
	// (anyblockjson.TypeResolver, §2d): a relation document's `object_types`
	// arrives as type keys and the store wants this space's type object ids
	// back. First-wins on a duplicated key, like keyById on a duplicated id.
	keyById    map[string]string
	idByKey    map[string]string
	bundledKey func(slug string) (string, bool)
	// bundledFold is that namespace's bundled fold table (chain step 4's
	// bundled arm), as stored-key strings.
	bundledFold func(input string) []string
	// bundled reports whether the bundled table speaks for a stored key.
	// Such a key takes its spelling from the code table in every space and
	// offline (§7.5a-1), so the space's own row never derives one for it.
	bundled func(key string) bool
	// label is the namespace's half of the §3 label rule
	// (anyblockjson.PropertyLabel / TypeLabel): what a document spells for
	// one stored key, given the entity's stored api slug and display name.
	label func(key, slug, name string) string
	// derived marks the spellings that came from a display NAME rather than
	// from a stored slug — a weaker claim, see bind.
	derived map[string]bool
}

// namespace is one key namespace's half of everything above: the listing it
// loads from, the two bundled tables it consults, and its label rule. It is
// a struct rather than five positional arguments because loadKeyMaps had
// four already and the two namespaces have to answer every one of these the
// same way, which is easier to see in one place.
type namespace struct {
	layout      model.ObjectTypeLayout
	keyOf       func(*domain.Details) string
	label       func(key, slug, name string) string
	bundled     func(key string) bool
	bundledKey  func(slug string) (string, bool)
	bundledFold func(input string) []string
}

// entity is one row of the one bounded listing: everything the space stores
// that a spelling can be built from.
type entity struct {
	key    string
	slug   string
	name   string
	hidden bool
}

func newKeyMaps(ns namespace) *keyMaps {
	return &keyMaps{
		slugByKey:   map[string]string{},
		keyBySlug:   map[string]string{},
		storedKey:   map[string]bool{},
		keysByFold:  map[string][]string{},
		keyById:     map[string]string{},
		idByKey:     map[string]string{},
		derived:     map[string]bool{},
		bundledKey:  ns.bundledKey,
		bundledFold: ns.bundledFold,
		bundled:     ns.bundled,
		label:       ns.label,
	}
}

// add records one live entity. A slug with two holders is dropped from BOTH
// directions: an ambiguous address must never resolve by store order (the git
// rule), and dropping it degrades to the stored key, which always works.
// Clearing only the reverse map left the first holder still EXPORTING a slug
// that import then refused to invert — a document naming an address the
// server itself rejects.
//
// A HIDDEN entity keeps its stored key (chain step 1 — the stored key is
// always an address, and roundTrips must still refuse to emit a spelling it
// owns) but does NOT enter the slug namespace, exactly as v2's request
// namespace has it (core/api/v2/service/keys.go, propertyEntry.Hidden). The
// rule has to be the same in both builders or the two disagree on one
// spelling: a hidden twin used to erase a VISIBLE holder's slug from
// keyBySlug, so a listing advertised `severity` while a POST naming it stored
// `severity` verbatim as a relation key no relation object owns; and a hidden
// squatter holding `due_date` used to win the reverse lookup outright, so the
// bundled property's own slug resolved to the squatter.
func (m *keyMaps) add(row entity) {
	if row.key == "" {
		return
	}
	m.storedKey[row.key] = true
	if row.hidden {
		return
	}
	m.addFold(bundle.FoldApiKey(row.key), row.key)
	if row.slug != "" && row.slug != row.key {
		// the RAW stored slug keeps its fold entry even when the label rule
		// spells the entity differently: documents written before that rule
		// spelled the slug itself, and chain step 4 is what still answers
		// for them
		m.addFold(bundle.FoldApiKey(row.slug), row.key)
	}
	// the display name is offered here ONLY when the row has a slug: there
	// it re-spells that slug's own word within one fold class (§3 label
	// rule), which is still this entity's explicit claim. A SLUGLESS row's
	// name is a weaker claim and belongs to addDerived's second pass, so it
	// must not be seen here — that ordering is the whole reason there are
	// two passes.
	var explicitName string
	if row.slug != "" && row.slug != row.key {
		explicitName = row.name
	}
	m.bind(m.label(row.key, row.slug, explicitName), row.key, false)
}

// addDerived is the second pass: the label a display NAME derives (§3 label
// rule step 3), offered only by an entity that has no spelling of its own.
//
// It runs as a pass rather than inline because a derived label is a WEAKER
// claim than a stored slug, and the twin rule cannot tell them apart. In the
// production corpus 14 name-derived labels land on a spelling some other
// relation already stores as its `apiObjectKey` — `more_information`,
// `active_competitors`, `killme` — and the one-pass rule would have dropped
// BOTH, so a relation that has had a readable, explicitly minted address for
// years would start spelling itself with a bson id because an unrelated
// relation was NAMED something similar. Two passes make the ordering a rule
// instead of an accident: every explicit slug is bound first, and a derived
// label takes only what is still free.
//
// A bundled key never reaches here: its spelling is the code table's in
// every space (§7.5a-1), so a space row's name has no say in it. Letting it
// in would also let a localized bundled name squat a label some space-minted
// relation could have used.
func (m *keyMaps) addDerived(row entity) {
	if row.key == "" || row.hidden || m.bundled(row.key) {
		return
	}
	if _, spelled := m.slugByKey[row.key]; spelled {
		return
	}
	m.bind(m.label(row.key, "", row.name), row.key, true)
}

// bind records one spelling, or refuses it. Two holders of one spelling
// still drop it from BOTH directions when their claims are equal — an
// ambiguous address must never resolve by store order (the git rule), and
// dropping it degrades to the stored key, which always works; clearing only
// the reverse map left the first holder still EXPORTING a slug that import
// then refused to invert, a document naming an address the server itself
// rejects. When the claims are NOT equal — a derived name against a stored
// slug — the explicit one keeps the spelling and the derived one goes
// without, which is the whole reason addDerived is a second pass.
func (m *keyMaps) bind(label, key string, derived bool) {
	if label == "" {
		return
	}
	if derived && !m.mintable(label, key) {
		return
	}
	// the fold class is not the slug namespace, and the entry goes in
	// whether or not the exact spelling is granted: an entity answers to its
	// own label's fold either way, and TWO entities in one fold class is the
	// ambiguity chain step 4 refuses rather than resolves. Folding only on
	// success made the forgiving layer hand a CONTESTED spelling to whichever
	// holder the store listed first — the one answer the twin rule exists to
	// prevent.
	m.addFold(bundle.FoldApiKey(label), key)
	if first, taken := m.keyBySlug[label]; taken {
		if derived && !m.derived[label] {
			return
		}
		m.keyBySlug[label] = "" // twin claims: neither wins
		delete(m.slugByKey, first)
		return
	}
	m.keyBySlug[label] = key
	m.slugByKey[key] = label
	if derived {
		m.derived[label] = true
	}
}

// roundTrips reports whether emitting `slug` for `key` inverts back to `key`
// through the §7.5a-5 chain — the emit side's whole obligation. An emitted
// spelling that resolves elsewhere does not degrade a document, it MISLABELS
// it: the value belongs to one entity and the key names another.
//
// Three ways it can fail, one per chain step:
//   - a live stored key answers to the spelling (step 1 wins over any slug);
//   - another live holder claims the slug (step 2 is ambiguous → a loud 400);
//   - the bundled table resolves it to a different key (step 3 — the §7.5a-6
//     shadow, e.g. a UI property that took `due_date` while bundled `dueDate`
//     derives it).
//
// This is the same predicate the API's servedKey applies to a listing row, and
// it must stay the same: the address a document carries and the address the
// listing advertises are the same address.
func (m *keyMaps) roundTrips(slug, key string) bool {
	if slug == "" || slug == key {
		return false
	}
	if m.storedKey[slug] {
		return false
	}
	if holder, ok := m.keyBySlug[slug]; ok && holder != key {
		return false
	}
	if m.bundledKey != nil {
		if other, ok := m.bundledKey(slug); ok && other != key {
			return false
		}
	}
	return true
}

func (m *keyMaps) key(slug string) (string, bool) {
	if m.storedKey[slug] {
		return "", false // chain step 1: an exact stored key wins
	}
	k, ok := m.keyBySlug[slug]
	if !ok || k == "" {
		return "", false
	}
	// The accept side owes the same answer the emit side gives. roundTrips
	// refuses to SPELL this holder with a slug the bundled table resolves
	// elsewhere; without the same guard here, accept BINDS that spelling to
	// this holder — so a document naming the bundled key `priority` lands on
	// whichever custom relation claimed `priority` as its api key: the
	// index is built from every holder's stored slug, and only the emit
	// side was filtering it. That is the mechanism behind the 12 re-pointed
	// objects a 36 808-object sweep found, established afterwards by the
	// unit fixture in keyvocab_test.go rather than by re-measurement —
	// those 12 objects have not been swept again since the guard landed.
	if m.bundledKey != nil {
		if other, ok := m.bundledKey(slug); ok && other != k {
			return "", false
		}
	}
	return k, true
}

// mintable reports whether a DERIVED label may be bound at all — the half of
// roundTrips that does not depend on what the other rows claim, asked before
// the claim rather than after.
//
// A stored slug is a fact about the space, so a slug that shadows a bundled
// spelling stays in the map and makes that spelling ambiguous for everyone
// (the §7.5a-6 shadow, which the API surface has too). A derived label is
// not a fact — it is minted here — so minting one that is already answered
// elsewhere gains nothing and costs the true owner its spelling: a space
// holding a relation NAMED `due_date` bound the label to it, the accept side
// refused it (keyMaps.key will not resolve a spelling the bundled table binds
// elsewhere), and BUNDLED `dueDate` was left spelling itself `dueDate` in
// that space — a spelling nobody could use, taken from the one key that could.
// Two production spaces, two bundled properties (`dueDate`, `iconEmoji`).
func (m *keyMaps) mintable(label, key string) bool {
	if m.storedKey[label] {
		return false // chain step 1 answers first; a label can never outrank it
	}
	if m.bundledKey != nil {
		if other, ok := m.bundledKey(label); ok && other != key {
			return false
		}
	}
	return true
}

func (m *keyMaps) addFold(fold, key string) {
	for _, existing := range m.keysByFold[fold] {
		if existing == key {
			return
		}
	}
	m.keysByFold[fold] = append(m.keysByFold[fold], key)
}

// fold is chain step 4, the forgiving layer (§7.5a-3): every exact lookup has
// already failed, so a SINGLE key whose stored key or stored slug folds to the
// input's fold is the intended forgiveness, and several are an ambiguity that
// degrades to the verbatim term — never a guess.
//
// The accept half had no step 4 at all, while the v2 ROUTE layer did: a
// dataview or a properties map naming `Severity` or `sever_ity` stored the
// term verbatim as a column key, 200 OK, though both fold to the live
// property that the very same request would have found through /properties.
// Only updateView was covered (canonicalViewKey). Hidden holders do not
// participate, as at every other step.
func (m *keyMaps) fold(input string) (string, bool) {
	stored := m.keysByFold[bundle.FoldApiKey(input)]
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
			// the bundled arms run through anyblockjson's v0.38 alias layer,
			// not pkg/lib/bundle directly: the sixteen relation-spelled
			// bundled keys answer to their property spellings (and folds),
			// and the vacated slugs answer to nothing — the same chain the
			// package-only reader runs, or the two disagree on one spelling
			bundledKey: func(slug string) (string, bool) {
				if key, ok := (anyblockjson.BundledKeyVocabulary{}).PropertyKey(slug); ok {
					return key, true
				}
				return "", false
			},
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
			// the alias-aware bundled arms, exactly as the relation
			// namespace above
			bundledKey: func(slug string) (string, bool) {
				if key, ok := (anyblockjson.BundledKeyVocabulary{}).TypeKey(slug); ok {
					return key, true
				}
				return "", false
			},
			bundledFold: anyblockjson.BundledTypeKeysByFold,
		})
	}
	return r.typeVocab
}

// loadKeyMaps runs the one bounded listing. A store error yields an EMPTY
// vocabulary, not a partial one: the caller then falls back to the bundled
// table, which is the offline-safe answer — never a stale or half-built map,
// which would resolve a write against the wrong property (the exact
// silent-failure class §7.5a-2 forbids a cache from ever producing).
func (r *Resolvers) loadKeyMaps(ns namespace) *keyMaps {
	maps := newKeyMaps(ns)
	// ONE listing, TWO populations. The uninstalled entity is excluded from
	// the SLUG namespace and included in the id→key naming, and the two
	// questions are genuinely different:
	//
	//   - "which entity owns the spelling `project`?" — a UI-deleted type
	//     must vacate it, or a new type minted under the freed spelling is
	//     shadowed by a corpse (§7.5 requirement 2). That policy is why this
	//     listing filtered uninstalled entities out in the first place.
	//
	//   - "what does the id I am HOLDING point at?" — naming a corpse claims
	//     nothing. The store never removes the type, so the answer exists;
	//     refusing to look was what put raw object ids into `object_types`,
	//     where the slot's vocabulary is type KEYS. Measured before this:
	//     59 of 232 targets on property documents and 55 of 726 on
	//     dictionary entries were unresolved object ids — and an object id
	//     is worthless in a bundle anyway, because it differs in every space
	//     while a key does not.
	//
	// The filter therefore moves off the query and onto the slug half.
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
					slug:   record.Details.GetString(bundle.RelationKeyApiObjectKey),
					name:   record.Details.GetString(bundle.RelationKeyName),
					hidden: record.Details.GetBool(bundle.RelationKeyIsHidden),
				})
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
	// two passes, in this order: every explicit stored slug claims its
	// spelling first, and only then do the display names take what is left
	// (addDerived). The listing is materialized for exactly that reason —
	// the order rows arrive in must not decide which of two entities keeps
	// a contested spelling.
	for _, row := range rows {
		maps.add(row)
	}
	for _, row := range rows {
		maps.addDerived(row)
	}
	return maps
}

// PropertySlug implements anyblockjson.KeyVocabulary: bundled keys spell as
// their DERIVED slug (the code table is their authority in every space and
// offline — §7.5a-1), the rest as their STORED slug. Either way the spelling
// is emitted only when it round-trips back to this very key (keyMaps.
// roundTrips): the alternative is not a lost slug but a mislabeled value.
//
// A bundled key therefore consults the space's vocabulary too, which costs the
// one bounded listing §7.5a-2 budgets — the same query the custom path already
// pays, and the price of the emit half implementing the same chain the accept
// half does. Without it, a space where a UI property squats `due_date` emitted
// the bundled property's value under a key naming the squatter.
func (r *Resolvers) PropertySlug(key string) string {
	maps := r.relationKeyMaps()
	candidate := maps.slugByKey[key]
	if bundle.HasRelation(domain.RelationKey(key)) {
		// through the alias layer, so the sixteen relation-spelled bundled
		// keys emit their property spellings here too (v0.38, alias.go)
		candidate = (anyblockjson.BundledKeyVocabulary{}).PropertySlug(key)
	}
	if maps.roundTrips(candidate, key) {
		return candidate
	}
	return key
}

func (r *Resolvers) PropertyKey(slug string) (string, bool) {
	maps := r.relationKeyMaps()
	if key, ok := maps.key(slug); ok {
		return key, true
	}
	if maps.storedKey[slug] {
		return slug, false // chain step 1 — do not consult the bundled table
	}
	if key, ok := (anyblockjson.BundledKeyVocabulary{}).PropertyKey(slug); ok {
		return key, true
	}
	if key, ok := maps.fold(slug); ok {
		return key, true
	}
	return slug, false
}

// TypeSlug is PropertySlug for the type namespace.
func (r *Resolvers) TypeSlug(key string) string {
	maps := r.typeKeyMaps()
	candidate := maps.slugByKey[key]
	if bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
		candidate = (anyblockjson.BundledKeyVocabulary{}).TypeSlug(key)
	}
	if maps.roundTrips(candidate, key) {
		return candidate
	}
	return key
}

func (r *Resolvers) TypeKey(slug string) (string, bool) {
	maps := r.typeKeyMaps()
	if key, ok := maps.key(slug); ok {
		return key, true
	}
	if maps.storedKey[slug] {
		return slug, false
	}
	if key, ok := (anyblockjson.BundledKeyVocabulary{}).TypeKey(slug); ok {
		return key, true
	}
	if key, ok := maps.fold(slug); ok {
		return key, true
	}
	return slug, false
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
