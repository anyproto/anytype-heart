package schemaplan

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// AllowedBundledTarget is one bundled relation a plan may redirect properties
// onto, with the one-line semantic the prompt advertises.
type AllowedBundledTarget struct {
	Key  domain.RelationKey
	Hint string
}

// AllowedBundledTargets is the CLOSED set of legal bundled targets. The
// prompt offers exactly this set and Sanitize enforces it: bundle.HasRelation
// alone would admit ~200 system relations (isArchived, isHidden, coverId, …)
// whose values are machine bookkeeping — a checkbox remapped onto isArchived
// would import every ticked row invisible.
//
// The set is small on purpose. Since the plan always mints its own types, a
// bundled relation is only worth targeting when it is load-bearing (`done` is
// a required relation of the todo layout, and the title-row checkbox reads it)
// or when its space-wide option pool is the actual point (`tag`, `genre`).
// Deliberately absent:
//
//   - `assignee` / `author` are `objects` targeting contact/participant, which
//     minted people types do not satisfy;
//   - `company` is `objects` while a source "Company" is normally text;
//   - `priority` is a `number` sort key while a source "Priority" is normally
//     a select;
//   - `status` is a genuine select, but one option pool per space — admitting
//     it would merge every database's lifecycle vocabulary into one dropdown.
//     Per-container custom keys (see scopedKey) are how status is imported.
var AllowedBundledTargets = []AllowedBundledTarget{
	{bundle.RelationKeyDueDate, "deadline / due date"},
	{bundle.RelationKeyDone, "completion checkbox"},
	{bundle.RelationKeyTag, "labels meant to be shared space-wide"},
	{bundle.RelationKeyGenre, "genre, shared space-wide"},
	{bundle.RelationKeyEmail, "email address"},
	{bundle.RelationKeyPhone, "phone number"},
}

var allowedBundled = func() map[domain.RelationKey]bool {
	out := make(map[domain.RelationKey]bool, len(AllowedBundledTargets))
	for _, target := range AllowedBundledTargets {
		out[target.Key] = true
	}
	return out
}()

// AllowedIcons is the CLOSED icon vocabulary a plan may name for a minted
// type — a curated subset of core/api/model's 390 IconName constants, small
// enough to offer in the prompt. Same contract as AllowedBundledTargets: the
// prompt offers exactly this set and Sanitize drops anything else, so an
// unvalidated string can never reach a snapshot.
var AllowedIcons = []string{
	"document", "folder", "library", "newspaper", "bookmark", "book", "school",
	"checkbox", "briefcase", "build", "rocket", "flask", "bug", "flag", "trophy",
	"calendar", "time", "people", "person", "chatbubble", "mail", "call", "home",
	"cart", "cash", "wallet", "pricetag", "star", "heart", "location",
	"restaurant", "barbell", "musical-notes", "film", "image",
}

var allowedIcons = func() map[string]bool {
	out := make(map[string]bool, len(AllowedIcons))
	for _, icon := range AllowedIcons {
		out[icon] = true
	}
	return out
}()

// maxNameRunes bounds every plan-supplied display name. Models routinely write
// an explanation into the name field ("Contact Type remapped to Contact Type
// (select)"), and these land verbatim in RelationKeyName.
const maxNameRunes = 64

// cleanDisplayName removes characters that have no business in a label and
// collapses whitespace runs. Control characters (NUL, ANSI escapes) can upset
// storage and terminals, and format characters carry the bidi overrides that
// let a name render as something other than what it is — both reach
// RelationKeyName verbatim otherwise.
func cleanDisplayName(name string) string {
	stripped := strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			return ' ' // becomes a field separator below
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, name)
	return strings.Join(strings.Fields(stripped), " ")
}

// boundedName cleans a plan-supplied name and falls back rather than
// truncating mid-word: an over-long name is prose, not a label, so the source's
// own name is a better answer than the first 64 runes of an explanation. The
// fallback is clamped too, so a caller can always rely on getting something
// usable — returning "" would drop whatever the name belonged to.
func boundedName(name, fallback string) string {
	if cleaned := cleanDisplayName(name); cleaned != "" && len([]rune(cleaned)) <= maxNameRunes {
		return cleaned
	}
	cleaned := cleanDisplayName(fallback)
	if runes := []rune(cleaned); len(runes) > maxNameRunes {
		return strings.TrimSpace(string(runes[:maxNameRunes]))
	}
	return cleaned
}

