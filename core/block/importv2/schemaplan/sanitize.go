package schemaplan

import (
	"fmt"
	"sort"
	"strings"

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
var AllowedBundledTargets = []AllowedBundledTarget{
	{bundle.RelationKeyDueDate, "deadline / due date"},
	{bundle.RelationKeyDone, "completion checkbox"},
	{bundle.RelationKeyPriority, "numeric priority"},
	{bundle.RelationKeyTag, "generic labels"},
	{bundle.RelationKeyEmail, "email address"},
	{bundle.RelationKeyPhone, "phone number"},
	{bundle.RelationKeyCompany, "company / organization"},
	{bundle.RelationKeyAuthor, "author / creator of a work"},
	{bundle.RelationKeyGenre, "genre"},
	{bundle.RelationKeyAssignee, "person responsible (object link)"},
}

var allowedBundled = func() map[domain.RelationKey]bool {
	out := make(map[domain.RelationKey]bool, len(AllowedBundledTargets))
	for _, target := range AllowedBundledTargets {
		out[target.Key] = true
	}
	return out
}()

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

	newTypes := sanitizeNewTypes(plan.NewTypes, report)
	planTypes := make(map[domain.TypeKey]bool, len(newTypes))
	// anchors fixes one format per custom target key across the whole plan —
	// the shared relation is emitted once, so every contributor must agree.
	// Type definitions declare first; containers follow in sorted order.
	anchors := map[domain.RelationKey]model.RelationFormat{}
	for _, def := range newTypes {
		planTypes[def.Key] = true
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
		clean := sanitizeContainer(schema, containerPlan, planTypes, anchors, report)
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

func sanitizeNewTypes(defs []TypeDefinition, report func(importv2.Issue)) []TypeDefinition {
	var out []TypeDefinition
	seen := make(map[domain.TypeKey]bool)
	for _, def := range defs {
		switch {
		case def.Key == "" || def.Name == "":
			report(dropped(string(def.Key), "new type without key or name"))
			continue
		case bundle.HasObjectTypeByKey(def.Key):
			// The bundled type wins; containers naming this key type onto it.
			continue
		case seen[def.Key]:
			report(dropped(string(def.Key), "duplicate new type"))
			continue
		}
		seen[def.Key] = true
		var props []TypeProperty
		for _, prop := range def.Properties {
			if prop.Key == "" {
				report(dropped(string(def.Key), "new type property without key"))
				continue
			}
			if bundle.HasRelation(prop.Key) && !allowedBundled[prop.Key] {
				report(dropped(string(def.Key), fmt.Sprintf("bundled relation %q is not an allowed plan target", prop.Key)))
				continue
			}
			props = append(props, prop)
		}
		def.Properties = props
		out = append(out, def)
	}
	return out
}

func sanitizeContainer(schema ContainerSchema, plan ContainerPlan, planTypes map[domain.TypeKey]bool, anchors map[domain.RelationKey]model.RelationFormat, report func(importv2.Issue)) ContainerPlan {
	clean := ContainerPlan{Reason: plan.Reason}
	if plan.TypeKey != "" {
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
