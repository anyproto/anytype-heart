package notion

import (
	"crypto/sha256"
	"encoding/hex"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// propertySchema is the shared shape of a property definition, whether it
// comes from a database schema or a page's property map.
type propertySchema struct {
	Id     string `json:"id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Select struct {
		Options []selectOption `json:"options"`
	} `json:"select"`
	MultiSelect struct {
		Options []selectOption `json:"options"`
	} `json:"multi_select"`
	Status struct {
		Options []selectOption `json:"options"`
	} `json:"status"`
}

type selectOption struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// relationDef is the converter's resident record of one imported relation.
type relationDef struct {
	key       string // anytype internal key (deterministic from notion id)
	sourceKey string // stream reference ("relation:<key>") or bundled URL
	format    model.RelationFormat
	name      string
	bundled   bool
}

// propertiesStore dedupes relations and options across the whole workspace
// (a property shared by a database and its pages yields one relation), and
// implements the v1 Tag redirection rules exactly:
// select/multi_select named "Tag" — or "Tags"/"tags" only when no "Tag"
// property exists — redirect to the bundled Tag relation; first match wins;
// status and people are never redirected.
type propertiesStore struct {
	byNotionId   map[string]*relationDef
	byNameFormat map[string]*relationDef
	// byKey dedupes by final anytype key — the identity plan targets share
	// (two containers remapping onto dueDate resolve to one def).
	byKey         map[string]*relationDef
	options       map[string]bool // option source key → emitted
	tagRedirected bool
	hasTagNamed   bool // a property named exactly "Tag" was seen
}

func newPropertiesStore() *propertiesStore {
	return &propertiesStore{
		byNotionId:   map[string]*relationDef{},
		byNameFormat: map[string]*relationDef{},
		byKey:        map[string]*relationDef{},
		options:      map[string]bool{},
	}
}

// resolveRelation returns the relation for a property, creating the
// definition on first sight. created=true means the caller must emit the
// relation object before using it.
func (p *propertiesStore) resolveRelation(property propertySchema) (def *relationDef, created bool) {
	format, ok := relationFormatOf(property.Type)
	if !ok {
		return nil, false
	}
	if def, ok := p.byNotionId[property.Id]; ok {
		return def, false
	}
	nameFormat := property.Name + "\x00" + format.String()
	if def, ok := p.byNameFormat[nameFormat]; ok {
		p.byNotionId[property.Id] = def
		return def, false
	}

	if p.isTagRedirect(property) {
		p.tagRedirected = true
		def = &relationDef{
			key:       bundle.RelationKeyTag.String(),
			sourceKey: bundle.RelationKeyTag.BundledURL(),
			format:    model.RelationFormat_tag,
			name:      property.Name,
			bundled:   true,
		}
		p.byNotionId[property.Id] = def
		p.byNameFormat[nameFormat] = def
		p.byKey[def.key] = def
		return def, false
	}

	key := "nprop" + shortHash(property.Id)
	def = &relationDef{
		key:       key,
		sourceKey: "relation:" + key,
		format:    format,
		name:      property.Name,
	}
	p.byNotionId[property.Id] = def
	p.byNameFormat[nameFormat] = def
	p.byKey[key] = def
	return def, true
}

// resolvePlanTarget resolves a property onto its schema-plan target: the
// bundled relation, or the plan's shared custom key. created follows the
// resolveRelation contract (the caller emits the relation object once).
func (p *propertiesStore) resolvePlanTarget(property propertySchema, plan schemaplan.PropertyPlan) (def *relationDef, created bool) {
	if def, ok := p.byNotionId[property.Id]; ok {
		return def, false
	}
	if bundle.HasRelation(plan.Key) {
		key := plan.Key.String()
		if def, ok := p.byKey[key]; ok {
			p.byNotionId[property.Id] = def
			return def, false
		}
		bundled := bundle.MustGetRelation(plan.Key)
		def = &relationDef{
			key:       key,
			sourceKey: plan.Key.BundledURL(),
			format:    bundled.Format,
			name:      bundled.Name,
			bundled:   true,
		}
		p.byKey[key] = def
		p.byNotionId[property.Id] = def
		return def, false
	}
	key := schemaplan.CustomRelationKey(plan.Key).String()
	if def, ok := p.byKey[key]; ok {
		p.byNotionId[property.Id] = def
		return def, false
	}
	name := plan.Name
	if name == "" {
		name = property.Name
	}
	format := plan.Format
	if format == 0 {
		format, _ = relationFormatOf(property.Type)
	}
	def = &relationDef{
		key:       key,
		sourceKey: "relation:" + key,
		format:    format,
		name:      name,
	}
	p.byKey[key] = def
	p.byNotionId[property.Id] = def
	return def, true
}

// registerPlanDef seeds a plan-minted relation def ahead of use (plan type
// definitions emit their relations before any container resolves them).
// Returns whether the def was new — i.e. whether the caller must emit it.
func (p *propertiesStore) registerPlanDef(key, sourceKey, name string, format model.RelationFormat) bool {
	if _, ok := p.byKey[key]; ok {
		return false
	}
	p.byKey[key] = &relationDef{key: key, sourceKey: sourceKey, format: format, name: name}
	return true
}

func (p *propertiesStore) isTagRedirect(property propertySchema) bool {
	if p.tagRedirected {
		return false // only the first match redirects
	}
	if property.Type != "select" && property.Type != "multi_select" {
		return false
	}
	switch property.Name {
	case "Tag":
		return true
	case "Tags", "tags":
		return !p.hasTagNamed
	default:
		return false
	}
}

// noteName tracks whether an exact "Tag" property exists anywhere, which
// blocks the "Tags"/"tags" fallback (call for every property before use).
func (p *propertiesStore) noteName(name string) {
	if name == "Tag" {
		p.hasTagNamed = true
	}
}

// resolveOption returns the option's source key and whether it still needs
// to be emitted (workspace-wide dedup by relation key + option name).
func (p *propertiesStore) resolveOption(relationKey, optionName string) (sourceKey string, created bool) {
	sourceKey = "option:" + relationKey + ":" + optionName
	if p.options[sourceKey] {
		return sourceKey, false
	}
	p.options[sourceKey] = true
	return sourceKey, true
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}

// relationFormatOf maps a property type to the anytype relation format
// (verified against the current API docs; v1 mapped schema phone_number to
// number). Unsupported types return false and are decided explicitly by the
// caller.
func relationFormatOf(propertyType string) (model.RelationFormat, bool) {
	switch propertyType {
	case "title":
		return model.RelationFormat_shorttext, true
	case "rich_text":
		return model.RelationFormat_longtext, true
	case "number":
		return model.RelationFormat_number, true
	case "select":
		// Pick-one cardinality preserved: status is anytype's single-select
		// format (decision §13.8 — v1 collapsed select into tag and the
		// choice was irreversibly multi-valued after import, GO-6345).
		return model.RelationFormat_status, true
	case "multi_select", "people":
		return model.RelationFormat_tag, true
	case "status":
		return model.RelationFormat_status, true
	case "date", "created_time", "last_edited_time":
		return model.RelationFormat_date, true
	case "created_by", "last_edited_by":
		return model.RelationFormat_shorttext, true
	case "checkbox":
		return model.RelationFormat_checkbox, true
	case "url":
		return model.RelationFormat_url, true
	case "email":
		return model.RelationFormat_email, true
	case "phone_number":
		return model.RelationFormat_phone, true
	case "files":
		return model.RelationFormat_file, true
	case "relation":
		return model.RelationFormat_object, true
	case "unique_id":
		return model.RelationFormat_longtext, true
	case "formula":
		return model.RelationFormat_shorttext, true
	case "rollup":
		return model.RelationFormat_longtext, true
	default:
		// verification and future types: deliberate skip with an issue at
		// the call site.
		return 0, false
	}
}

func relationObject(def *relationDef) *importv2.Object {
	uniqueKey, _ := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, def.key)
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:           domain.String(def.name),
		bundle.RelationKeyRelationKey:    domain.String(def.key),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(def.format)),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
	})
	if uniqueKey != nil {
		details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	}
	return &importv2.Object{
		SourceKey: def.sourceKey,
		SbType:    coresb.SmartBlockTypeRelation,
		Payload: &importv2.Snapshot{
			Key:         def.key,
			Details:     details,
			ObjectTypes: []string{bundle.TypeKeyRelation.String()},
		},
	}
}

func optionObject(relationKey, sourceKey, optionName, notionColor string) *importv2.Object {
	optionKey := "nopt" + shortHash(relationKey+"\x00"+optionName)
	uniqueKey, _ := domain.NewUniqueKey(coresb.SmartBlockTypeRelationOption, optionKey)
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:                domain.String(optionName),
		bundle.RelationKeyRelationKey:         domain.String(relationKey),
		bundle.RelationKeyRelationOptionColor: domain.String(anytypeColor(notionColor)),
		bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
	})
	if uniqueKey != nil {
		details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())
	}
	return &importv2.Object{
		SourceKey: sourceKey,
		SbType:    coresb.SmartBlockTypeRelationOption,
		Payload: &importv2.Snapshot{
			Key:         optionKey,
			Details:     details,
			ObjectTypes: []string{bundle.TypeKeyRelationOption.String()},
		},
	}
}

// anytypeColor maps notion option/text colors onto the anytype palette.
// brown has no anytype counterpart and maps to its nearest hue (approved
// data decision — v1 silently dropped it to the default).
func anytypeColor(notionColor string) string {
	switch notionColor {
	case "gray", "gray_background":
		return "grey"
	case "green", "green_background":
		return "lime"
	case "brown", "brown_background":
		return "orange"
	case "default", "default_background", "":
		return ""
	default:
		base := notionColor
		if idx := len(base) - len("_background"); idx > 0 && base[idx:] == "_background" {
			base = base[:idx]
		}
		return base
	}
}

func zeroValueOf(format model.RelationFormat) domain.Value {
	switch format {
	case model.RelationFormat_number:
		return domain.Int64(0)
	case model.RelationFormat_checkbox:
		return domain.Bool(false)
	case model.RelationFormat_tag, model.RelationFormat_status, model.RelationFormat_object, model.RelationFormat_file:
		return domain.StringList(nil)
	default:
		return domain.String("")
	}
}
