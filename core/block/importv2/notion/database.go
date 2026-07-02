package notion

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// databaseObject is the GET /databases/{id} payload subset the converter
// uses. Fetched fresh in pass 2 (search bodies are not retained).
type databaseObject struct {
	Id             string                    `json:"id"`
	Title          []richText                `json:"title"`
	Description    []richText                `json:"description"`
	Icon           *iconValue                `json:"icon"`
	Cover          *fileValue                `json:"cover"`
	Archived       bool                      `json:"archived"`
	CreatedTime    string                    `json:"created_time"`
	LastEditedTime string                    `json:"last_edited_time"`
	Properties     map[string]propertySchema `json:"properties"`
}

// convertDatabase emits, in order: new relation definitions, their declared
// options, then the collection object whose members are the database's
// pages (known from the pass-1 hierarchy).
func (c *Converter) convertDatabase(ctx context.Context, stub Entity, sink importv2.Sink) error {
	var database databaseObject
	if err := c.client.Request(ctx, http.MethodGet, "/databases/"+stub.Id, nil, &database); err != nil {
		sink.Issue(importv2.ObjectError(importv2.IssueObjectFailed, stub.Id, fmt.Errorf("fetch database: %w", err)))
		return nil
	}

	for name := range database.Properties {
		c.properties.noteName(name)
	}
	names := make([]string, 0, len(database.Properties))
	for name := range database.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	details := domain.NewDetails()
	var schemaDefs []*relationDef
	for _, name := range names {
		property := database.Properties[name]
		property.Name = name
		if property.Type == "title" {
			continue // the title property is the object name, not a relation
		}
		if property.Type == "verification" {
			sink.Issue(importv2.Warning(importv2.IssueDataLoss, stub.Id,
				fmt.Sprintf("property %q (verification) has no anytype counterpart and was skipped", name)))
			continue
		}
		def, err := c.emitProperty(ctx, property, sink)
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

	object, err := c.factory.MakeCollection(plainText(database.Title), c.databaseMembers(stub.Id))
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
	object.Payload.Details.SetString(bundle.RelationKeyName, plainText(database.Title))
	if description := plainText(database.Description); description != "" {
		object.Payload.Details.SetString(bundle.RelationKeyDescription, description)
	}
	object.Payload.Details.SetString(bundle.RelationKeySourceFilePath, stub.Id)
	setTimestamps(object.Payload.Details, database.CreatedTime, database.LastEditedTime)
	if err := c.applyIcon(ctx, object, database.Icon, database.Cover, sink); err != nil {
		return err
	}
	wireDataview(object, schemaDefs)
	return sink.Object(ctx, object)
}

// emitProperty resolves one property against the shared store and emits the
// relation and its schema-declared options on first sight.
func (c *Converter) emitProperty(ctx context.Context, property propertySchema, sink importv2.Sink) (*relationDef, error) {
	if _, supported := relationFormatOf(property.Type); !supported {
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, property.Id,
			fmt.Sprintf("property %q of type %q is not supported and was skipped", property.Name, property.Type)))
		return nil, nil
	}
	def, created := c.properties.resolveRelation(property)
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

// databaseMembers lists the database's pages in stream (search) order.
func (c *Converter) databaseMembers(databaseId string) []string {
	var members []string
	for _, page := range c.pages {
		if page.Parent.Type == "database_id" && page.Parent.DatabaseId == databaseId {
			members = append(members, page.Id)
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
		view.Relations = append(view.Relations, &model.BlockContentDataviewRelation{
			Key:       def.key,
			IsVisible: true,
		})
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