// ScopedKey makes a plan key container-local. Two containers naming the same
// key for a select property would otherwise mint one relation and merge their
// option pools into a single dropdown — a live import collapsed four
// databases' "Category" vocabularies (Breakfast … Social Media) this way.
//
// Scoping is per container, not per page: members of one container still share
// the relation, which is what makes a select a select.
// The key is length-prefixed rather than merely separated, because both halves
// are attacker-controlled: a plain "key@scope" join lets ScopedKey("a", "b@c")
// and ScopedKey("a@b", "c") produce the same string, which would hand two
// different types one relation and merge their option pools — the very defect
// scoping exists to prevent. Notion property ids and Obsidian folder paths can
// both contain "@", so this is reachable without a hostile model.
func ScopedKey(planKey domain.RelationKey, scope string) domain.RelationKey {
	return domain.RelationKey(strconv.Itoa(len(planKey)) + "@" + string(planKey) + scope)
}

// scopeMap decides, once and up front, every plan key that must become
// type-local. A property belongs to the type that declares it; sharing one
// across types is the whitelisted exception, spelled by naming a bundled
// target from AllowedBundledTargets. Everything else is scoped, so two
// containers can never be handed the same relation by accident.
//
// The scope is the container's type when it has a REAL one — containers
// sharing a type are one kind of thing and should share its properties — and
// the container itself otherwise.
//
// "Real" matters: a TypeKey naming a type the plan never defined is dropped
// later by sanitizeContainer, and scoping by it anyway would let two
// containers share a relation on the strength of a kind that does not exist.
// Type keys and container ids are also one string namespace, so an
// unvalidated TypeKey can name another container and collide with its scope.
// canonical resolves a re-keyed definition back to the key its containers
// named, so a bundled-collision rename cannot split the two apart.
//
// Deciding it here rather than mid-sanitize is what keeps a type definition
// and its container on the same relation: settle it in two places and they
// silently diverge, the type declaring one format while the container writes
// another.
func scopeMap(containers map[string]ContainerPlan, schemaById map[string]ContainerSchema,
	planTypes map[domain.TypeKey]bool, renamed map[domain.TypeKey]domain.TypeKey) map[string]map[domain.RelationKey]domain.RelationKey {
	originalOf := make(map[domain.TypeKey]domain.TypeKey, len(renamed))
	for original, current := range renamed {
		originalOf[current] = original
	}
	out := map[string]map[domain.RelationKey]domain.RelationKey{}
	for containerId, containerPlan := range containers {
		if _, ok := schemaById[containerId]; !ok {
			continue
		}
		scope := containerId
		if typeKey := containerPlan.TypeKey; typeKey != "" {
			// Definitions scope by the key their containers named, so resolve a
			// container that named the re-keyed form back to that original.
			if original, wasRenamed := originalOf[typeKey]; wasRenamed {
				typeKey = original
			}
			if planTypes[typeKey] || planTypes[renamed[typeKey]] || bundle.HasObjectTypeByKey(typeKey) {
				scope = typeKey.String()
			}
		}
		for _, propertyPlan := range containerPlan.Properties {
			if propertyPlan.Key == "" || bundle.HasRelation(propertyPlan.Key) {
				continue
			}
			if out[containerId] == nil {
				out[containerId] = map[domain.RelationKey]domain.RelationKey{}
			}
			out[containerId][propertyPlan.Key] = ScopedKey(propertyPlan.Key, scope)
		}
	}
	return out
}

// textFormats can substitute for each other: values render as text either way.
var textFormats = map[model.RelationFormat]bool{
	model.RelationFormat_longtext:  true,
	model.RelationFormat_shorttext: true,
	model.RelationFormat_url:       true,
	model.RelationFormat_email:     true,
	model.RelationFormat_phone:     true,
}

// listFormats can substitute for each other: values are option lists.
var listFormats = map[model.RelationFormat]bool{
	model.RelationFormat_status: true,
	model.RelationFormat_tag:    true,
}

