package notion

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// sampleTitleCap bounds the page titles shared per container when the request
// opted into content samples.
const sampleTitleCap = 8

// schemaFetch is one database stub's prefetched schema — or the issue that
// stands in for it, emitted (in database order) when the stub converts.
type schemaFetch struct {
	schemaId string
	database *databaseObject
	issue    *importv2.Issue
}

// planStructure is the plan phase (docs/ImportV2LLM.md §3): prefetch every
// pass-1 data-source schema, hand them all to the planner at once, sanitize
// what comes back, and emit the plan's new types. Runs before the first
// object is emitted; late-discovered databases are outside the plan and keep
// the naive per-container suggestion.
func (c *Converter) planStructure(ctx context.Context, sink importv2.Sink) error {
	for _, stub := range c.databases {
		fetch, err := c.fetchSchema(ctx, stub)
		if err != nil {
			return err
		}
		c.schemaFetches[stub.Id] = fetch
	}
	schemas := c.containerSchemas()
	if len(schemas) == 0 {
		return nil
	}
	for _, schema := range schemas {
		c.planned[schema.Id] = true
	}
	plan, err := c.planner.Plan(ctx, schemas)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("plan structure: %w", ctx.Err())
		}
		sink.Issue(importv2.Warning(importv2.IssueLLMPlanFailed, "plan",
			fmt.Sprintf("structure analysis unavailable (%s); imported with built-in rules", schemaplan.SummarizeError(err))))
		plan, _ = schemaplan.NewNaive().Plan(ctx, schemas)
	}
	c.plan = schemaplan.Sanitize(plan, schemas, sink.Issue)
	return c.emitPlanTypes(ctx, sink)
}

// fetchSchema resolves a database stub to its data-source schema. A fetch
// that fails without the run being cancelled degrades to an issue the caller
// emits at conversion time — same per-database degrade as before the plan
// phase existed.
func (c *Converter) fetchSchema(ctx context.Context, stub Entity) (*schemaFetch, error) {
	schemaId := stub.Id
	if stub.Kind == kindDatabase {
		// Defensive: a bare database result — resolve its first data source
		// for the schema (search normally returns data_source objects).
		var database struct {
			DataSources []struct {
				Id string `json:"id"`
			} `json:"data_sources"`
		}
		if err := c.client.Request(ctx, http.MethodGet, "/databases/"+stub.Id, nil, &database); err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("fetch database: %w", err)
			}
			issue := importv2.ObjectError(importv2.IssueObjectFailed, stub.Id, fmt.Errorf("fetch database: %w", err))
			return &schemaFetch{issue: &issue}, nil
		}
		if len(database.DataSources) == 0 {
			issue := importv2.Warning(importv2.IssueDataLoss, stub.Id, "database exposes no data sources; skipped")
			return &schemaFetch{issue: &issue}, nil
		}
		schemaId = database.DataSources[0].Id
		c.dataSourcesByDatabase[stub.Id] = append(c.dataSourcesByDatabase[stub.Id], stub.Id)
	}
	var database databaseObject
	if err := c.client.Request(ctx, http.MethodGet, "/data_sources/"+schemaId, nil, &database); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("fetch data source: %w", err)
		}
		issue := importv2.ObjectError(importv2.IssueObjectFailed, stub.Id, fmt.Errorf("fetch data source: %w", err))
		return &schemaFetch{issue: &issue}, nil
	}
	return &schemaFetch{schemaId: schemaId, database: &database}, nil
}

