// Package storeresolver provides objectstore-backed resolvers for the
// anyblockjson Options: property formats, select/multiSelect option names,
// and property definitions, all read from one space's index. It is the
// standard wiring for exports/imports that run inside a full node
// (API v2 reads, cmd/anyblockroundtrip).
package storeresolver

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Resolvers implements anyblockjson.OptionResolver and
// anyblockjson.PropertyResolver over a space index, with per-instance
// caching. Instances are not safe for concurrent use; create one per
// export/import operation.
type Resolvers struct {
	index      spaceindex.Store
	optionsFor map[domain.RelationKey][]*model.RelationOption

	// participantNames caches ParticipantName's answers, misses included —
	// an empty value IS the miss, so an unnamed or unknown participant is
	// asked about once per export rather than once per object.
	participantNames map[string]string

	// objectRows caches one point lookup per referenced object id, answering
	// BOTH object-namespace questions from it: ObjectName's (the display
	// name; miss = "") and ObjectExists's (whether the index holds a row at
	// all). One cache rather than two because they are one GetDetails — an
	// export asks about the same referenced id once per slot it appears in,
	// and splitting the caches would pay the lookup twice. The two answers
	// MUST stay distinct inside the entry: a present-and-unnamed object and
	// an absent id both answer ObjectName with "", and collapsing them is
	// exactly the conflation ObjectExists exists to avoid (§9). A store
	// error caches nothing: an unasked question has no answer worth
	// remembering, and a transient failure must not become a persistent
	// wrong claim of absence.
	objectRows map[string]objectRow

	relsLoaded bool
	relById    map[string]anyblockjson.PropertyDefinition
	relKeyToId map[string]string

	// the §3 key vocabulary, primed lazily — see keyvocab.go
	relVocab  *keyMaps
	typeVocab *keyMaps
}

// objectRow is one cached point lookup: what the object-namespace seams know
// about one referenced id.
type objectRow struct {
	name   string
	exists bool
	// deleted marks a TOMBSTONE: the row survives stripped to its
	// bookkeeping. It still counts as exists — see ObjectExists — and only
	// the icon rule asks this narrower question.
	deleted bool
}

// New creates resolvers over one space's index.
func New(index spaceindex.Store) *Resolvers {
	return &Resolvers{
		index:            index,
		optionsFor:       map[domain.RelationKey][]*model.RelationOption{},
		participantNames: map[string]string{},
		objectRows:       map[string]objectRow{},
	}
}

// Options returns anyblockjson.Options pre-wired with the resolvers and the
// index's own space id (which enables the participant fold, §9); callers set
// the remaining fields (compaction flags, RefNames, etc.) on the returned
// value.
func (r *Resolvers) Options() anyblockjson.Options {
	return anyblockjson.Options{
		ResolveFormat:       r.ResolveFormat,
		ResolveOptions:      r,
		ResolveProperties:   r,
		ResolveParticipants: r,
		ResolveObjectNames:  r,
		SpaceId:             r.index.SpaceId(),
		Keys:                r,
	}
}

// loadRelations snapshots the space's relation objects once: the point
// lookups (GetRelationByKey) miss for some legacy relations that the full
// listing still returns, so the map is the primary source and the point
// lookups are the fallback.
func (r *Resolvers) loadRelations() {
	if r.relsLoaded {
		return
	}
	r.relsLoaded = true
	r.relById = map[string]anyblockjson.PropertyDefinition{}
	r.relKeyToId = map[string]string{}
	rels, err := r.index.ListAllRelations()
	if err != nil {
		return
	}
	for _, rel := range rels {
		if rel == nil || rel.Relation == nil {
			continue
		}
		def := anyblockjson.PropertyDefinition{
			Key:         domain.RelationKey(rel.Key),
			Name:        rel.Name,
			Format:      rel.Format,
			ObjectTypes: r.targetTypeKeys(rel.ObjectTypes),
		}
		r.relById[rel.Id] = def
		if _, taken := r.relKeyToId[rel.Key]; !taken {
			r.relKeyToId[rel.Key] = rel.Id
		}
	}
}

