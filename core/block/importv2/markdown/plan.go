package markdown

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema/yaml"
)

const (
	// dirContainerPrefix distinguishes folder containers from csv containers
	// (whose ids are entry names) in the plan's id space.
	dirContainerPrefix = "dir:"
	sampleTitleCap     = 8
	optionSampleCap    = 20
)

// planStructure is the markdown plan phase: render
// the run's containers — csv collections under SuggestTypes profiles, folders
// under FolderContainers profiles — as planner evidence, plan once, sanitize,
// then surface type verdicts (in container order, where suggestCsvTypes used
// to speak) and emit the plan's new types.
func (c *Converter) planStructure(ctx context.Context, sink importv2.Sink) error {
	// The ANALYZING stage, bracketed UNCONDITIONALLY: a client that saw
	// the stage begin must always see it end, whatever the planner found to
	// do and however this step exits. Under an LLM planner this is the
	// 10-20 s of unexplained silence that has to be reported
	// and nothing ever did.
	sink.Phase(importv2.PhaseAnalyzing)
	defer sink.Phase(importv2.PhaseFetching)
	schemas := c.containerSchemas(ctx)
	// The sweep swallows read errors (page conversion reports them properly),
	// so a cancelled run must be recognized here, not misattributed later.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("plan structure: %w", err)
	}
	if len(schemas) == 0 {
		return nil
	}
	order := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		c.planned[schema.Id] = true
		order = append(order, schema.Id)
	}
	// The shared reuse rule (schemaplan.Resolve — the Notion sibling calls
	// the same function): a resumed crawl reuses the recorded plan verbatim,
	// a fresh run plans, degrades to naive on failure, sanitizes, records.
	plan, err := schemaplan.Resolve(ctx, c.params.PlanReuse, c.planner, schemas, sink.Issue)
	if err != nil {
		return err
	}
	c.plan = plan
	if err := c.emitPlanTypes(ctx, sink); err != nil {
		return err
	}
	c.applyPlanTypes(order, sink)
	return nil
}

// containerSchemas builds the planner evidence: csv collections carry
// name-only evidence (csv rows are never parsed — v1 parity), folders carry
// the union of their pages' front-matter schemas from a sweep over the
// re-readable source.
func (c *Converter) containerSchemas(ctx context.Context) []schemaplan.ContainerSchema {
	var out []schemaplan.ContainerSchema
	if c.flavour.SuggestTypes && c.flavour.CSVCollections {
		for _, entry := range c.csvEntries {
			if len(c.csvMembers(entry.Name)) == 0 {
				continue // no member pages: nothing to type
			}
			out = append(out, schemaplan.ContainerSchema{
				Id:   entry.Name,
				Name: notionPageTitle(entry.Name),
			})
		}
	}
	if c.flavour.FolderContainers {
		out = append(out, c.folderSchemas(ctx)...)
	}
	return out
}

// folderSchemas sweeps every folder's markdown front matter into one
// container schema per folder. Read or parse failures are skipped silently —
// page conversion reports them properly later.
func (c *Converter) folderSchemas(ctx context.Context) []schemaplan.ContainerSchema {
	byDir := map[string][]string{}
	var dirs []string
	for _, entry := range c.mdEntries {
		dir := path.Dir(entry.Name)
		if dir == "." {
			continue // the root mixes everything; only real folders are containers
		}
		if _, seen := byDir[dir]; !seen {
			dirs = append(dirs, dir)
		}
		byDir[dir] = append(byDir[dir], entry.Name)
	}
	sort.Strings(dirs)

	var out []schemaplan.ContainerSchema
	for _, dir := range dirs {
		schema := schemaplan.ContainerSchema{
			Id:   dirContainerPrefix + dir,
			Name: path.Base(dir),
		}
		properties := map[string]*schemaplan.PropertySchema{}
		var propertyOrder []string
		var titles []string
		for _, entryName := range byDir[dir] {
			if c.includeSamples() && len(titles) < sampleTitleCap {
				titles = append(titles, pageTitleFromPath(entryName))
			}
			c.sweepFrontMatter(ctx, entryName, properties, &propertyOrder)
		}
		sort.Strings(propertyOrder)
		for _, name := range propertyOrder {
			schema.Properties = append(schema.Properties, *properties[name])
		}
		if len(titles) > 0 {
			schema.Samples = &schemaplan.ContainerSamples{Titles: titles}
		}
		out = append(out, schema)
	}
	return out
}

