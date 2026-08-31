// Package compose is the bundle-level composition of an AnyBlock JSON
// export: everything above one document that a bundle must state —
// properties.json, index.json with its manifest, and the omission-and-lift
// of the documents those two files carry INSTEAD of (SPEC.md §2c, §2f).
//
// It exists because composition is a bundle-level act the one-document codec
// deliberately does not own (SPEC.md §13 gives it this named home), and
// because two independent writers need the SAME implementation: the
// production exporter (core/block/export/anyblock) and the cmd tools
// (cmd/anyblockroundtrip's corpus sweep, which is what makes the sweep an
// end-to-end test of production composition rather than of a private copy).
//
// The shape follows the exporter design (EXPORTER_DESIGN.md §1.1/§1.5):
// BuildPlan runs single-threaded over details-level facts before the first
// emit; the Composer's Observe* methods are safe for concurrent emit tasks
// and accumulate only commutative aggregates under one mutex; Finish sorts
// everything it writes and re-reads both files through the package's own
// Unmarshal before handing them back — the bundle-level I1 discipline, so a
// bundle this code writes that the package refuses is found at export time.
package compose

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/snapshotdiff"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// Issue is one bundle-level finding — an omitted document whose lift or
// reconstruction does not account for everything it held. The exporter logs
// these (an omission that loses data is a bug here, not a reason to fail a
// user's export); the round-trip harness counts them as failures.
type Issue struct {
	Category string
	Detail   string
}

// Stats is what Finish can say about the composed bundle, for summaries.
type Stats struct {
	DictionaryInstalled int
	DictionaryEntries   int
	ManifestTypes       int
	ManifestFiles       int
	OptionDocs          int
	DictionaryBytes     int
	IndexBytes          int
	OmittedDocs         int
	// OrphanUsedKeys are referenced property keys with no definition
	// anywhere — no relation object, not bundled — so the dictionary cannot
	// state a format for them (§2f names every property it CAN).
	OrphanUsedKeys []string
}

// Composer accumulates, across one bundle's emit, everything the two
// bundle-level files state: which bundled relations are installed (and which
// of their documents the emit omitted), the definitions the dictionary
// carries, the option vocabularies, the index lift from the omitted
// space-settings and widget documents, and where the manifest finds each
// type and each file blob.
//
// Observe, ObserveWritten and ObserveFileBlob are safe for concurrent use —
// the emit phase runs width-bounded tasks (design §1.5) and everything
// shared here is commutative map/set insertion under one mutex, held for
// microseconds against marshal work measured in milliseconds. Finish is
// called once, after every emit task returned.
type Composer struct {
	mu sync.Mutex

	opts      anyblockjson.Options
	spaceName string

	installed map[string]bool
	// entries the space's own documents define: a KEPT bundled-key relation
	// document (divergent from the table, or carrying something only a
	// document can) contributes its stored definition, so the dictionary
	// states the divergence the `installed` list alone would paper over
	entries map[string]anyblockjson.PropertyDefinition

	typePaths   map[string]string
	filePaths   map[string]string
	optionPaths map[string]string
	// optionsByKey is the select vocabulary each property actually has in
	// this space, gathered from the option documents so the dictionary can
	// state it inline (§2f). Keyed by STORED property key, and held with the
	// stored `orderId` so the inline array can be written in the order the
	// space actually shows.
	optionsByKey map[string][]storedOption

	// used is the referenced-key census the dictionary's used-only rule
	// needs (§2f), gathered from each document's marshalled bytes as it is
	// observed. From the BYTES, not a re-read: a zip export cannot re-read
	// its own entries before Close, so the scan runs before the write —
	// which is also what lets the cmd tools and production share it
	// (UsedPropertyKeysFromBytes, design §1.1).
	used map[string]bool

	written int
	omitted int

	// the fields the space's own document and the widget object are the
	// sources of (§2c). Lifted as each document is observed and omitted, so
	// the index states what the dropped documents held.
	index anyblockjson.Index
}

