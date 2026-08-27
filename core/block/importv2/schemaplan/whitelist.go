package schemaplan

// This file is the whitelist property mapper (docs/superpowers/specs/
// 2026-08-07-importv2-whitelist-planner-design.md §4): the LLM only groups
// containers into kinds and names them; CompleteKinds turns those kinds into a
// full Plan in pure code — bundled targets via the closed rules table,
// kind-local (name, format) keys for everything else, merge guards vetoing
// unsound kinds and shares. Output flows through Sanitize like any plan, so a
// bug here degrades per-entry with a warning, same as a model error would.

import (
	"sort"
	"strconv"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/importv2/typesuggest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// KindPlan is one grouped-and-named kind, the planner's whole verdict for it.
type KindPlan struct {
	Name          string
	PluralName    string
	IconName      string
	Layout        model.ObjectTypeLayout
	ContainerIds  []string // resolved from ordinals; all present in the evidence
	FeaturedNames []string // source property names; exact-match or dropped
}

// coverageGateThreshold is merge guard 1: every member of a
// multi-container kind must cover at least this share of the kind's merged
// (lowercased name, format) property union, or the kind is split. Calibrated
// on a real 37-container workspace: duplicated databases score 1.00, the
// LLM's own bad three-tracker merge scored 0.14 and is vetoed.
const coverageGateThreshold = 0.5

// dueTokens are the word tokens a date property's normalized name must carry
// to hit the bundled dueDate target. Token match, not substring, keeps
// "Overdue" out; the token rule is new semantics deliberately NOT shared with
// typesuggest's dueNames (which are exact full-name matches for the type
// verdict) — see the rules table for the measured 0-false-positive run
// over 27 real date property names.
var dueTokens = map[string]bool{"due": true, "deadline": true}

// tagSkipNames are the exact property names the shipped Notion tag redirect
// (notion/properties.go isTagRedirect) owns. CompleteKinds creates no plan
// entry for a list-format property with one of these names, so it stays
// unplanned and reaches the redirect — space-wide sharing is the entire point
// of tags, and the redirect already implements the global
// "Tags-only-when-no-Tag-exists" latch. Markdown's schema-less front-matter
// path likewise keeps its own shipped handling for unplanned properties.
var tagSkipNames = map[string]bool{"Tag": true, "Tags": true, "tags": true}

// CompleteKinds derives the full plan from the model's kind grouping: type
// keys from kind names, bundled targets from the whitelist rules, kind-local
// shared relations from (name, format) identity, featured properties by exact
// name match. Containers absent from every kind fall back to their
// typesuggest verdict with bundled-only mappings. Deterministic for identical
// input, and its output sanitizes with zero property-entry drops by
// construction.
func CompleteKinds(kinds []KindPlan, schemas []ContainerSchema) Plan {
	schemaById := make(map[string]ContainerSchema, len(schemas))
	for _, schema := range schemas {
		schemaById[schema.Id] = schema
	}

	kinds = dedupeKindMembers(kinds, schemaById)
	kinds = applyCoverageGate(kinds, schemaById)

	plan := Plan{}
	assigned := map[string]bool{}
	usedKeys := map[domain.TypeKey]bool{}
	for _, kind := range kinds {
		def, containers := completeKind(kind, schemaById, usedKeys)
		plan.NewTypes = append(plan.NewTypes, def)
		for containerId, containerPlan := range containers {
			if plan.Containers == nil {
				plan.Containers = map[string]ContainerPlan{}
			}
			plan.Containers[containerId] = containerPlan
			assigned[containerId] = true
		}
	}

	// Containers the model left unassigned degrade to today's behaviour per
	// container: the typesuggest verdict plus bundled whitelist entries only.
	// NEVER kind-local keys here — naive verdicts are bundled type keys, and
	// scopeMap scopes by type key for bundled types too, so kind-local keys
	// would let two unrelated databases both typed `task` share every
	// same-named select: the historical four-databases-one-"Category" defect
	// re-entering through the fallback door. Bundled targets are safe
	// everywhere — they are the whitelisted sharing exception by definition.
	suggestor := typesuggest.NewNaive()
	for _, schema := range schemas {
		if assigned[schema.Id] {
			continue
		}
		containerPlan := ContainerPlan{}
		evidence := typesuggest.Evidence{ContainerName: schema.Name}
		for _, property := range schema.Properties {
			evidence.Properties = append(evidence.Properties, typesuggest.Property{
				Name:   property.Name,
				Format: property.Format,
			})
		}
		if suggestion, ok := suggestor.Suggest(evidence); ok {
			containerPlan.TypeKey = suggestion.TypeKey
			containerPlan.Reason = suggestion.Reason
		}
		for propertyId, target := range bundledTargets(schema) {
			if containerPlan.Properties == nil {
				containerPlan.Properties = map[string]PropertyPlan{}
			}
			containerPlan.Properties[propertyId] = PropertyPlan{Key: target}
		}
		if containerPlan.TypeKey == "" && len(containerPlan.Properties) == 0 {
			continue
		}
		if plan.Containers == nil {
			plan.Containers = map[string]ContainerPlan{}
		}
		plan.Containers[schema.Id] = containerPlan
	}
	return plan
}

// dedupeKindMembers drops container ids the evidence does not carry and
// resolves a container claimed by two kinds to the first (kind order), then
// drops kinds with no surviving members.
func dedupeKindMembers(kinds []KindPlan, schemaById map[string]ContainerSchema) []KindPlan {
	claimed := map[string]bool{}
	out := make([]KindPlan, 0, len(kinds))
	for _, kind := range kinds {
		var members []string
		for _, containerId := range kind.ContainerIds {
			if _, ok := schemaById[containerId]; !ok || claimed[containerId] {
				continue
			}
			claimed[containerId] = true
			members = append(members, containerId)
		}
		if len(members) == 0 {
			continue
		}
		kind.ContainerIds = members
		out = append(out, kind)
	}
	return out
}

// applyCoverageGate is merge guard 1: a multi-member kind whose union
// of (lowercased name, format) pairs any member covers below the threshold is
// unsound — merging produces a type most of whose relations are permanently
// empty for that member's pages. The kind is split wholesale: each member
// becomes its own single-container kind named from its container's
// id-stripped title, keeping the model's icon and layout. The guard can only
// un-merge — no container, property or page is dropped, so a veto degrades to
// today's per-database behaviour. Always-mint is preserved: a split member
// still gets a minted type.
func applyCoverageGate(kinds []KindPlan, schemaById map[string]ContainerSchema) []KindPlan {
	var out []KindPlan
	for _, kind := range kinds {
		if len(kind.ContainerIds) < 2 || kindCoverageSound(kind, schemaById) {
			out = append(out, kind)
			continue
		}
		for _, containerId := range kind.ContainerIds {
			out = append(out, KindPlan{
				Name:          schemaById[containerId].Name,
				IconName:      kind.IconName,
				Layout:        kind.Layout,
				ContainerIds:  []string{containerId},
				FeaturedNames: kind.FeaturedNames,
			})
		}
	}
	return out
}

func kindCoverageSound(kind KindPlan, schemaById map[string]ContainerSchema) bool {
	union := map[nameFormat]bool{}
	memberPairs := make([]map[nameFormat]bool, 0, len(kind.ContainerIds))
	for _, containerId := range kind.ContainerIds {
		pairs := map[nameFormat]bool{}
		for _, property := range schemaById[containerId].Properties {
			pair := nameFormat{name: strings.ToLower(property.Name), format: property.Format}
			pairs[pair] = true
			union[pair] = true
		}
		memberPairs = append(memberPairs, pairs)
	}
	if len(union) == 0 {
		return true
	}
	for _, pairs := range memberPairs {
		covered := 0
		for pair := range pairs {
			if union[pair] {
				covered++
			}
		}
		if float64(covered) < coverageGateThreshold*float64(len(union)) {
			return false
		}
	}
	return true
}

type nameFormat struct {
	name   string
	format model.RelationFormat
}

// completeKind builds one kind's type definition and its members' container
// plans.
func completeKind(kind KindPlan, schemaById map[string]ContainerSchema,
	usedKeys map[domain.TypeKey]bool) (TypeDefinition, map[string]ContainerPlan) {
	name := cleanDisplayName(kind.Name)
	if name == "" {
		// A nameless kind would be dropped by Sanitize and take its containers'
		// types with it; the first member's own title is the honest fallback.
		name = schemaById[kind.ContainerIds[0]].Name
	}
	typeKey := kindTypeKey(name, schemaById[kind.ContainerIds[0]].Name, usedKeys)

	salted := vocabularyVetoes(kind, schemaById)
	containers := make(map[string]ContainerPlan, len(kind.ContainerIds))
	// One union entry per target key, ordered featured-first then by property
	// name.
	type unionEntry struct {
		property PropertySchema
		plan     PropertyPlan
	}
	union := map[domain.RelationKey]unionEntry{}
	// Guards the recommended list against several keys sharing one label; see
	// the dedup note below. Keyed by (name, format) rather than name alone, so
	// a genuine same-name/different-format pair still lists both — the UI
	// distinguishes those by format, and suppressing one would hide a relation
	// that really is different.
	unionNames := map[nameFormat]bool{}
	for _, containerId := range kind.ContainerIds {
		schema := schemaById[containerId]
		containerPlan := ContainerPlan{TypeKey: typeKey, Reason: "LLM plan"}
		bundled := bundledTargets(schema)
		taken := map[domain.RelationKey]bool{}
		for _, property := range schema.Properties {
			var propertyPlan PropertyPlan
			switch {
			case bundled[property.Id] != "":
				propertyPlan = PropertyPlan{Key: bundled[property.Id]}
			case isTagSkip(property):
				continue // stays unplanned; the shipped tag redirect owns it
			default:
				key := kindLocalKey(property.Name, property.Format)
				if salted[nameFormat{name: property.Name, format: property.Format}] {
					key = saltedKindLocalKey(property.Name, property.Format, containerId)
				}
				propertyPlan = PropertyPlan{
					Key:    key,
					Name:   "",              // keep the user's name (boundedName falls back to source)
					Format: property.Format, // explicit, so the anchor settles to the source format
				}
			}
			if taken[propertyPlan.Key] {
				// Two identically-named same-format properties in one container
				// would collide on one target; the later one imports unplanned
				// (today's behaviour) instead of becoming a sanitizer drop.
				continue
			}
			taken[propertyPlan.Key] = true
			if containerPlan.Properties == nil {
				containerPlan.Properties = map[string]PropertyPlan{}
			}
			containerPlan.Properties[property.Id] = propertyPlan
			if _, ok := union[propertyPlan.Key]; !ok {
				// The type's recommended list is deduped by DISPLAY NAME as
				// well as by key. Salted keys (a vocabulary-vetoed property)
				// and format drift both put several distinct keys behind one
				// name, and listing "Status" three times on one type is the
				// very "every board sprouts the others' empty columns"
				// symptom the merge guards exist to prevent. Only the first
				// member's entry is recommended; the rest stay on their
				// container plans, so no data and no relation is lost — the
				// type just does not advertise duplicates.
				if !unionNames[nameFormat{name: property.Name, format: property.Format}] {
					unionNames[nameFormat{name: property.Name, format: property.Format}] = true
					union[propertyPlan.Key] = unionEntry{property: property, plan: propertyPlan}
				}
			}
		}
		containers[containerId] = containerPlan
	}

	def := TypeDefinition{
		Key:        typeKey,
		Name:       name,
		PluralName: kind.PluralName,
		IconName:   kind.IconName,
		Layout:     kind.Layout,
	}
	featuredKeys := resolveFeatured(kind, schemaById, salted)
	featured := map[domain.RelationKey]bool{}
	for _, key := range featuredKeys {
		if entry, ok := union[key]; ok && !featured[key] {
			featured[key] = true
			def.Properties = append(def.Properties, typeProperty(entry.property, entry.plan, true))
		}
	}
	rest := make([]domain.RelationKey, 0, len(union))
	for key := range union {
		if !featured[key] {
			rest = append(rest, key)
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		left, right := union[rest[i]], union[rest[j]]
		if left.property.Name != right.property.Name {
			return left.property.Name < right.property.Name
		}
		return rest[i] < rest[j]
	})
	for _, key := range rest {
		entry := union[key]
		def.Properties = append(def.Properties, typeProperty(entry.property, entry.plan, false))
	}
	return def, containers
}

func typeProperty(property PropertySchema, plan PropertyPlan, isFeatured bool) TypeProperty {
	if bundle.HasRelation(plan.Key) {
		bundled := bundle.MustGetRelation(plan.Key)
		return TypeProperty{Key: plan.Key, Name: bundled.Name, Format: bundled.Format, Featured: isFeatured}
	}
	// Names are the source property names; the format is the settled one the
	// key embeds, so the type's declared anchor and the containers agree.
	return TypeProperty{Key: plan.Key, Name: property.Name, Format: property.Format, Featured: isFeatured}
}

// bundledTargets is the rules table: property id → bundled relation for
// one container. All rules are format-anchored and carry the "sole match per
// container" guard, so ambiguity degrades to no mapping (today's behaviour,
// zero loss), never to a sanitizer drop.
func bundledTargets(schema ContainerSchema) map[string]domain.RelationKey {
	var emails, phones, dues, dones []string
	for _, property := range schema.Properties {
		switch property.Format {
		case model.RelationFormat_email:
			// The user typed the property as *email* in Notion; the value
			// domain is emails. Format-preserving, name-independent — the real
			// workspace's "Email 📧 " (trailing space) is why this is a format
			// rule, not a name table.
			emails = append(emails, property.Id)
		case model.RelationFormat_phone:
			phones = append(phones, property.Id)
		case model.RelationFormat_date:
			if hasDueToken(property.Name) {
				dues = append(dues, property.Id)
			}
		case model.RelationFormat_checkbox:
			if typesuggest.MappingCompletionNames[typesuggest.Normalize(property.Name)] {
				dones = append(dones, property.Id)
			}
		}
	}
	out := map[string]domain.RelationKey{}
	assignSole(out, emails, bundle.RelationKeyEmail)
	assignSole(out, phones, bundle.RelationKeyPhone)
	assignSole(out, dues, bundle.RelationKeyDueDate)
	assignSole(out, dones, bundle.RelationKeyDone)
	return out
}

func assignSole(out map[string]domain.RelationKey, candidates []string, target domain.RelationKey) {
	if len(candidates) == 1 {
		out[candidates[0]] = target
	}
}

// hasDueToken token-matches a normalized property name: any whole word equal
// to "due" or "deadline". "Bid Due Date" hits; "Overdue" (substring) and the
// 20+ non-due date names of the measured workspace (Created Date, Start Date,
// Timeline, …) do not.
func hasDueToken(name string) bool {
	for _, token := range strings.Fields(typesuggest.Normalize(name)) {
		if dueTokens[token] {
			return true
		}
	}
	return false
}

func isTagSkip(property PropertySchema) bool {
	if property.Format != model.RelationFormat_status && property.Format != model.RelationFormat_tag {
		return false
	}
	return tagSkipNames[property.Name]
}

// vocabularyVetoes is merge guard 2: two members of a surviving kind
// may carry the same name and format but different *meanings*. A
// select/multiSelect (name, format) unifies across members only if every pair
// of member option sets intersects in at least half the smaller set;
// otherwise each member keeps its own relation (kind-local key salted with
// the container id). Measured on the real workspace, 108 of 170
// same-name/same-format select pairs have zero option overlap — the guard has
// real work to do.
func vocabularyVetoes(kind KindPlan, schemaById map[string]ContainerSchema) map[nameFormat]bool {
	options := map[nameFormat][]map[string]bool{}
	for _, containerId := range kind.ContainerIds {
		for _, property := range schemaById[containerId].Properties {
			if property.Format != model.RelationFormat_status && property.Format != model.RelationFormat_tag {
				continue
			}
			if isTagSkip(property) {
				continue
			}
			set := make(map[string]bool, len(property.Options))
			for _, option := range property.Options {
				set[option] = true
			}
			pair := nameFormat{name: property.Name, format: property.Format}
			options[pair] = append(options[pair], set)
		}
	}
	vetoed := map[nameFormat]bool{}
	for pair, sets := range options {
		if len(sets) < 2 {
			continue
		}
		for i := 0; i < len(sets) && !vetoed[pair]; i++ {
			for j := i + 1; j < len(sets); j++ {
				if !optionsAgree(sets[i], sets[j]) {
					vetoed[pair] = true
					break
				}
			}
		}
	}
	return vetoed
}

// optionsAgree reports whether two option vocabularies overlap in at least
// half the smaller set.
func optionsAgree(a, b map[string]bool) bool {
	smaller, larger := a, b
	if len(b) < len(a) {
		smaller, larger = b, a
	}
	if len(smaller) == 0 {
		return false
	}
	overlap := 0
	for option := range smaller {
		if larger[option] {
			overlap++
		}
	}
	return 2*overlap >= len(smaller)
}

// kindLocalKey is the sharing rule: a pure function of (name, format), so
// two containers of one kind carrying a byte-identical property name with the
// same format derive the same plan key; ScopedKey then scopes it by the
// kind's type key and CustomRelationKey mints ONE shared relation — the
// duplicated-database case. A differently-spelled property derives a
// different key and stays separate; two different kinds scope differently and
// can never share. Embedding the format means same-name-different-format
// drift yields two relations instead of a format-anchor drop. The "prop\x00"
// prefix keeps derived keys out of bundle.HasRelation's namespace, and the
// format tail carries no NUL, so the composite parses unambiguously from the
// right; ScopedKey's length-prefix makes the whole injection-safe.
func kindLocalKey(name string, format model.RelationFormat) domain.RelationKey {
	return domain.RelationKey("prop\x00" + name + "\x00" + strconv.Itoa(int(format)))
}

// saltedKindLocalKey keeps a vocabulary-vetoed property container-private.
func saltedKindLocalKey(name string, format model.RelationFormat, containerId string) domain.RelationKey {
	return domain.RelationKey(string(kindLocalKey(name, format)) + "\x00" + containerId)
}

// kindTypeKey slugs the kind's name into the plan-scoped type key
// (normalize → dash-join, "Launch Task" → "launch-task"); in-plan duplicate
// slugs get a deterministic "-2" suffix. Because CustomTypeKey hashes the
// plan key, the emitted uniqueKey is stable across runs iff the model names
// the kind the same — better for re-import correlation than a free-form key.
// A slug landing on a bundled key ("task") is re-keyed by sanitizeNewTypes.
func kindTypeKey(name, fallbackName string, usedKeys map[domain.TypeKey]bool) domain.TypeKey {
	slug := strings.Join(strings.Fields(typesuggest.Normalize(name)), "-")
	if slug == "" {
		slug = strings.Join(strings.Fields(typesuggest.Normalize(fallbackName)), "-")
	}
	if slug == "" {
		slug = "kind"
	}
	// A slug that collides with a bundled type key must be avoided HERE, not
	// left for sanitizeNewTypes to re-key. Its rename table is applied to
	// every container plan's TypeKey, including the bundled verdicts that
	// unassigned containers inherit from typesuggest — so a kind the model
	// named "Task" (slug "task", re-keyed to "plan_task") would silently pull
	// every naive-typed `task` container onto this minted type, bypassing the
	// coverage gate that exists to keep unrelated databases off one type.
	// "kind-" keeps the key stable across runs, which §7 re-import
	// correlation depends on.
	if bundle.HasObjectTypeByKey(domain.TypeKey(slug)) {
		slug = "kind-" + slug
	}
	key := domain.TypeKey(slug)
	for suffix := 2; usedKeys[key]; suffix++ {
		key = domain.TypeKey(slug + "-" + strconv.Itoa(suffix))
	}
	usedKeys[key] = true
	return key
}

// resolveFeatured maps the model's featured property names onto target keys:
// a name matches a member property when the two are equal after
// trimming surrounding whitespace (Unicode-exact otherwise — the evidence
// carries names like "Email 📧 " and the model predictably writes the trimmed
// form). A name matching properties of several formats resolves to the
// (name, format) pair present in the most member containers, ties broken by
// lowest format value. A name matching nothing — or only a tag-skipped or
// vocabulary-vetoed property, which has no single kind relation to feature —
// is dropped silently; the prompt said it would be. Capped at 4.
func resolveFeatured(kind KindPlan, schemaById map[string]ContainerSchema,
	salted map[nameFormat]bool) []domain.RelationKey {
	names := kind.FeaturedNames
	if len(names) > 4 {
		names = names[:4]
	}
	var out []domain.RelationKey
	for _, featuredName := range names {
		wanted := strings.TrimSpace(featuredName)
		// candidate (name, format) pairs → count of member containers carrying
		// them.
		counts := map[nameFormat]int{}
		for _, containerId := range kind.ContainerIds {
			seen := map[nameFormat]bool{}
			for _, property := range schemaById[containerId].Properties {
				if strings.TrimSpace(property.Name) != wanted {
					continue
				}
				pair := nameFormat{name: property.Name, format: property.Format}
				if !seen[pair] {
					seen[pair] = true
					counts[pair]++
				}
			}
		}
		best := nameFormat{}
		bestCount := 0
		for pair, count := range counts {
			if count > bestCount || (count == bestCount && bestCount > 0 && pair.format < best.format) {
				best, bestCount = pair, count
			}
		}
		if bestCount == 0 || salted[best] {
			continue
		}
		out = append(out, featuredTargetKey(best, kind, schemaById))
	}
	return out
}

// featuredTargetKey resolves a (name, format) pair to the key its plan
// entries actually carry: the bundled target when the rules table claimed it
// in some member, else the kind-local key. Tag-skipped properties have no
// entry and resolve to the kind-local key, which the union does not carry —
// the caller's lookup then drops the slot, costing a header line, never data.
func featuredTargetKey(pair nameFormat, kind KindPlan, schemaById map[string]ContainerSchema) domain.RelationKey {
	for _, containerId := range kind.ContainerIds {
		schema := schemaById[containerId]
		bundled := bundledTargets(schema)
		for _, property := range schema.Properties {
			if property.Name == pair.name && property.Format == pair.format && bundled[property.Id] != "" {
				return bundled[property.Id]
			}
		}
	}
	return kindLocalKey(pair.name, pair.format)
}