// FormatChangeAllowed says whether values emitted in `from` format can be
// carried by a relation of `to` format without value conversion. Conservative
// by design: dates, numbers, checkboxes, files and object links never change.
func FormatChangeAllowed(from, to model.RelationFormat) bool {
	if from == to {
		return true
	}
	if textFormats[from] && textFormats[to] {
		return true
	}
	if listFormats[from] && listFormats[to] {
		return true
	}
	return false
}

// SummarizeError renders an error for a user-facing issue: first line only,
// bounded — provider errors can embed whole response bodies, and issue text
// persists onto the import report page.
func SummarizeError(err error) string {
	message := err.Error()
	if idx := strings.IndexByte(message, '\n'); idx >= 0 {
		message = message[:idx]
	}
	const maxRunes = 200
	runes := []rune(message)
	if len(runes) > maxRunes {
		message = string(runes[:maxRunes]) + "…"
	}
	return message
}

// Sanitize validates a plan against the schemas it was made for and returns
// the trustworthy subset, with every property entry's Format normalized to
// the exact format the emitted relation will carry. Every dropped entry is
// reported as an llmPlanEntryDropped warning, in deterministic (sorted)
// order; converters apply the result without further checks. The zero plan
// sanitizes to itself.
func Sanitize(plan Plan, schemas []ContainerSchema, report func(importv2.Issue)) Plan {
	if report == nil {
		report = func(importv2.Issue) {}
	}
	schemaById := make(map[string]ContainerSchema, len(schemas))
	for _, schema := range schemas {
		schemaById[schema.Id] = schema
	}

	// Only used to give a nameless type its container's name; several
	// containers may legitimately share one type, and then there is no single
	// owner to borrow from.
	owners := typeOwners(plan.Containers)
	newTypes, renamed := sanitizeNewTypes(plan.NewTypes, owners, schemaById, report)

	planTypes := make(map[domain.TypeKey]bool, len(newTypes))
	for _, def := range newTypes {
		planTypes[def.Key] = true
	}
	// Scoping is decided only once the surviving types are known: a container
	// naming a type that did not survive must fall back to its own id, or two
	// containers would share a relation on the strength of a kind that never
	// materialised.
	scopes := scopeMap(plan.Containers, schemaById, planTypes, renamed)

	// anchors fixes one format per custom target key across the whole plan —
	// the shared relation is emitted once, so every contributor must agree.
	// Type definitions declare first; containers follow in sorted order.
	anchors := map[domain.RelationKey]model.RelationFormat{}
	for _, def := range newTypes {
		for _, prop := range def.Properties {
			if !bundle.HasRelation(prop.Key) && prop.Format != 0 {
				if _, taken := anchors[prop.Key]; !taken {
					anchors[prop.Key] = prop.Format
				}
			}
		}
	}

	out := Plan{}
	for _, containerId := range sortedKeys(plan.Containers) {
		containerPlan := plan.Containers[containerId]
		schema, ok := schemaById[containerId]
		if !ok {
			report(dropped(containerId, "unknown container"))
			continue
		}
		if renamedKey, ok := renamed[containerPlan.TypeKey]; ok {
			containerPlan.TypeKey = renamedKey
		}
		clean := sanitizeContainer(schema, containerPlan, planTypes, anchors, scopes[containerId], report)
		if clean.TypeKey == "" && len(clean.Properties) == 0 {
			continue
		}
		if out.Containers == nil {
			out.Containers = make(map[string]ContainerPlan)
		}
		out.Containers[containerId] = clean
	}

	// Normalize type-definition property formats to their anchors so the
	// pre-emitted relations carry the format the containers settled on.
	for i := range newTypes {
		for j := range newTypes[i].Properties {
			prop := &newTypes[i].Properties[j]
			if bundle.HasRelation(prop.Key) || prop.Format != 0 {
				continue
			}
			if anchor, ok := anchors[prop.Key]; ok {
				prop.Format = anchor
			} else {
				prop.Format = model.RelationFormat_longtext
			}
		}
	}
	out.NewTypes = newTypes
	return out
}

// typeOwners maps each plan type key to the single container that names it.
// A type named by several containers has no unique owner and reports false.
func typeOwners(containers map[string]ContainerPlan) map[domain.TypeKey]string {
	count := map[domain.TypeKey]int{}
	owner := map[domain.TypeKey]string{}
	for _, containerId := range sortedKeys(containers) {
		key := containers[containerId].TypeKey
		if key == "" {
			continue
		}
		count[key]++
		owner[key] = containerId
	}
	for key, n := range count {
		if n > 1 {
			delete(owner, key)
		}
	}
	return owner
}