// NewComposer creates a composer for one bundle. opts is consulted from
// inside the composer's own mutex only, so a store-backed resolver that is
// not safe for concurrent use (storeresolver.Resolvers) is fine HERE — but
// it must then be a dedicated instance, not one an emit worker also uses.
// spaceName is the fallback for a space whose own document states no name.
func NewComposer(opts anyblockjson.Options, spaceName string) *Composer {
	return &Composer{
		opts:         opts,
		spaceName:    spaceName,
		installed:    map[string]bool{},
		entries:      map[string]anyblockjson.PropertyDefinition{},
		typePaths:    map[string]string{},
		filePaths:    map[string]string{},
		optionPaths:  map[string]string{},
		optionsByKey: map[string][]storedOption{},
		used:         map[string]bool{},
	}
}

// Observe classifies one snapshot for the composition. For an omitted
// document it also verifies the trip the object takes INSTEAD of a document
// — the index lift, or installed key → the reader's bundled table — through
// the same comparator as every ordinary round trip, so the omission
// predicate and the reconstruction cannot drift apart silently.
//
// The caller emits the document iff omitted is false; issues are reported
// either way (an issue on an omitted document means the lift lost
// something, which the §1.7 contract forbids).
func (c *Composer) Observe(sbType model.SmartBlockType, base *model.SmartBlockSnapshotBase) (omitted bool, issues []Issue) {
	c.mu.Lock()
	defer c.mu.Unlock()
	omitted, issues = c.observe(sbType, base)
	if omitted {
		c.omitted++
	}
	return omitted, issues
}

func (c *Composer) observe(sbType model.SmartBlockType, base *model.SmartBlockSnapshotBase) (bool, []Issue) {
	if base == nil {
		return false, nil
	}
	// the space's own object: index.json states everything it holds (§2c),
	// so the composer lifts those fields and drops the document. The lift
	// runs BEFORE the omission is recorded, so a bundle can never drop the
	// document without having written what it carried.
	if anyblockjson.OmittedSpaceSettings(sbType, base) {
		anyblockjson.IndexFromSpaceSettings(&c.index, base)
		return true, nil
	}
	// the deprecated per-space profile object: superseded by `participant`,
	// and what survives in a real account is an empty hidden object carrying
	// someone else's name, dragged in by an import (§2c)
	if anyblockjson.OmittedProfilePage(sbType, base) {
		return true, nil
	}
	// the sidebar's object: index.json states everything it holds (§2c) —
	// the wrapper-and-link pairs flat in `widgets`, the auto-widget ledger
	// at index level — so the composer lifts those fields and drops the
	// document, the space-settings rule again. The lift runs BEFORE the
	// omission is recorded, and the snapshot a bundle carries INSTEAD
	// (WidgetsSnapshot, the same function cmd/anyblockconvert installs
	// from) is verified against the original through the same comparator as
	// every ordinary round trip, so the lift and the rebuild cannot drift
	// apart silently. A nil snapshot means the index carries no sidebar
	// state because the object held none — the predicate is the proof.
	if anyblockjson.OmittedWidgetObject(sbType, base) {
		anyblockjson.IndexFromWidgetObject(&c.index, base)
		rebuilt, err := anyblockjson.WidgetsSnapshot(&c.index)
		if err != nil {
			return true, []Issue{{Category: "omitted_reconstruction",
				Detail: fmt.Sprintf("widget object: %v", err)}}
		}
		var issues []Issue
		if rebuilt != nil {
			for _, d := range snapshotdiff.Compare(base, rebuilt, sbType, c.opts) {
				issues = append(issues, Issue{Category: "omitted_reconstruction", Detail: d})
			}
		}
		return true, issues
	}
	if key, ok := anyblockjson.OmittedBundledRelation(sbType, base, c.opts); ok {
		c.installed[key] = true
		det, ok := anyblockjson.InstalledRelationDetails(key, c.opts)
		if !ok {
			return true, []Issue{{Category: "omitted_reconstruction",
				Detail: fmt.Sprintf("installed key %q has no bundled reconstruction", key)}}
		}
		var issues []Issue
		got := &model.SmartBlockSnapshotBase{Details: det, ObjectTypes: base.ObjectTypes}
		for _, d := range snapshotdiff.Compare(base, got, sbType, c.opts) {
			issues = append(issues, Issue{Category: "omitted_reconstruction", Detail: d})
		}
		return true, issues
	}
	if det := base.GetDetails().GetFields(); det != nil &&
		(sbType == model.SmartBlockType_STRelation || sbType == model.SmartBlockType_BundledRelation) {
		key := det["relationKey"].GetStringValue()
		if key != "" && bundle.HasRelation(domain.RelationKey(key)) {
			// installed but not omittable: the document stays, and the
			// dictionary carries its stored definition as the full entry
			// the §2f divergence rule requires
			c.installed[key] = true
			c.entries[key] = storedRelationDefinition(base, c.opts)
		}
	}
	return false, nil
}