// containerSchemas renders the prefetched schemas as planner evidence, in
// database stream order. The property filter mirrors suggestPageType exactly
// (title excluded, unsupported excluded, formula/rollup INCLUDED with their
// schema formats) so the naive planner reproduces the suggestor's verdicts
// verbatim.
func (c *Converter) containerSchemas() []schemaplan.ContainerSchema {
	var out []schemaplan.ContainerSchema
	for _, stub := range c.databases {
		fetch := c.schemaFetches[stub.Id]
		if fetch == nil || fetch.issue != nil {
			continue
		}
		names := make([]string, 0, len(fetch.database.Properties))
		for name := range fetch.database.Properties {
			names = append(names, name)
		}
		sort.Strings(names)
		schema := schemaplan.ContainerSchema{Id: stub.Id, Name: fetch.database.title()}
		for _, name := range names {
			property := fetch.database.Properties[name]
			if property.Type == "title" {
				continue
			}
			format, supported := relationFormatOf(property.Type)
			if !supported {
				continue
			}
			propertySchema := schemaplan.PropertySchema{Id: property.Id, Name: name, Format: format}
			for _, option := range schemaOptions(property) {
				propertySchema.Options = append(propertySchema.Options, option.Name)
			}
			schema.Properties = append(schema.Properties, propertySchema)
		}
		if c.includeSamples {
			schema.Samples = c.containerSamples(stub.Id, fetch.schemaId)
		}
		out = append(out, schema)
	}
	return out
}

// containerSamples shares a few member page titles as content evidence.
// Property values would need page fetches ahead of the plan and are not
// offered for Notion (the schema already carries option names).
func (c *Converter) containerSamples(entityId, schemaId string) *schemaplan.ContainerSamples {
	var titles []string
	for _, memberId := range c.databaseMembers(entityId, schemaId) {
		if len(titles) == sampleTitleCap {
			break
		}
		if title := c.entityById[memberId].Title; title != "" {
			titles = append(titles, title)
		}
	}
	if len(titles) == 0 {
		return nil
	}
	return &schemaplan.ContainerSamples{Titles: titles}
}

// emitPlanTypes emits the plan's referenced new types and the non-bundled
// relations their definitions name (definitions before use). The minted keys
// are what containers' pages reference in ObjectTypes.
func (c *Converter) emitPlanTypes(ctx context.Context, sink importv2.Sink) error {
	if len(c.plan.NewTypes) == 0 {
		return nil
	}
	referenced := map[string]bool{}
	for _, containerPlan := range c.plan.Containers {
		if containerPlan.TypeKey != "" {
			referenced[containerPlan.TypeKey.String()] = true
		}
	}
	for _, def := range c.plan.NewTypes {
		if !referenced[def.Key.String()] {
			continue
		}
		for _, prop := range def.Properties {
			if bundle.HasRelation(prop.Key) {
				continue
			}
			key := schemaplan.CustomRelationKey(prop.Key).String()
			name := prop.Name
			if name == "" {
				name = string(prop.Key)
			}
			format := prop.Format
			if format == 0 {
				format = model.RelationFormat_longtext
			}
			if added := c.properties.registerPlanDef(key, "relation:"+key, name, format); added {
				if err := sink.Object(ctx, schemaplan.RelationObject(prop.Key, prop.Name, prop.Format)); err != nil {
					return err
				}
			}
		}
		object, minted, err := schemaplan.TypeObject(def)
		if err != nil {
			sink.Issue(importv2.Warning(importv2.IssueLLMPlanEntryDropped, string(def.Key),
				fmt.Sprintf("plan type not emitted: %s", err)))
			continue
		}
		if err := sink.Object(ctx, object); err != nil {
			return err
		}
		c.planTypeKeys[def.Key] = minted
	}
	return nil
}

// applyPlanType is suggestPageType's plan-driven counterpart for containers
// the planner saw: it records the container's type verdict for pageTypeKey
// and surfaces it as the same typeSuggested issue.
func (c *Converter) applyPlanType(entityId, schemaId string, database *databaseObject, sink importv2.Sink) {
	containerPlan, ok := c.plan.Containers[entityId]
	if !ok || containerPlan.TypeKey == "" {
		return
	}
	typeKey := containerPlan.TypeKey
	if minted, ok := c.planTypeKeys[typeKey]; ok {
		typeKey = minted
	} else if !bundle.HasObjectTypeByKey(typeKey) {
		return // the plan type was dropped at emission; keep default Page
	}
	c.suggestedTypes[entityId] = typeKey
	c.suggestedTypes[schemaId] = typeKey
	sink.Issue(importv2.Info(importv2.IssueTypeSuggested,
		fmt.Sprintf("database %q pages imported as %s (%s)", database.title(), typeKey, containerPlan.Reason)))
}
