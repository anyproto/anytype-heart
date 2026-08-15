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
	// The §15 ANALYZING stage, bracketed UNCONDITIONALLY (the markdown
	// sibling does the same): schema prefetch plus the planner call is the
	// 10-20 s of unexplained silence ImportV2LLM.md §3 specified reporting
	// and nothing ever did, and a client that saw the stage begin must
	// always see it end — on the error paths too.
	sink.Phase(importv2.PhaseAnalyzing)
	defer sink.Phase(importv2.PhaseFetching)
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
	// Resolve is the shared reuse rule (schemaplan.Reuse): a resumed crawl
	// gets the recorded plan back verbatim; a fresh run plans, degrades to
	// naive on failure, sanitizes and records. The `planned` marking above
	// covers THIS incarnation's schema list either way: a container first
	// discovered on a resumed session is absent from the recorded plan and
	// imports on the default type — conservative, deterministic, and
	// consistent with containers the recorded plan deliberately declined.
	plan, err := schemaplan.Resolve(ctx, c.planReuse, c.planner, schemas, sink.Issue)
	if err != nil {
		return err
	}
	c.plan = plan
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
	// soleContainer is the one database backing a type, when exactly one does.
	// Such a type and a collection over it would be the same list, so the type
	// takes the database's place: its source key, so links and mentions to the
	// database keep resolving, its icon and description, and its spot in the
	// import root. Types shared by several databases keep their collections —
	// there the collection is what tells the lists apart.
	containerCount := map[string]int{}
	soleContainer := map[string]string{}
	for containerId, containerPlan := range c.plan.Containers {
		if containerPlan.TypeKey == "" {
			continue
		}
		key := containerPlan.TypeKey.String()
		referenced[key] = true
		containerCount[key]++
		soleContainer[key] = containerId
	}
	for key, count := range containerCount {
		if count > 1 {
			delete(soleContainer, key)
		}
	}
	// A page carries exactly one type, so containers that share members cannot
	// both be represented by types — whichever type the page did not take
	// would lose that member with no collection left to record it. Notion
	// reaches this whenever search returns both a database stub and its own
	// data source: databaseMembers matches the pages under either id.
	claimsPerMember := map[string]int{}
	for _, stub := range c.databases {
		if fetch := c.schemaFetches[stub.Id]; fetch != nil && fetch.database != nil {
			for _, member := range c.databaseMembers(stub.Id, fetch.schemaId) {
				claimsPerMember[member]++
			}
		}
	}
	for key, containerId := range soleContainer {
		fetch := c.schemaFetches[containerId]
		if fetch == nil || fetch.database == nil {
			delete(soleContainer, key)
			continue
		}
		for _, member := range c.databaseMembers(containerId, fetch.schemaId) {
			if claimsPerMember[member] > 1 {
				delete(soleContainer, key)
				break
			}
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
		c.planTypeKeys[def.Key] = minted
		if containerId, sole := soleContainer[def.Key.String()]; sole {
			// Hold this one back. It replaces the container's collection, and
			// the collection's job was to surface EVERY schema property — so
			// the type must list the database's other relations too, and those
			// only exist once convertDatabase has emitted them. References are
			// resolved per object against the identity index (ResolveRef
			// reports an unknown key as missing rather than waiting), so a type
			// naming a relation emitted later would degrade to missingTarget.
			c.typeBackedContainers[containerId] = true
			c.deferredTypes[containerId] = def
			continue
		}
		if err := sink.Object(ctx, object); err != nil {
			return err
		}
	}
	return nil
}

// emitDeferredType emits the type that replaces a database's collection, once
// that database's own relations exist. schemaDefs are every relation the
// schema produced; the ones the plan did not already name are added as
// regular recommended relations, so no imported property goes unlisted just
// because the model did not enumerate it.
func (c *Converter) emitDeferredType(ctx context.Context, stub Entity, schemaDefs []*relationDef, sink importv2.Sink) error {
	def, ok := c.deferredTypes[stub.Id]
	if !ok {
		return nil
	}
	object, _, err := schemaplan.TypeObject(def)
	if err != nil {
		sink.Issue(importv2.Warning(importv2.IssueLLMPlanEntryDropped, string(def.Key),
			fmt.Sprintf("plan type not emitted: %s", err)))
		return nil
	}
	listed := map[string]bool{}
	for _, ref := range object.Payload.Details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations) {
		listed[ref] = true
	}
	regular := object.Payload.Details.GetStringList(bundle.RelationKeyRecommendedRelations)
	for _, ref := range regular {
		listed[ref] = true
	}
	for _, schemaDef := range schemaDefs {
		if listed[schemaDef.sourceKey] {
			continue
		}
		listed[schemaDef.sourceKey] = true
		regular = append(regular, schemaDef.sourceKey)
	}
	if len(regular) > 0 {
		object.Payload.Details.SetStringList(bundle.RelationKeyRecommendedRelations, regular)
	}
	if err := c.adoptDatabaseIdentity(ctx, object, stub.Id, sink); err != nil {
		return err
	}
	return sink.Object(ctx, object)
}

// adoptDatabaseIdentity hands a database's identity to the type that replaces
// it. The type keeps its own name — the model named the kind, which reads
// better than the database's title — but takes the source key so existing
// references resolve, plus the database's description, timestamps, icon and
// root candidacy. convertDatabase then skips the collection.
func (c *Converter) adoptDatabaseIdentity(ctx context.Context, object *importv2.Object, containerId string, sink importv2.Sink) error {
	fetch := c.schemaFetches[containerId]
	if fetch == nil || fetch.database == nil {
		return nil
	}
	stub, ok := c.entityById[containerId]
	if !ok {
		return nil
	}
	c.typeBackedContainers[containerId] = true

	database := fetch.database
	object.SourceKey = containerId
	object.IsRootCandidate = c.isRootCandidate(stub)
	// Deliberately NOT archived with the database. A collection in the bin is
	// recoverable content, but a type in the bin is still the shape of every
	// live row that carries it, and emptying the bin would take the shape with
	// it. The rows of an archived database are archived individually anyway.
	if description := plainText(database.Description); description != "" {
		object.Payload.Details.SetString(bundle.RelationKeyDescription, description)
	}
	object.Payload.Details.SetString(bundle.RelationKeySourceFilePath, containerId)
	setTimestamps(object.Payload.Details, database.CreatedTime, database.LastEditedTime)
	if database.Icon == nil && database.Cover == nil {
		// Nothing to inherit; keep the icon the plan chose.
		return nil
	}
	// The database's own icon is the more faithful one, and two icon details on
	// one object leave the rendered choice up to the client.
	object.Payload.Details.Delete(bundle.RelationKeyIconName)
	return c.applyIcon(ctx, object, database.Icon, database.Cover, "/data_sources/"+fetch.schemaId, sink)
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