// targetTypeKeys turns a relation's stored `relationFormatObjectTypes` into
// the third type slot of §2a — `type_properties[].object_types`.
//
// It is a translation, not a copy: the store holds the target types' OBJECT
// IDs (objectcreator.fillRelationFormatObjectTypes rewrites bundled urls to
// derived ids at creation), while PropertyDefinition.ObjectTypes is defined in
// stored type KEYS, which the codec then spells at the document boundary. Left
// empty — as it was — a node export of a type document silently dropped every
// property's target types: the slot had no node-backed emitter at all, so an
// `objects` property came back untargeted and would accept any object.
//
// The mapping rides the one bounded type listing the vocabulary already pays
// for (§3 budgets one query per kind per resolver, never one per
// reference), plus the bundled-url arm for ids that were never rewritten. An
// id that resolves to nothing is DROPPED, which is the policy export already
// applies to a recommended-list entry that no longer resolves (§2a) — a
// dangling target is not a type key, and writing one would export a document
// naming a type no reader can find.
func (r *Resolvers) targetTypeKeys(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	var out []string
	for _, id := range ids {
		if key, err := bundle.TypeKeyFromUrl(id); err == nil && key != "" {
			out = append(out, string(key))
			continue
		}
		if key := r.typeKeyMaps().keyById[id]; key != "" {
			out = append(out, key)
		}
	}
	return out
}

// ResolveFormat implements anyblockjson.FormatResolver.
func (r *Resolvers) ResolveFormat(key domain.RelationKey) (model.RelationFormat, bool) {
	rel, err := r.index.GetRelationByKey(string(key))
	if err != nil || rel == nil {
		return 0, false
	}
	return rel.Format, true
}

func (r *Resolvers) options(key domain.RelationKey) []*model.RelationOption {
	if cached, ok := r.optionsFor[key]; ok {
		return cached
	}
	opts, err := r.index.ListRelationOptions(key)
	if err != nil {
		opts = nil
	}
	r.optionsFor[key] = opts
	return opts
}

// OptionName implements anyblockjson.OptionResolver.
func (r *Resolvers) OptionName(key domain.RelationKey, id string) (string, bool) {
	for _, o := range r.options(key) {
		if o.Id == id {
			return o.Text, true
		}
	}
	return "", false
}

// OptionId implements anyblockjson.OptionResolver.
func (r *Resolvers) OptionId(key domain.RelationKey, name string) (string, bool) {
	for _, o := range r.options(key) {
		if o.Text == name {
			return o.Id, true
		}
	}
	return "", false
}

// OptionsNamed returns every live option whose display name is exactly name,
// in the store's stable listing order. Write surfaces use this richer answer
// before OptionId: duplicate labels are legal data, but a bare label is not
// an unambiguous address and must never silently select the first row.
func (r *Resolvers) OptionsNamed(key domain.RelationKey, name string) []*model.RelationOption {
	var matches []*model.RelationOption
	for _, option := range r.options(key) {
		if option.Text == name {
			matches = append(matches, option)
		}
	}
	return matches
}

// ParticipantName implements anyblockjson.ParticipantResolver: the display
// name of the space member a participant id names (§3).
//
// A participant is an ordinary indexed object whose id is
// `_participant_<space>_<account>`, so one point lookup answers it — there is
// no listing to load and no vocabulary to prime. The answer is the `name` the
// space last saw on that member's profile.
//
// The lookup is by id and asks nothing about layout, which is what makes it
// answer for the one attribution value that is NOT a participant:
// `_anytype_profile`, the app itself, stands in `creator` on 7.9% of a
// 36,966-object corpus (bundled types and relations copied into a space) and
// is indexed per space by `reindexIDs`. It resolves to "Anytype", which is
// the true answer to who wrote those objects.
//
// **No name is an answer of "no", not an empty string.** A member who never
// set a profile name has none here, and so does an id this space has no
// participant row for (a member of a space this export is not running in, an
// account whose participant object was never indexed). Both make export omit
// the property, which is the same thing it does with no resolver at all —
// the format's rule is that `creator` is a name or is absent, never a blank.
//
// **Only `name`, deliberately.** A member may also carry `globalName` (their
// any-name) and `identity`; neither is substituted for a missing `name`,
// because a document that falls back to an address has re-introduced the
// address this spelling exists to remove.
func (r *Resolvers) ParticipantName(id string) (string, bool) {
	if name, cached := r.participantNames[id]; cached {
		return name, name != ""
	}
	name := ""
	if details, err := r.index.GetDetails(id); err == nil && details != nil {
		name = details.GetString(bundle.RelationKeyName)
	}
	r.participantNames[id] = name
	return name, name != ""
}

// objectRow answers one referenced id's row from the cache or one GetDetails.
// ok is false only when the store errored: nothing is cached then and the
// caller answers as if it had not been asked.
//
// GetDetails is what makes the existence answer precise: it returns an EMPTY
// details struct for an id the index has no row for (spaceindex/objects.go),
// while a stored row always carries at least its own id — so emptiness IS
// the absence signal, and it is available on the very lookup the name
// already pays for. A deleted object's tombstone row ({id, isDeleted}) is a
// row: the id still means something in this space, and the missing-reference
// rule (§9) leaves references to it alone.
func (r *Resolvers) objectRow(id string) (objectRow, bool) {
	if row, cached := r.objectRows[id]; cached {
		return row, true
	}
	details, err := r.index.GetDetails(id)
	if err != nil {
		return objectRow{}, false
	}
	row := objectRow{}
	if details != nil && details.Len() > 0 {
		row.exists = true
		row.name = details.GetString(bundle.RelationKeyName)
		row.deleted = details.GetBool(bundle.RelationKeyIsDeleted)
	}
	r.objectRows[id] = row
	return row, true
}