// ObserveWritten records one emitted document: its place for the manifest —
// a type by its STORED key, a file blob binding is ObserveFileBlob's — the
// option vocabulary an option document contributes, and the property keys
// the document's bytes reference (the dictionary's used-only census, §2f).
// path is the document's bundle-relative path, slash-separated, exactly as
// it should appear in the manifest.
func (c *Composer) ObserveWritten(sbType model.SmartBlockType, base *model.SmartBlockSnapshotBase, doc []byte, path string) error {
	used, err := UsedPropertyKeysFromBytes(doc)
	if err != nil {
		return fmt.Errorf("scan used property keys: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.written++
	for key := range used {
		c.used[key] = true
	}
	det := base.GetDetails().GetFields()
	if det == nil {
		return nil
	}
	switch sbType {
	case model.SmartBlockType_STType, model.SmartBlockType_BundledObjectType:
		if key := strings.TrimPrefix(det["uniqueKey"].GetStringValue(), "ot-"); key != "" {
			c.typePaths[key] = path
		}
	case model.SmartBlockType_STRelationOption:
		// an option's whole meaning is three details — which property it
		// belongs to, its name, and its colour — wrapped in a document whose
		// remaining forty lines are derived scaffolding. The dictionary
		// states those three inline, so a bundle declares a select vocabulary
		// in the same place it declares the property (§2f).
		if key := det["relationKey"].GetStringValue(); key != "" {
			if name := det["name"].GetStringValue(); name != "" {
				c.optionsByKey[key] = append(c.optionsByKey[key], storedOption{
					order: det["orderId"].GetStringValue(),
					id:    det["id"].GetStringValue(),
					def: anyblockjson.OptionDefinition{
						Name:  name,
						Color: det["relationOptionColor"].GetStringValue(),
						// the option's stored key: minted, so derivable from
						// nothing, unlike its name, colour, position and api
						// key (§2f). Carried by uniqueKey `opt-<key>`.
						InternalKey: strings.TrimPrefix(
							det["uniqueKey"].GetStringValue(), "opt-"),
					},
				})
			}
		}
		if id := det["id"].GetStringValue(); id != "" {
			c.optionPaths[id] = path
		}
	}
	return nil
}

// ObserveFileBlob records one written blob for the manifest `files` map
// (§2c): the file object's id → the blob's bundle-relative path.
// Called only after the bytes are actually written, so the manifest never
// points at a blob a failed stream left absent.
func (c *Composer) ObserveFileBlob(objectId, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.filePaths[objectId] = path
}

// Finish composes the bundle's two files and re-reads both through the
// package's own Unmarshal — the bundle-level twin of the I1 discipline: a
// file this composer writes that the package refuses is a bug here, found
// at export time rather than at restore time. Both byte slices are nil when
// nothing was written (an empty bundle states nothing).
func (c *Composer) Finish() (index, properties []byte, stats Stats, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	stats.OmittedDocs = c.omitted
	if c.written == 0 {
		return nil, nil, stats, nil
	}

	// the dictionary names every property the documents actually reference
	// (§2f, used-only): the space's own definitions first (divergent
	// installed copies, space-minted relation documents keep their files but
	// the dictionary still answers for every USED key), then the resolver,
	// then the bundled table. A key none of them can define — an orphan
	// detail no relation object describes — is reported, not invented.
	entries := map[string]anyblockjson.PropertyDefinition{}
	for key, def := range c.entries {
		if c.used[key] {
			entries[key] = def
		} else if _, installedToo := c.installed[key]; installedToo {
			// a divergent installed copy is an entry whether or not
			// anything uses it: `installed` would otherwise restore the
			// table's shape over the divergence
			entries[key] = def
		}
	}
	var orphans []string
	for key := range c.used {
		if _, have := entries[key]; have {
			continue
		}
		if def, ok := resolvedDefinition(key, c.opts); ok {
			entries[key] = def
			continue
		}
		if rel, relErr := bundle.GetRelation(domain.RelationKey(key)); relErr == nil {
			entries[key] = anyblockjson.PropertyDefinition{
				Key: domain.RelationKey(key), Name: rel.Name, Format: rel.Format,
				ObjectTypes: bundledTargetKeys(rel.ObjectTypes),
			}
			continue
		}
		orphans = append(orphans, key)
	}
	sort.Strings(orphans)

	// the select vocabulary travels with the property that owns it. A
	// property whose options a space minted has no entry otherwise — it is an
	// ordinary installed bundled key — and its vocabulary would exist only in
	// the option documents, where an author generating a bundle has to know
	// to look for it.
	for key, stored := range c.optionsByKey {
		if !c.used[key] {
			continue // §2f is used-only: an unused property's vocabulary buys a reader nothing
		}
		// in the order the SPACE shows them, which the stored `orderId`
		// carries: `status` really reads To Do → In Progress → Done, and
		// sorting by name turned that workflow into Done → In Progress →
		// To Do on 42 of the 61 vocabularies that state an order.
		//
		// An option with no orderId sorts AFTER the ordered ones, by name,
		// and that is not a compromise — it is the app's own model. Ordering
		// is a newer feature than options: 229 of 312 vocabularies state no
		// order at all and 21 state one for only some members, and the app's
		// own placement query (objectcreator/relation_option.go) filters
		// `orderId NotEmpty`, so an option without one is not in the app's
		// ordering either. There is no order to lose; name is what makes the
		// canonical form deterministic.
		sort.SliceStable(stored, func(i, j int) bool {
			a, b := stored[i], stored[j]
			if (a.order == "") != (b.order == "") {
				return a.order != ""
			}
			if a.order != b.order {
				return a.order < b.order
			}
			if a.def.Name != b.def.Name {
				return a.def.Name < b.def.Name
			}
			// the total-order tie-break (see storedOption.id): without it a
			// name shared by two options left the pair in insertion order,
			// which the concurrent emit does not fix
			return a.id < b.id
		})
		opts := make([]anyblockjson.OptionDefinition, 0, len(stored))
		for _, so := range stored {
			opts = append(opts, so.def)
		}
		def, have := entries[key]
		if !have {
			if resolved, ok := resolvedDefinition(key, c.opts); ok {
				def = resolved
			} else if rel, relErr := bundle.GetRelation(domain.RelationKey(key)); relErr == nil {
				def = anyblockjson.PropertyDefinition{
					Key: domain.RelationKey(key), Name: rel.Name, Format: rel.Format,
					ObjectTypes: bundledTargetKeys(rel.ObjectTypes),
				}
			} else {
				continue // nothing can say what this property is; §2f reports it as an orphan
			}
		}
		def.Options = opts
		entries[key] = def
	}

	dict := &anyblockjson.PropertyDictionary{}
	for key := range c.installed {
		dict.Installed = append(dict.Installed, key)
	}
	for _, key := range sortedEntryKeys(entries) {
		dict.Properties = append(dict.Properties, entries[key])
	}
	dictData, err := anyblockjson.MarshalPropertyDictionary(dict)
	if err != nil {
		return nil, nil, stats, fmt.Errorf("marshal property dictionary: %w", err)
	}
	if _, err := anyblockjson.UnmarshalPropertyDictionary(dictData); err != nil {
		return nil, nil, stats, fmt.Errorf("re-read property dictionary: %w", err)
	}

	// start from what the space's own document was lifted into (§2c) rather
	// than copying its fields across by hand: the hand-written version listed
	// three, and silently dropped the space ICON the moment the lift learned
	// to carry one. Whatever IndexFromSpaceSettings writes now travels
	// without this function being told about it.
	idx := c.index
	// the caller's name is the fallback for a space whose document has none
	if idx.Name == "" {
		idx.Name = c.spaceName
	}
	idx.Manifest = &anyblockjson.Manifest{
		Types:      copyNonEmpty(c.typePaths),
		Properties: anyblockjson.PropertiesFileName,
		Files:      copyNonEmpty(c.filePaths),
	}
	idxData, err := anyblockjson.MarshalIndex(&idx)
	if err != nil {
		return nil, nil, stats, fmt.Errorf("marshal index: %w", err)
	}
	if _, err := anyblockjson.UnmarshalIndex(idxData); err != nil {
		return nil, nil, stats, fmt.Errorf("re-read index: %w", err)
	}

	stats.DictionaryInstalled = len(dict.Installed)
	stats.DictionaryEntries = len(dict.Properties)
	stats.ManifestTypes = len(c.typePaths)
	stats.ManifestFiles = len(c.filePaths)
	stats.OptionDocs = len(c.optionPaths)
	stats.DictionaryBytes = len(dictData)
	stats.IndexBytes = len(idxData)
	stats.OrphanUsedKeys = orphans
	return idxData, dictData, stats, nil
}

// storedOption is one option document's contribution to the inline
// vocabulary: the definition the dictionary states, plus the stored `orderId`
// that decides where it sits. The orderId itself never reaches a document —
// it is a lexid, which is exactly the spelling this format keeps out of an
// author's way; the ARRAY POSITION is what carries the order.
type storedOption struct {
	order string
	// id is the option document's own id — the total-order tie-break. Two
	// options of one property may legitimately share a name (and even a
	// colour), and (order, name) alone is then not a total order: the tie
	// fell back to insertion order, which under the concurrent emit is
	// scheduling order, and the corpus sweep caught two exports of one
	// space disagreeing about which colour sat at which position. The id is
	// the one member that cannot tie.
	id  string
	def anyblockjson.OptionDefinition
}

// storedRelationDefinition reads the definition a kept relation document
// states, off its stored details — the §2f full entry for a divergent
// installed copy. Members mirror what the document itself would carry.
func storedRelationDefinition(base *model.SmartBlockSnapshotBase, opts anyblockjson.Options) anyblockjson.PropertyDefinition {
	det := base.GetDetails().GetFields()
	def := anyblockjson.PropertyDefinition{
		Key:         domain.RelationKey(det["relationKey"].GetStringValue()),
		Name:        det["name"].GetStringValue(),
		Format:      model.RelationFormat(int32(det["relationFormat"].GetNumberValue())),
		Description: det["description"].GetStringValue(),
		MaxCount:    int64(det["relationMaxCount"].GetNumberValue()),
		Readonly:    det["relationReadonlyValue"].GetBoolValue(),
	}
	if v := det["relationFormatIncludeTime"]; v != nil {
		if _, isBool := v.GetKind().(*types.Value_BoolValue); isBool {
			b := v.GetBoolValue()
			def.IncludeTime = &b
		}
	}
	if v := det["relationDefaultValue"]; v != nil {
		if _, isNull := v.GetKind().(*types.Value_NullValue); !isNull {
			def.DefaultValue = pbtypes.ValueToInterface(v)
		}
	}
	if v := det["relationFormatObjectTypes"]; v != nil {
		tr, _ := opts.ResolveProperties.(anyblockjson.TypeResolver)
		for _, entry := range pbtypes.GetStringListValue(v) {
			if key, err := bundle.TypeKeyFromUrl(entry); err == nil {
				def.ObjectTypes = append(def.ObjectTypes, string(key))
				continue
			}
			if tr != nil {
				if key, ok := tr.TypeKeyById(entry); ok && key != "" {
					def.ObjectTypes = append(def.ObjectTypes, key)
					continue
				}
			}
			def.ObjectTypes = append(def.ObjectTypes, entry)
		}
	}
	return def
}

// resolvedDefinition asks the space's resolver for a used key's definition —
// the storeresolver path a live export runs on.
func resolvedDefinition(key string, opts anyblockjson.Options) (anyblockjson.PropertyDefinition, bool) {
	r := opts.ResolveProperties
	if r == nil {
		return anyblockjson.PropertyDefinition{}, false
	}
	if id, ok := r.PropertyId(anyblockjson.PropertyDefinition{Key: domain.RelationKey(key)}); ok {
		if def, ok := r.PropertyById(id); ok {
			return def, true
		}
	}
	return anyblockjson.PropertyDefinition{}, false
}

// bundledTargetKeys turns the bundled table's target urls into type keys.
func bundledTargetKeys(urls []string) []string {
	var out []string
	for _, u := range urls {
		if k, err := bundle.TypeKeyFromUrl(u); err == nil {
			out = append(out, string(k))
		}
	}
	return out
}

// sortedEntryKeys lists a map's keys in order — the canonical entry order.
func sortedEntryKeys(m map[string]anyblockjson.PropertyDefinition) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// copyNonEmpty snapshots a path map, nil when there is nothing to state —
// the §4 omit-empty canon for the manifest's tables.
func copyNonEmpty(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
