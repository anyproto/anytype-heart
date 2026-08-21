package notion

import (
	"context"
	"fmt"
	"sort"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/block/importv2/typesuggest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// databaseObject is the GET /data_sources/{id} payload subset the converter
// uses (the schema moved from the database to its data sources in the
// 2025-09-03 API). Fetched fresh in pass 2 (search bodies are not retained).
type databaseObject struct {
	Id             string                    `json:"id"`
	Title          []richText                `json:"title"`
	Name           string                    `json:"name"`
	Description    []richText                `json:"description"`
	Icon           *iconValue                `json:"icon"`
	Cover          *fileValue                `json:"cover"`
	Archived       bool                      `json:"archived"`
	CreatedTime    string                    `json:"created_time"`
	LastEditedTime string                    `json:"last_edited_time"`
	Properties     map[string]propertySchema `json:"properties"`
}

func (d *databaseObject) title() string {
	if title := plainText(d.Title); title != "" {
		return title
	}
	return d.Name
}

// convertDatabase emits, in order: new relation definitions, their declared
// options, then the collection object whose members are the data source's
// pages (known from the pass-1 hierarchy).
func (c *Converter) convertDatabase(ctx context.Context, stub Entity, sink importv2.Sink) error {
	sink.Item(importv2.DisplayText(stub.Title)) // §15 currentItem, see emitFetchedPage
	fetch := c.schemaFetches[stub.Id]
	if fetch == nil {
		// Late discovery — pass-1 schemas were prefetched by the plan phase,
		// this stub was found during pass 2.
		var err error
		if fetch, err = c.fetchSchema(ctx, stub); err != nil {
			return err
		}
	}
	if fetch.issue != nil {
		sink.Issue(*fetch.issue)
		return nil
	}
	database, schemaId := fetch.database, fetch.schemaId
	c.registerPropertyScope(stub.Id, schemaId)

	for name := range database.Properties {
		c.properties.noteName(name)
	}
	names := make([]string, 0, len(database.Properties))
	for name := range database.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	var containerPlan schemaplan.ContainerPlan
	if c.planned[stub.Id] {
		containerPlan = c.plan.Containers[stub.Id]
	}

	details := domain.NewDetails()
	var schemaDefs []*relationDef
	for _, name := range names {
		property := database.Properties[name]
		property.Name = name
		if property.Type == "title" {
			continue // the title property is the object name, not a relation
		}
		planProp := containerPlan.Properties[property.Id]
		if property.Type == "formula" || property.Type == "rollup" {
			// Their format is value-typed (a date formula must become a date
			// relation); the first page value defines the relation instead
			// of the schema committing a generic text format.
			if planProp.Key != "" {
				sink.Issue(importv2.Warning(importv2.IssueLLMPlanEntryDropped, stub.Id,
					"Formula and rollup properties take their type from their values, so the suggested mapping could not be applied").About(name))
			}
			continue
		}
		if property.Type == "verification" {
			sink.Issue(importv2.Warning(importv2.IssueDataLoss, stub.Id,
				`Notion "verification" properties have no Anytype counterpart and were skipped`).About(name))
			continue
		}
		def, err := c.emitProperty(ctx, c.propertyScope(stub.Id), property, planProp, container{key: stub.Id, name: database.title(), schema: true}, sink)
		if err != nil {
			return err
		}
		if def == nil {
			continue
		}
		schemaDefs = append(schemaDefs, def)
		// Seed the zero value so the relation shows on the collection even
		// when no page fills it (v1 behavior).
		details.Set(domain.RelationKey(def.key), zeroValueOf(def.format))
	}

	if c.planned[stub.Id] {
		c.applyPlanType(stub.Id, schemaId, database, sink)
	} else {
		c.suggestPageType(stub.Id, schemaId, database, names, sink)
	}

	if c.typeBackedContainers[stub.Id] {
		// The minted type already carries this database's identity, and a
		// collection over a single-database type lists exactly the type's own
		// objects. Emitting both would duplicate the same list. The type is
		// emitted here rather than in the plan phase so it can recommend the
		// relations this loop just created.
		return c.emitDeferredType(ctx, stub, schemaDefs, sink)
	}

	object, err := c.factory.MakeCollection(database.title(), c.databaseMembers(stub.Id, schemaId))
	if err != nil {
		return fmt.Errorf("make database collection: %w", err)
	}
	object.SourceKey = stub.Id
	object.SbType = coresb.SmartBlockTypePage
	object.IsRootCandidate = c.isRootCandidate(stub)
	object.Archived = database.Archived
	for key, value := range details.Iterate() {
		object.Payload.Details.Set(key, value)
	}
	object.Payload.Details.SetString(bundle.RelationKeyName, database.title())
	if description := plainText(database.Description); description != "" {
		object.Payload.Details.SetString(bundle.RelationKeyDescription, description)
	}
	object.Payload.Details.SetString(bundle.RelationKeySourceFilePath, stub.Id)
	setTimestamps(object.Payload.Details, database.CreatedTime, database.LastEditedTime)
	if err := c.applyIcon(ctx, object, database.Icon, database.Cover, "/data_sources/"+schemaId, sink); err != nil {
		return err
	}
	wireDataview(object, schemaDefs)
	return sink.Object(ctx, object)
}

// suggestPageType records a bundled object type for the data source's rows
// when the database looks like one (name/shape rules, §11.5). Rows would
// otherwise import as plain Pages, so a confident suggestion only upgrades.
func (c *Converter) suggestPageType(entityId, schemaId string, database *databaseObject, sortedNames []string, sink importv2.Sink) {
	evidence := typesuggest.Evidence{ContainerName: database.title()}
	for _, name := range sortedNames {
		property := database.Properties[name]
		if property.Type == "title" {
			continue
		}
		format, supported := relationFormatOf(property.Type)
		if !supported {
			continue
		}
		evidence.Properties = append(evidence.Properties, typesuggest.Property{Name: name, Format: format})
	}
	suggestion, ok := c.suggestor.Suggest(evidence)
	if !ok {
		return
	}
	c.suggestedTypes[entityId] = suggestion.TypeKey
	c.suggestedTypes[schemaId] = suggestion.TypeKey
	sink.Issue(importv2.Issue{
		Severity: importv2.SeverityInfo, Code: importv2.IssueTypeSuggested, SourceKey: entityId,
		// The type and the reason belong to the subject, not the sentence:
		// interpolated into the message they would split this one fact into
		// a summary row per (type, reason) pair.
		Subject: fmt.Sprintf("%s → %s (%s)", database.title(), suggestion.TypeKey, suggestion.Reason),
		Message: "Rows of these databases were imported as an existing Anytype type",
	})
}

// emitProperty resolves one property against the shared store and emits the
// relation and its schema-declared options on first sight. A non-zero
// planProp redirects the property onto the plan's target relation instead of
// minting one from the notion id (docs/ImportV2LLM.md §4).
// container identifies what a property belongs to, for reporting: the source
// key so the report can link the database (or page) the user knows, and its
// title for the property-mapping note. Property ids are Notion's own
// ("%40egk") and mean nothing to a reader, so they are never the source key.
type container struct {
	key  string
	name string
	// schema marks the database's own declaration of a property, as opposed
	// to the copy every row carries. Unsupported types are reported from the
	// schema only: reporting per row said the same thing 112 times about one
	// workspace's button columns.
	schema bool
}

func (c *Converter) emitProperty(ctx context.Context, scope string, property propertySchema, planProp schemaplan.PropertyPlan, owner container, sink importv2.Sink) (*relationDef, error) {
	if _, supported := relationFormatOf(property.Type); !supported {
		switch {
		case !owner.schema:
			// The database that declares this property already reported it.
		case valuelessProperty(property.Type):
			sink.Issue(importv2.Issue{
				Severity: importv2.SeverityInfo, Code: importv2.IssueDataLoss, SourceKey: owner.key, Subject: property.Name,
				Message: fmt.Sprintf("Notion %q properties hold no value, so there was nothing to import", property.Type),
			})
		default:
			sink.Issue(importv2.Warning(importv2.IssueDataLoss, owner.key,
				fmt.Sprintf("Notion %q properties have no Anytype counterpart and were skipped", property.Type)).About(property.Name))
		}
		return nil, nil
	}
	var def *relationDef
	var created bool
	if planProp.Key != "" {
		def, created = c.properties.resolvePlanTarget(scope, property, planProp)
		if def == nil {
			// The shared target settled on a format this property's values
			// cannot carry — degrade to the unplanned path, loudly.
			sink.Issue(importv2.Warning(importv2.IssueLLMPlanEntryDropped, owner.key,
				"This property could not share the suggested relation (its values do not fit that format) and was imported on its own").About(property.Name))
			def, created = c.properties.resolveRelation(scope, property)
		} else {
			sink.Issue(importv2.Info(importv2.IssuePropertyMapped,
				fmt.Sprintf("database %q property %q imported as %q (%s)", owner.name, property.Name, def.name, def.key)))
		}
	} else {
		def, created = c.properties.resolveRelation(scope, property)
	}
	if def == nil {
		return nil, nil
	}
	if created {
		if err := sink.Object(ctx, relationObject(def)); err != nil {
			return nil, err
		}
	}
	for _, option := range schemaOptions(property) {
		sourceKey, optionCreated := c.properties.resolveOption(def.key, option.Name)
		if !optionCreated {
			continue
		}
		if err := sink.Object(ctx, optionObject(def.key, sourceKey, option.Name, option.Color)); err != nil {
			return nil, err
		}
	}
	return def, nil
}

func schemaOptions(property propertySchema) []selectOption {
	switch property.Type {
	case "select":
		return property.Select.Options
	case "multi_select":
		return property.MultiSelect.Options
	case "status":
		return property.Status.Options
	default:
		return nil
	}
}

// databaseMembers lists the data source's pages in stream (search) order.
// Pages parent onto data_source_id under the 2025-09-03 API; the database_id
// form is kept for defensive coverage of mixed responses.
func (c *Converter) databaseMembers(entityId, dataSourceId string) []string {
	var members []string
	for _, page := range c.pages {
		switch page.Parent.Type {
		case "data_source_id":
			if page.Parent.DataSourceId == dataSourceId || page.Parent.DataSourceId == entityId {
				members = append(members, page.Id)
			}
		case "database_id":
			if page.Parent.DatabaseId == entityId {
				members = append(members, page.Id)
			}
		}
	}
	return members
}

// wireDataview surfaces the schema relations as visible dataview columns:
// relations already present in the default view (name, and tag when
// redirected) are made visible; new ones are appended to both the view and
// the dataview relation links (v1's ReplaceRelations/AddRelations split).
func wireDataview(object *importv2.Object, defs []*relationDef) {
	var dataview *model.BlockContentDataview
	for _, block := range object.Payload.Blocks {
		if content := block.GetDataview(); content != nil {
			dataview = content
			break
		}
	}
	if dataview == nil || len(dataview.Views) == 0 {
		return
	}
	view := dataview.Views[0]
	existing := map[string]*model.BlockContentDataviewRelation{}
	for _, relation := range view.Relations {
		existing[relation.Key] = relation
	}
	for _, def := range defs {
		if viewRelation, ok := existing[def.key]; ok {
			viewRelation.IsVisible = true
			continue
		}
		appended := &model.BlockContentDataviewRelation{
			Key:       def.key,
			IsVisible: true,
		}
		// Track appends too: plan remaps can resolve two source properties
		// onto one def, which must not duplicate the column.
		existing[def.key] = appended
		view.Relations = append(view.Relations, appended)
		dataview.RelationLinks = append(dataview.RelationLinks, &model.RelationLink{
			Key:    def.key,
			Format: def.format,
		})
		object.Payload.RelationLinks = append(object.Payload.RelationLinks, &model.RelationLink{
			Key:    def.key,
			Format: def.format,
		})
	}
}