// ObjectName implements anyblockjson.ObjectNameResolver: the display name of
// the object a reference points at, for the informative `#name` suffix (§9).
//
// The same one-point-lookup shape as ParticipantName, deliberately kept
// separate from it: this one answers for EVERY indexed object — pages,
// files, types, participants alike — because the suffix rides any object
// reference, while the participant seam serves exactly two derived
// properties. No name is an answer of "no", never an empty string: the
// format's rule for the suffix is a name or nothing (a bare reference),
// never a dangling `#`.
func (r *Resolvers) ObjectName(id string) (string, bool) {
	row, ok := r.objectRow(id)
	return row.name, ok && row.name != ""
}

// ObjectExists implements anyblockjson.ObjectExistenceResolver: whether the
// space's index holds a row for id at all — the question behind the
// missing-reference rule (§9), riding the same cached point lookup
// ObjectName pays for.
//
// It exists because ObjectName cannot answer it: that seam's ok is
// `name != ""`, so it says "no" for an object that exists UNTITLED — and
// untitled objects are common. known is false only when the store errored,
// and the codec then leaves the reference untouched: a claim of absence has
// to come from the store actually answering, never from a failure to ask.
func (r *Resolvers) ObjectExists(id string) (exists, known bool) {
	row, ok := r.objectRow(id)
	return row.exists, ok
}

// ObjectDeleted implements anyblockjson.ObjectDeletionResolver: whether the
// row the space kept for id is a TOMBSTONE — deleted, stripped to its
// bookkeeping. It rides the same cached point lookup ObjectExists pays for,
// and is a strictly narrower question: every deleted id also exists.
//
// Only the icon rule consults it (§2b). A deleted object stays a live
// reference everywhere else, which is ObjectExists's documented position.
func (r *Resolvers) ObjectDeleted(id string) (deleted, known bool) {
	row, ok := r.objectRow(id)
	return row.deleted, ok
}

// PropertyById implements anyblockjson.PropertyResolver.
func (r *Resolvers) PropertyById(id string) (anyblockjson.PropertyDefinition, bool) {
	r.loadRelations()
	if def, ok := r.relById[id]; ok {
		return def, true
	}
	rel, err := r.index.GetRelationById(id)
	if err != nil || rel == nil {
		return anyblockjson.PropertyDefinition{}, false
	}
	def := anyblockjson.PropertyDefinition{
		Key:         domain.RelationKey(rel.Key),
		Name:        rel.Name,
		Format:      rel.Format,
		ObjectTypes: r.targetTypeKeys(rel.ObjectTypes),
	}
	// cache the point-lookup hit both ways: some relations resolve by id but
	// are absent from the listing AND the by-key lookup (deleted or index
	// gap — anomaly #9 class), so without this PropertyId cannot invert the
	// key export just produced and the entry is dropped on re-export
	// (resolvers must be equivalent both directions, SPEC §2a/§13)
	r.relById[id] = def
	if _, taken := r.relKeyToId[rel.Key]; !taken {
		r.relKeyToId[rel.Key] = id
	}
	return def, true
}

// SeedProperty registers a definition for an id the index cannot currently
// answer for. The one caller class is the API v2 wiring's tombstone window: a
// deleted relation's index row is stripped to {id, isDeleted} until the next
// space load, so GetRelationById fails on an id the surviving TREE still
// fully describes — the wiring reads the live object and seeds what the
// index will hold again after reindex. Seeds never override a loaded row
// (the by-key map keeps its first binding), matching the cache discipline of
// PropertyById's point-lookup arm.
func (r *Resolvers) SeedProperty(id string, def anyblockjson.PropertyDefinition) {
	r.loadRelations()
	if _, ok := r.relById[id]; ok {
		return
	}
	r.relById[id] = def
	if _, taken := r.relKeyToId[string(def.Key)]; !taken {
		r.relKeyToId[string(def.Key)] = id
	}
}

// PropertyId implements anyblockjson.PropertyResolver.
func (r *Resolvers) PropertyId(def anyblockjson.PropertyDefinition) (string, bool) {
	r.loadRelations()
	if id, ok := r.relKeyToId[string(def.Key)]; ok {
		return id, true
	}
	rel, err := r.index.GetRelationByKey(string(def.Key))
	if err != nil || rel == nil {
		return "", false
	}
	r.relKeyToId[rel.Key] = rel.Id
	return rel.Id, true
}