func sanitizeNewTypes(defs []TypeDefinition, owners map[domain.TypeKey]string,
	schemaById map[string]ContainerSchema, report func(importv2.Issue)) ([]TypeDefinition, map[domain.TypeKey]domain.TypeKey) {
	var out []TypeDefinition
	seen := make(map[domain.TypeKey]bool)
	renamed := map[domain.TypeKey]domain.TypeKey{}
	for _, def := range defs {
		if def.Key == "" || def.Name == "" {
			report(dropped(string(def.Key), "new type without key or name"))
			continue
		}
		// Containers scope their property keys by the type key they name, so a
		// definition must scope by its own — its ORIGINAL key, before any
		// re-keying below, which is what the containers saw. Routing this
		// through an owning container instead would break the moment several
		// containers share one type, leaving the type's recommended relations
		// pointing at a relation nobody emits.
		original := def.Key
		scope := original.String()
		if bundle.HasObjectTypeByKey(def.Key) {
			// The plan always mints its own types: reusing a bundled key would
			// reshape the built-in type space-wide and hand it to a migration
			// that can rewrite its featured properties. Re-key rather than
			// drop, so a model spelling its type "task" still gets a working
			// type instead of the container silently losing one.
			def.Key = domain.TypeKey("plan_" + original.String())
		}
		if seen[def.Key] {
			report(dropped(string(def.Key), "duplicate new type"))
			continue
		}
		seen[def.Key] = true
		// Record the rename only once the definition is known to survive:
		// registering it earlier would retype containers onto whichever
		// definition happened to hold the re-keyed name already.
		if def.Key != original {
			renamed[original] = def.Key
			if owner, ok := owners[original]; ok {
				owners[def.Key] = owner
			}
		}

		// Names reach RelationKeyName verbatim; a model writing prose into the
		// field must not be able to name a user's type after its own reasoning.
		// The key is the last-resort fallback: several containers sharing one
		// type is the designed common case and leaves no single container name
		// to borrow, and dropping the type there would silently undo minting.
		fallback := original.String()
		if owner, ok := owners[def.Key]; ok && schemaById[owner].Name != "" {
			fallback = schemaById[owner].Name
		}
		def.Name = boundedName(def.Name, fallback)
		if def.Name == "" {
			report(dropped(string(def.Key), "new type without key or name"))
			continue
		}
		def.PluralName = boundedName(def.PluralName, "")
		if !allowedIcons[def.IconName] {
			def.IconName = ""
		}
		var props []TypeProperty
		seenProps := map[domain.RelationKey]bool{}
		for _, prop := range def.Properties {
			if prop.Key == "" {
				report(dropped(string(def.Key), "new type property without key"))
				continue
			}
			// The type's relations are emitted BEFORE any container resolves
			// one, so an unbounded name here wins over the container's bounded
			// one. Falling back to the unscoped key keeps the internal
			// "key@scope" spelling out of a user-visible property name.
			prop.Name = boundedName(prop.Name, string(prop.Key))
			if bundle.HasRelation(prop.Key) {
				if !allowedBundled[prop.Key] {
					report(dropped(string(def.Key), fmt.Sprintf("bundled relation %q is not an allowed plan target", prop.Key)))
					continue
				}
			} else {
				prop.Key = ScopedKey(prop.Key, scope)
			}
			if seenProps[prop.Key] {
				// One relation cannot be both featured and regular, nor carry
				// two declared formats.
				report(dropped(string(def.Key), fmt.Sprintf("duplicate type property %q", prop.Key)))
				continue
			}
			seenProps[prop.Key] = true
			props = append(props, prop)
		}
		def.Properties = props
		out = append(out, def)
	}
	return out, renamed
}