// sweepFrontMatter merges one page's front-matter properties into the
// folder's union schema, using a throwaway resolver so the sweep leaves no
// trace in the run's real key/option state.
func (c *Converter) sweepFrontMatter(ctx context.Context, entryName string, properties map[string]*schemaplan.PropertySchema, order *[]string) {
	content, err := c.readEntry(ctx, entryName)
	if err != nil {
		return
	}
	frontMatter, _, err := yaml.ExtractYAMLFrontMatter(content)
	if err != nil || len(frontMatter) == 0 {
		return
	}
	sweep := newResolver(nil)
	parsed, err := yaml.ParseYAMLFrontMatterWithResolverAndPath(frontMatter, sweep, path.Dir(entryName))
	if err != nil || parsed == nil {
		return
	}
	optionsByKey := map[string][]string{}
	for _, option := range sweep.takePending() {
		optionsByKey[option.relationKey] = append(optionsByKey[option.relationKey], option.optionName)
	}
	for _, property := range parsed.Properties {
		if property.Name == "" || property.Name == "_collection" {
			continue
		}
		schema, seen := properties[property.Name]
		if !seen {
			schema = &schemaplan.PropertySchema{Id: property.Name, Name: property.Name, Format: property.Format}
			properties[property.Name] = schema
			*order = append(*order, property.Name)
		}
		for _, option := range optionsByKey[property.Key] {
			if len(schema.Options) >= optionSampleCap {
				break
			}
			if !containsString(schema.Options, option) {
				schema.Options = append(schema.Options, option)
			}
		}
	}
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// emitPlanTypes emits the plan's referenced new types with the non-bundled
// relations they name (definitions before use).
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
			if c.emittedRelations[key] {
				continue
			}
			c.emittedRelations[key] = true
			if err := sink.Object(ctx, schemaplan.RelationObject(prop.Key, prop.Name, prop.Format)); err != nil {
				return err
			}
		}
		object, minted, err := schemaplan.TypeObject(def)
		if err != nil {
			sink.Issue(importv2.Issue{
				Severity: importv2.SeverityWarning, Code: importv2.IssueLLMPlanEntryDropped, Subject: def.Name,
				Message: "A suggested object type could not be created; its folders were imported as collections",
				Err:     err,
			})
			continue
		}
		if err := sink.Object(ctx, object); err != nil {
			return err
		}
		c.planTypeKeys[def.Key] = minted
	}
	return nil
}

// applyPlanTypes surfaces every container's type verdict, in container order,
// and records it under the directory suggestedDirTypes serves page conversion
// from (the same fill-the-Page-gap application as before the plan phase).
func (c *Converter) applyPlanTypes(order []string, sink importv2.Sink) {
	for _, containerId := range order {
		containerPlan, ok := c.plan.Containers[containerId]
		if !ok || containerPlan.TypeKey == "" {
			continue
		}
		typeKey := containerPlan.TypeKey
		if minted, ok := c.planTypeKeys[typeKey]; ok {
			typeKey = minted
		} else if !bundle.HasObjectTypeByKey(typeKey) {
			continue // the plan type was dropped at emission; keep default Page
		}
		if dir, isDir := strings.CutPrefix(containerId, dirContainerPrefix); isDir {
			c.suggestedDirTypes[dir] = typeKey
			sink.Issue(importv2.Issue{
				Severity: importv2.SeverityInfo, Code: importv2.IssueTypeSuggested,
				Subject: fmt.Sprintf("%s → %s (%s)", path.Base(dir), typeKey, containerPlan.Reason),
				Message: "Pages of these folders were imported as an Anytype type",
			})
			continue
		}
		// csv container: id is the entry name, members live in its directory
		dir := strings.TrimSuffix(containerId, path.Ext(containerId))
		c.suggestedDirTypes[dir] = typeKey
		sink.Issue(importv2.Issue{
			Severity: importv2.SeverityInfo, Code: importv2.IssueTypeSuggested,
			Subject: fmt.Sprintf("%s → %s (%s)", notionPageTitle(containerId), typeKey, containerPlan.Reason),
			Message: "Rows of these collections were imported as an Anytype type",
		})
	}
}

