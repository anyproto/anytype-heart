package schemaplan

import (
	"fmt"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// deniedTargets are relation keys a plan may never redirect a property onto:
// object identity and machine-managed bookkeeping. Everything else the plan
// proposes still has to pass the bundled-key/format checks below.
var deniedTargets = map[domain.RelationKey]bool{
	bundle.RelationKeyName:           true,
	bundle.RelationKeyId:             true,
	bundle.RelationKeyType:           true,
	bundle.RelationKeyLayout:         true,
	bundle.RelationKeyResolvedLayout: true,
	bundle.RelationKeySpaceId:        true,
	bundle.RelationKeyCreator:        true,
	bundle.RelationKeyCreatedDate:    true,
	bundle.RelationKeySourceFilePath: true,
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

// formatChangeAllowed says whether values emitted in `from` format can be
// carried by a relation of `to` format without value conversion. Conservative
// by design: dates, numbers, checkboxes, files and object links never change.
func formatChangeAllowed(from, to model.RelationFormat) bool {
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

// Sanitize validates a plan against the schemas it was made for and returns
// the trustworthy subset. Every dropped entry is reported as an
// llmPlanEntryDropped warning; converters apply the result without further
// checks. The zero plan sanitizes to itself.
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
	for _, def := range newTypes {
		planTypes[def.Key] = true
	}

	out := Plan{NewTypes: newTypes}
	for containerId, containerPlan := range plan.Containers {
		schema, ok := schemaById[containerId]
		if !ok {
			report(dropped(containerId, "unknown container"))
			continue
		}
		clean := sanitizeContainer(schema, containerPlan, planTypes, report)
		if clean.TypeKey == "" && len(clean.Properties) == 0 {
			continue
		}
		if out.Containers == nil {
			out.Containers = make(map[string]ContainerPlan)
		}
		out.Containers[containerId] = clean
	}
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
			if prop.Key == "" || deniedTargets[prop.Key] {
				report(dropped(string(def.Key), fmt.Sprintf("new type property %q not allowed", prop.Key)))
				continue
			}
			props = append(props, prop)
		}
		def.Properties = props
		out = append(out, def)
	}
	return out
}

func sanitizeContainer(schema ContainerSchema, plan ContainerPlan, planTypes map[domain.TypeKey]bool, report func(importv2.Issue)) ContainerPlan {
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
	for propertyId, propertyPlan := range plan.Properties {
		source, ok := propertyById[propertyId]
		if !ok {
			report(dropped(schema.Id, fmt.Sprintf("unknown property %q", propertyId)))
			continue
		}
		cleanProp, ok := sanitizeProperty(schema.Id, source, propertyPlan, report)
		if !ok {
			continue
		}
		if clean.Properties == nil {
			clean.Properties = make(map[string]PropertyPlan)
		}
		clean.Properties[propertyId] = cleanProp
	}
	return clean
}

func sanitizeProperty(containerId string, source PropertySchema, plan PropertyPlan, report func(importv2.Issue)) (PropertyPlan, bool) {
	if plan.Key == "" {
		report(dropped(containerId, fmt.Sprintf("property %q remap without target", source.Name)))
		return PropertyPlan{}, false
	}
	if deniedTargets[plan.Key] {
		report(dropped(containerId, fmt.Sprintf("property %q may not target %q", source.Name, plan.Key)))
		return PropertyPlan{}, false
	}
	if bundle.HasRelation(plan.Key) {
		bundled := bundle.MustGetRelation(plan.Key)
		if !formatChangeAllowed(source.Format, bundled.Format) {
			report(dropped(containerId, fmt.Sprintf(
				"property %q (%s) cannot become %q (%s)",
				source.Name, source.Format.String(), plan.Key, bundled.Format.String())))
			return PropertyPlan{}, false
		}
		// Bundled targets own their name and format.
		return PropertyPlan{Key: plan.Key}, true
	}
	if plan.Format != 0 && !formatChangeAllowed(source.Format, plan.Format) {
		report(dropped(containerId, fmt.Sprintf(
			"property %q format %s cannot become %s",
			source.Name, source.Format.String(), plan.Format.String())))
		return PropertyPlan{}, false
	}
	return plan, true
}

func dropped(sourceKey, message string) importv2.Issue {
	return importv2.Warning(importv2.IssueLLMPlanEntryDropped, sourceKey, message)
}