func sanitizeContainer(schema ContainerSchema, plan ContainerPlan, planTypes map[domain.TypeKey]bool,
	anchors map[domain.RelationKey]model.RelationFormat, scoped map[domain.RelationKey]domain.RelationKey,
	report func(importv2.Issue)) ContainerPlan {
	clean := ContainerPlan{Reason: plan.Reason}
	if plan.TypeKey != "" {
		// Pointing a container's pages at a bundled type stays legal: that is
		// what the naive planner does with every typesuggest verdict, and it
		// changes nothing about the bundled type itself. Preferring a minted
		// type is a fit judgement the prompt asks the LLM for — not a safety
		// invariant, so it is not enforced here. Defining a type document
		// under a bundled key is the dangerous one, and sanitizeNewTypes
		// re-keys that.
		if bundle.HasObjectTypeByKey(plan.TypeKey) || planTypes[plan.TypeKey] {
			clean.TypeKey = plan.TypeKey
		} else {
			report(dropped(schema.Id, fmt.Sprintf("unknown type %q", plan.TypeKey)))
		}
	}

	propertyById := make(map[string]PropertySchema, len(schema.Properties))
	for _, prop := range schema.Properties {
		propertyById[prop.Id] = prop
	}
	takenTargets := map[domain.RelationKey]string{} // target key → first source property
	for _, propertyId := range sortedKeys(plan.Properties) {
		propertyPlan := plan.Properties[propertyId]
		source, ok := propertyById[propertyId]
		if !ok {
			report(dropped(schema.Id, fmt.Sprintf("unknown property %q", propertyId)))
			continue
		}
		if scopedKey, ok := scoped[propertyPlan.Key]; ok {
			propertyPlan.Key = scopedKey
		}
		cleanProp, ok := sanitizeProperty(schema.Id, source, propertyPlan, anchors, report)
		if !ok {
			continue
		}
		if first, taken := takenTargets[cleanProp.Key]; taken {
			// Two source properties onto one target would silently collide
			// last-writer-wins on every page's details.
			report(dropped(schema.Id, fmt.Sprintf("property %q duplicates target %q already taken by %q", source.Name, cleanProp.Key, first)))
			continue
		}
		takenTargets[cleanProp.Key] = source.Name
		if clean.Properties == nil {
			clean.Properties = make(map[string]PropertyPlan)
		}
		clean.Properties[propertyId] = cleanProp
	}
	return clean
}

func sanitizeProperty(containerId string, source PropertySchema, plan PropertyPlan, anchors map[domain.RelationKey]model.RelationFormat, report func(importv2.Issue)) (PropertyPlan, bool) {
	if plan.Key == "" {
		report(dropped(containerId, fmt.Sprintf("property %q remap without target", source.Name)))
		return PropertyPlan{}, false
	}
	if bundle.HasRelation(plan.Key) {
		if !allowedBundled[plan.Key] {
			report(dropped(containerId, fmt.Sprintf("bundled relation %q is not an allowed plan target", plan.Key)))
			return PropertyPlan{}, false
		}
		bundled := bundle.MustGetRelation(plan.Key)
		if !FormatChangeAllowed(source.Format, bundled.Format) {
			report(dropped(containerId, fmt.Sprintf(
				"property %q (%s) cannot become %q (%s)",
				source.Name, source.Format.String(), plan.Key, bundled.Format.String())))
			return PropertyPlan{}, false
		}
		// Bundled targets own their name and format.
		return PropertyPlan{Key: plan.Key, Format: bundled.Format}, true
	}
	// Custom target: settle the effective format now — the plan's explicit
	// override when legal, else the source format — and hold it against the
	// key's anchor so a shared relation is never fed two disagreeing formats.
	effective := plan.Format
	if effective == 0 {
		effective = source.Format
	} else if !FormatChangeAllowed(source.Format, effective) {
		report(dropped(containerId, fmt.Sprintf(
			"property %q format %s cannot become %s",
			source.Name, source.Format.String(), effective.String())))
		return PropertyPlan{}, false
	}
	plan.Name = boundedName(plan.Name, source.Name)
	if anchor, ok := anchors[plan.Key]; ok {
		if !FormatChangeAllowed(effective, anchor) {
			report(dropped(containerId, fmt.Sprintf(
				"property %q (%s) conflicts with target %q used as %s elsewhere",
				source.Name, effective.String(), plan.Key, anchor.String())))
			return PropertyPlan{}, false
		}
		effective = anchor
	} else {
		anchors[plan.Key] = effective
	}
	plan.Format = effective
	return plan, true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func dropped(sourceKey, message string) importv2.Issue {
	return importv2.Warning(importv2.IssueLLMPlanEntryDropped, sourceKey, message)
}
