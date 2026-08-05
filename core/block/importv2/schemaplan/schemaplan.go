// Package schemaplan models the whole-workspace structure plan an import run
// consults while converting (docs/ImportV2LLM.md §4): which object type each
// container's pages get, and how source properties normalize onto bundled or
// shared relations. Plans come from a Planner — the LLM implementation, the
// naive wrapper over typesuggest, or a scripted one in tests — and are
// sanitized before converters trust a single entry.
package schemaplan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ContainerSchema is everything a converter shares about one container of
// pages (a Notion data source, a csv collection, a vault folder).
type ContainerSchema struct {
	// Id is converter-scoped and opaque to the planner; the plan echoes it.
	Id         string
	Name       string // id-stripped title
	Properties []PropertySchema
	// Samples is nil unless the import request opted into content samples.
	Samples *ContainerSamples
}

// PropertySchema is one schema property as the source sees it.
type PropertySchema struct {
	Id      string // source property id (notion property id, frontmatter key)
	Name    string
	Format  model.RelationFormat
	Options []string // select option names, when any
}

// ContainerSamples carries the optional content evidence.
type ContainerSamples struct {
	Titles []string            // a few page titles
	Values map[string][]string // property id → a few distinct rendered values
}

// Plan is the whole-run verdict. Zero value = no plan (import runs as today).
type Plan struct {
	Containers map[string]ContainerPlan // by ContainerSchema.Id
	NewTypes   []TypeDefinition
}

// ContainerPlan types one container and normalizes its properties.
type ContainerPlan struct {
	// TypeKey is a bundled type key or the key of a Plan.NewTypes entry;
	// empty leaves the container's pages on their default type.
	TypeKey domain.TypeKey
	// Reason is the short human phrase for the typeSuggested issue
	// ("container name", "LLM plan").
	Reason string
	// Properties remaps source properties by PropertySchema.Id; a property
	// absent here imports unchanged.
	Properties map[string]PropertyPlan
}

// PropertyPlan redirects one source property onto a target relation.
type PropertyPlan struct {
	// Key is the target: a bundled relation key, or a shared plan key that
	// CustomRelationKey mints identically in every container that names it —
	// that identity is what makes cross-container merges work.
	Key domain.RelationKey
	// Name overrides the display name for non-bundled targets ("" = keep).
	Name string
	// Format overrides the relation format for non-bundled targets
	// (0 = keep the source format). Bundled targets always keep the bundled
	// format.
	Format model.RelationFormat
}

// TypeDefinition is a new type the plan introduces, in anyblockjson §2a shape.
type TypeDefinition struct {
	Key    domain.TypeKey // plan-scoped key; CustomTypeKey mints the emitted one
	Name   string
	Layout model.ObjectTypeLayout
	// PluralName labels collections of this type ("Tasks"); empty keeps Name.
	PluralName string
	// IconName is a member of the closed icon vocabulary
	// (core/api/model.IconName); anything else is dropped by Sanitize.
	IconName   string
	Properties []TypeProperty
}

// TypeProperty is one recommended property of a new type.
type TypeProperty struct {
	Key      domain.RelationKey // bundled or plan key, same resolution as PropertyPlan.Key
	Name     string
	Format   model.RelationFormat
	Featured bool
}

// Planner produces a plan from the run's container schemas. Implementations
// must be deterministic for identical schemas (typesuggest seam rules) and
// must respect ctx — the engine's run context.
type Planner interface {
	Plan(ctx context.Context, schemas []ContainerSchema) (Plan, error)
}

// PlannerFunc adapts a function to Planner (scripted planners in tests).
type PlannerFunc func(ctx context.Context, schemas []ContainerSchema) (Plan, error)

func (f PlannerFunc) Plan(ctx context.Context, schemas []ContainerSchema) (Plan, error) {
	return f(ctx, schemas)
}

// CustomRelationKey mints the emitted relation key for a non-bundled plan
// key. Deterministic and converter-agnostic, so the same plan key merges into
// one relation across containers and across sources.
func CustomRelationKey(planKey domain.RelationKey) domain.RelationKey {
	return domain.RelationKey("aiprop" + shortHash(string(planKey)))
}

// CustomTypeKey mints the emitted type key for a plan-defined type.
func CustomTypeKey(planKey domain.TypeKey) domain.TypeKey {
	return domain.TypeKey("aitype" + shortHash(string(planKey)))
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}