// planRedirect is one property's plan target, resolved for page conversion:
// key redirects during yaml parsing (option values embed the key), name and
// format rewrite after.
type planRedirect struct {
	key     string
	name    string
	format  model.RelationFormat
	bundled bool
}

// planRedirectsFor resolves a folder's property remaps by front-matter name.
func (c *Converter) planRedirectsFor(dir string) map[string]planRedirect {
	containerPlan, ok := c.plan.Containers[dirContainerPrefix+dir]
	if !ok || len(containerPlan.Properties) == 0 {
		return nil
	}
	redirects := make(map[string]planRedirect, len(containerPlan.Properties))
	for name, propertyPlan := range containerPlan.Properties {
		if bundle.HasRelation(propertyPlan.Key) {
			bundled := bundle.MustGetRelation(propertyPlan.Key)
			redirects[name] = planRedirect{
				key:     propertyPlan.Key.String(),
				name:    bundled.Name,
				format:  bundled.Format,
				bundled: true,
			}
			continue
		}
		redirects[name] = planRedirect{
			key:    schemaplan.CustomRelationKey(propertyPlan.Key).String(),
			name:   propertyPlan.Name,
			format: propertyPlan.Format,
		}
	}
	return redirects
}

// applyPlanRedirects rewrites parsed front-matter properties onto their plan
// targets (the resolver already redirected the keys) and reports each
// adoption once per folder. A page whose value cannot carry the target's
// format — the plan was validated against the folder's UNION schema, and
// pages may disagree with it — degrades per page: scalar values revert to
// the original md property, option-list values drop (their option source
// keys already embed the target relation key).
func (c *Converter) applyPlanRedirects(dir string, properties []yaml.Property, redirects map[string]planRedirect, sink importv2.Sink) []yaml.Property {
	if len(redirects) == 0 {
		return properties
	}
	kept := properties[:0]
	for i := range properties {
		property := &properties[i]
		redirect, ok := redirects[property.Name]
		if !ok {
			kept = append(kept, *property)
			continue
		}
		sourceName := property.Name
		if redirect.format != 0 && !schemaplan.FormatChangeAllowed(property.Format, redirect.format) {
			c.reportOnce(dir, "drop\x00"+sourceName, sink, importv2.Warning(importv2.IssueLLMPlanEntryDropped, dir,
				fmt.Sprintf("folder %q property %q has values that do not fit %q (%s)",
					path.Base(dir), sourceName, redirect.key, redirect.format.String())))
			if listFormat(property.Format) {
				continue // value is option refs minted under the target key — unusable
			}
			property.Key = c.resolver.ResolvePropertyKey("", sourceName)
			kept = append(kept, *property)
			continue
		}
		if redirect.name != "" {
			property.Name = redirect.name
		}
		if redirect.format != 0 {
			property.Format = redirect.format
		}
		mapped := importv2.Issue{
			Severity: importv2.SeverityInfo, Code: importv2.IssuePropertyMapped,
			Subject: sourceName,
			Message: "Imported onto a property shared with the other folders that have it",
		}
		if bundle.HasRelation(domain.RelationKey(property.Key)) {
			mapped.Subject = fmt.Sprintf("%s → %s", sourceName, property.Name)
			mapped.Message = "Imported onto one of Anytype's built-in properties"
		}
		c.reportOnce(dir, sourceName, sink, mapped)
		kept = append(kept, *property)
	}
	return kept
}

func (c *Converter) reportOnce(dir, name string, sink importv2.Sink, issue importv2.Issue) {
	reportKey := dir + "\x00" + name
	if c.reportedMappings[reportKey] {
		return
	}
	c.reportedMappings[reportKey] = true
	sink.Issue(issue)
}

func listFormat(format model.RelationFormat) bool {
	return format == model.RelationFormat_status || format == model.RelationFormat_tag
}
