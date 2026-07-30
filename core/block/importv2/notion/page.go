package notion

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// pageObject is the GET /pages/{id} payload subset. Pages are re-fetched in
// pass 2 — search bodies are not retained, and a fresh fetch carries
// unexpired file URLs.
type pageObject struct {
	Id             string                   `json:"id"`
	Archived       bool                     `json:"archived"`
	Icon           *iconValue               `json:"icon"`
	Cover          *fileValue               `json:"cover"`
	CreatedTime    string                   `json:"created_time"`
	LastEditedTime string                   `json:"last_edited_time"`
	Properties     map[string]propertyValue `json:"properties"`
}

type propertyValue struct {
	Id             string         `json:"id"`
	Type           string         `json:"type"`
	Title          []richText     `json:"title"`
	RichText       []richText     `json:"rich_text"`
	Number         *float64       `json:"number"`
	Select         *selectOption  `json:"select"`
	MultiSelect    []selectOption `json:"multi_select"`
	Status         *selectOption  `json:"status"`
	Date           *dateValue     `json:"date"`
	People         []userValue    `json:"people"`
	Files          []fileValue    `json:"files"`
	Checkbox       bool           `json:"checkbox"`
	Url            *string        `json:"url"`
	Email          *string        `json:"email"`
	PhoneNumber    *string        `json:"phone_number"`
	Formula        *formulaValue  `json:"formula"`
	Relation       []idRef        `json:"relation"`
	HasMore        bool           `json:"has_more"`
	Rollup         *rollupValue   `json:"rollup"`
	UniqueId       *uniqueIdValue `json:"unique_id"`
	CreatedTime    string         `json:"created_time"`
	LastEditedTime string         `json:"last_edited_time"`
	CreatedBy      *userValue     `json:"created_by"`
	LastEditedBy   *userValue     `json:"last_edited_by"`
}

type userValue struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type idRef struct {
	Id string `json:"id"`
}

type formulaValue struct {
	Type    string     `json:"type"`
	String  *string    `json:"string"`
	Number  *float64   `json:"number"`
	Boolean *bool      `json:"boolean"`
	Date    *dateValue `json:"date"`
}

type rollupValue struct {
	Type   string          `json:"type"`
	Number *float64        `json:"number"`
	Date   *dateValue      `json:"date"`
	Array  []propertyValue `json:"array"`
}

type uniqueIdValue struct {
	Prefix *string `json:"prefix"`
	Number int64   `json:"number"`
}

// convertPage is the serial fetch+emit path (used by the discovery drain;
// the main loop pipelines fetches via prefetchPages instead).
func (c *Converter) convertPage(ctx context.Context, stub Entity, sink importv2.Sink) error {
	f := &fetchedPage{stub: stub}
	c.fetchPageData(ctx, f, sink)
	return c.emitFetchedPage(ctx, f, sink)
}

// emitFetchedPage is the converter-goroutine half of a page conversion:
// replays buffered fetch issues in page order, then maps and emits — all
// shared-state work (properties store, file registry, discovery) lives here.
func (c *Converter) emitFetchedPage(ctx context.Context, f *fetchedPage, sink importv2.Sink) error {
	for _, issue := range f.issues {
		sink.Issue(issue)
	}
	stub := f.stub
	if f.pageErr != nil {
		if ctx.Err() != nil {
			// Cancellation must stop the run, not degrade into one bogus
			// per-object failure per remaining entity.
			return fmt.Errorf("fetch page: %w", f.pageErr)
		}
		sink.Issue(importv2.ObjectError(importv2.IssueObjectFailed, stub.Id, fmt.Errorf("fetch page: %w", f.pageErr)))
		return nil
	}
	page := f.page

	details := domain.NewDetails()
	details.SetString(bundle.RelationKeySourceFilePath, stub.Id)
	setTimestamps(details, page.CreatedTime, page.LastEditedTime)

	if err := c.convertProperties(ctx, stub.Id, page.Properties, details, sink); err != nil {
		return err
	}

	blockIds := f.blockIds
	if blockIds == nil {
		blockIds = map[string]struct{}{}
	}
	blocks := f.blocks
	if f.blocksErr != nil {
		if ctx.Err() != nil {
			return f.blocksErr
		}
		// The page's properties and title are already in hand — import them
		// with a placeholder body instead of dropping the whole page.
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, stub.Id, fmt.Sprintf("page content could not be fetched: %s", f.blocksErr)))
		blocks = []notionBlock{{Id: stub.Id + "-lostcontent", Type: "unreadable"}}
	}
	modelBlocks, err := c.mapBlocks(ctx, mapContext{pageId: stub.Id, blockIds: blockIds}, blocks, sink)
	if err != nil {
		return err
	}

	object := &importv2.Object{
		SourceKey: stub.Id,
		SbType:    coresb.SmartBlockTypePage,
		Payload: &importv2.Snapshot{
			Blocks:      flattenBlocks(modelBlocks),
			Details:     details,
			ObjectTypes: []string{c.pageTypeKey(stub)},
		},
		IsRootCandidate: c.isRootCandidate(stub),
		Archived:        page.Archived,
	}
	// Root children are the top-level mapped blocks; persist adds the root.
	if err := c.applyIcon(ctx, object, page.Icon, page.Cover, "/pages/"+stub.Id, sink); err != nil {
		return err
	}
	return sink.Object(ctx, object)
}

// pageTypeKey returns the type suggested for the page's parent database
// (§11.5), Page otherwise. Databases convert before pages, so suggestions
// are complete by the time any row asks.
func (c *Converter) pageTypeKey(stub Entity) string {
	var parentId string
	switch stub.Parent.Type {
	case "data_source_id":
		parentId = stub.Parent.DataSourceId
	case "database_id":
		parentId = stub.Parent.DatabaseId
	}
	if typeKey, ok := c.suggestedTypes[parentId]; ok {
		return typeKey.String()
	}
	return bundle.TypeKeyPage.String()
}

// convertProperties maps every property value to a detail, emitting new
// relation/option definitions first (shared store — a property seen on a
// database and its pages is one relation).
func (c *Converter) convertProperties(ctx context.Context, pageId string, properties map[string]propertyValue, details *domain.Details, sink importv2.Sink) error {
	names := make([]string, 0, len(properties))
	for name := range properties {
		c.properties.noteName(name)
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		value := properties[name]
		if value.Type == "title" {
			details.SetString(bundle.RelationKeyName, plainText(value.Title))
			continue
		}
		if value.Type == "verification" {
			sink.Issue(importv2.Warning(importv2.IssueDataLoss, pageId,
				fmt.Sprintf("property %q (verification) has no anytype counterpart and was skipped", name)))
			continue
		}
		// Page-side definitions carry no plan target: schema-declared
		// properties were already resolved (and possibly remapped) by their
		// database, and the rest are value-typed formula/rollup or orphan-page
		// properties the plan does not cover (docs/ImportV2LLM.md §4).
		def, err := c.emitProperty(ctx, propertySchema{
			Id:   value.Id,
			Type: effectivePropertyType(value),
			Name: name,
		}, schemaplan.PropertyPlan{}, "", sink)
		if err != nil || def == nil {
			if err != nil {
				return err
			}
			continue
		}
		detailValue, companion, err := c.propertyDetail(ctx, pageId, name, value, def, sink)
		if err != nil {
			return err
		}
		if detailValue.Ok() {
			details.Set(domain.RelationKey(def.key), detailValue)
		}
		if companion != nil {
			details.Set(domain.RelationKey(companion.def.key), companion.value)
		}
	}
	return nil
}

// effectivePropertyType refines formula/rollup by their value type so the
// relation format matches the data (a date formula becomes a date relation —
// approved decision; v1 dropped date formulas).
func effectivePropertyType(value propertyValue) string {
	switch value.Type {
	case "formula":
		if value.Formula == nil {
			return "formula"
		}
		switch value.Formula.Type {
		case "date":
			return "date"
		case "number":
			return "number"
		case "boolean":
			return "checkbox"
		default:
			return "rich_text"
		}
	case "rollup":
		if value.Rollup == nil {
			return "rollup"
		}
		switch value.Rollup.Type {
		case "number":
			return "number"
		case "date":
			return "date"
		default:
			return "rollup" // arrays: joined longtext, lossy by decision
		}
	default:
		return value.Type
	}
}

type companionDetail struct {
	def   *relationDef
	value domain.Value
}

// propertyDetail converts one property value. The companion return carries
// the "<name> (end)" date relation for ranges (approved decision; v1
// dropped the end date).
func (c *Converter) propertyDetail(ctx context.Context, pageId, name string, value propertyValue, def *relationDef, sink importv2.Sink) (domain.Value, *companionDetail, error) {
	switch value.Type {
	case "rich_text":
		runs, err := c.completeRichText(ctx, pageId, name, value, sink)
		if err != nil {
			return domain.Invalid(), nil, err
		}
		return domain.String(plainText(runs)), nil, nil
	case "number":
		if value.Number == nil {
			return domain.Invalid(), nil, nil
		}
		return domain.Float64(*value.Number), nil, nil
	case "select":
		if value.Select == nil {
			return domain.Invalid(), nil, nil
		}
		keys, err := c.optionKeys(ctx, def, []selectOption{*value.Select}, sink)
		return domain.StringList(keys), nil, err
	case "multi_select":
		keys, err := c.optionKeys(ctx, def, value.MultiSelect, sink)
		return domain.StringList(keys), nil, err
	case "status":
		if value.Status == nil {
			return domain.Invalid(), nil, nil
		}
		keys, err := c.optionKeys(ctx, def, []selectOption{*value.Status}, sink)
		return domain.StringList(keys), nil, err
	case "people":
		people, err := c.completePeople(ctx, pageId, name, value, sink)
		if err != nil {
			return domain.Invalid(), nil, err
		}
		options := make([]selectOption, 0, len(people))
		for _, person := range people {
			if person.Name == "" {
				// v1 left the raw notion user id dangling in the detail.
				sink.Issue(importv2.Warning(importv2.IssueDataLoss, pageId,
					fmt.Sprintf("property %q references an unresolvable user; skipped", name)))
				continue
			}
			options = append(options, selectOption{Name: person.Name})
		}
		keys, err := c.optionKeys(ctx, def, options, sink)
		return domain.StringList(keys), nil, err
	case "date":
		return c.dateDetail(ctx, pageId, name, value.Date, sink)
	case "created_time":
		return c.timeDetail(value.CreatedTime), nil, nil
	case "checkbox":
		return domain.Bool(value.Checkbox), nil, nil
	case "url":
		return optionalString(value.Url), nil, nil
	case "email":
		return optionalString(value.Email), nil, nil
	case "phone_number":
		return optionalString(value.PhoneNumber), nil, nil
	case "files":
		keys := make([]string, 0, len(value.Files))
		for _, file := range value.Files {
			if file.url() == "" {
				continue
			}
			// Property files carry no per-file refresh source (re-reading the
			// whole page to match a file by name is not worth it) — nil skips
			// the expired-URL retry.
			sourceKey, err := c.emitFileFromUrl(ctx, sink, file.url(), file.Name, file.isExternal(), nil)
			if err != nil {
				return domain.Invalid(), nil, err
			}
			keys = append(keys, sourceKey)
		}
		return domain.StringList(keys), nil, nil
	case "relation":
		refs, err := c.completeRelationRefs(ctx, pageId, name, value, sink)
		if err != nil {
			return domain.Invalid(), nil, err
		}
		ids := make([]string, 0, len(refs))
		for _, ref := range refs {
			ids = append(ids, ref.Id)
		}
		return domain.StringList(ids), nil, nil
	case "formula":
		return c.formulaDetail(pageId, name, value.Formula, sink)
	case "rollup":
		return c.rollupDetail(pageId, name, value.Rollup, sink)
	case "unique_id":
		if value.UniqueId == nil {
			return domain.Invalid(), nil, nil
		}
		if value.UniqueId.Prefix != nil {
			return domain.String(fmt.Sprintf("%s-%d", *value.UniqueId.Prefix, value.UniqueId.Number)), nil, nil
		}
		return domain.String(fmt.Sprintf("%d", value.UniqueId.Number)), nil, nil
	case "created_by":
		return userName(value.CreatedBy), nil, nil
	case "last_edited_by":
		return userName(value.LastEditedBy), nil, nil
	case "last_edited_time":
		return c.timeDetail(value.LastEditedTime), nil, nil
	default:
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, pageId,
			fmt.Sprintf("property %q of type %q was skipped", name, value.Type)))
		return domain.Invalid(), nil, nil
	}
}

func (c *Converter) dateDetail(ctx context.Context, pageId, name string, date *dateValue, sink importv2.Sink) (domain.Value, *companionDetail, error) {
	if date == nil || date.Start == "" {
		return domain.Invalid(), nil, nil
	}
	start, _, err := parseNotionDate(date.Start, date.TimeZone)
	if err != nil {
		// v1 imported malformed dates as epoch 0.
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, pageId,
			fmt.Sprintf("property %q: %s; value omitted", name, err)))
		return domain.Invalid(), nil, nil
	}
	var companion *companionDetail
	if date.End != "" {
		if end, _, err := parseNotionDate(date.End, date.TimeZone); err == nil {
			endDef, created := c.properties.resolveRelation(propertySchema{
				Id:   "end:" + name,
				Type: "date",
				Name: name + " (end)",
			})
			if created {
				if err := sink.Object(ctx, relationObject(endDef)); err != nil {
					return domain.Invalid(), nil, err
				}
			}
			companion = &companionDetail{def: endDef, value: domain.Int64(end)}
		} else {
			sink.Issue(importv2.Warning(importv2.IssueDataLoss, pageId,
				fmt.Sprintf("property %q: unparseable range end; dropped", name)))
		}
	}
	return domain.Int64(start), companion, nil
}

func (c *Converter) timeDetail(raw string) domain.Value {
	if start, _, err := parseNotionDate(raw, ""); err == nil {
		return domain.Int64(start)
	}
	return domain.Invalid()
}

func (c *Converter) formulaDetail(pageId, name string, formula *formulaValue, sink importv2.Sink) (domain.Value, *companionDetail, error) {
	if formula == nil {
		return domain.Invalid(), nil, nil
	}
	switch formula.Type {
	case "string":
		return optionalString(formula.String), nil, nil
	case "number":
		if formula.Number == nil {
			return domain.Invalid(), nil, nil
		}
		return domain.Float64(*formula.Number), nil, nil
	case "boolean":
		if formula.Boolean == nil {
			return domain.Invalid(), nil, nil
		}
		return domain.Bool(*formula.Boolean), nil, nil
	case "date":
		if formula.Date == nil || formula.Date.Start == "" {
			return domain.Invalid(), nil, nil
		}
		if start, _, err := parseNotionDate(formula.Date.Start, formula.Date.TimeZone); err == nil {
			return domain.Int64(start), nil, nil
		}
		return domain.Invalid(), nil, nil
	default:
		return domain.Invalid(), nil, nil
	}
}

// rollupDetail: scalars keep their type; arrays flatten to joined text with
// a warning (approved decision — v1 produced tag values backed by nothing).
func (c *Converter) rollupDetail(pageId, name string, rollup *rollupValue, sink importv2.Sink) (domain.Value, *companionDetail, error) {
	if rollup == nil {
		return domain.Invalid(), nil, nil
	}
	switch rollup.Type {
	case "number":
		if rollup.Number == nil {
			return domain.Invalid(), nil, nil
		}
		return domain.Float64(*rollup.Number), nil, nil
	case "date":
		if rollup.Date == nil || rollup.Date.Start == "" {
			return domain.Invalid(), nil, nil
		}
		if start, _, err := parseNotionDate(rollup.Date.Start, rollup.Date.TimeZone); err == nil {
			return domain.Int64(start), nil, nil
		}
		return domain.Invalid(), nil, nil
	case "incomplete":
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, pageId,
			fmt.Sprintf("rollup %q aggregates more than 25 relations and was returned incomplete; value omitted", name)))
		return domain.Invalid(), nil, nil
	case "unsupported":
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, pageId,
			fmt.Sprintf("rollup %q uses an aggregation the API does not expose; value omitted", name)))
		return domain.Invalid(), nil, nil
	case "array":
		parts := make([]string, 0, len(rollup.Array))
		for _, item := range rollup.Array {
			if text := scalarText(item); text != "" {
				parts = append(parts, text)
			}
		}
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, pageId,
			fmt.Sprintf("rollup %q flattened to text (arrays have no anytype counterpart)", name)))
		return domain.String(strings.Join(parts, ", ")), nil, nil
	default:
		return domain.Invalid(), nil, nil
	}
}

func scalarText(value propertyValue) string {
	switch value.Type {
	case "title":
		return plainText(value.Title)
	case "rich_text":
		return plainText(value.RichText)
	case "number":
		if value.Number != nil {
			return trimFloat(*value.Number)
		}
	case "checkbox":
		return fmt.Sprintf("%t", value.Checkbox)
	case "select":
		if value.Select != nil {
			return value.Select.Name
		}
	case "status":
		if value.Status != nil {
			return value.Status.Name
		}
	case "multi_select":
		names := make([]string, 0, len(value.MultiSelect))
		for _, option := range value.MultiSelect {
			names = append(names, option.Name)
		}
		return strings.Join(names, ", ")
	case "people":
		names := make([]string, 0, len(value.People))
		for _, person := range value.People {
			if person.Name != "" {
				names = append(names, person.Name)
			}
		}
		return strings.Join(names, ", ")
	case "date":
		if value.Date != nil {
			if value.Date.End != "" {
				return value.Date.Start + " → " + value.Date.End
			}
			return value.Date.Start
		}
	case "formula":
		if value.Formula != nil {
			switch value.Formula.Type {
			case "string":
				if value.Formula.String != nil {
					return *value.Formula.String
				}
			case "number":
				if value.Formula.Number != nil {
					return trimFloat(*value.Formula.Number)
				}
			case "boolean":
				if value.Formula.Boolean != nil {
					return fmt.Sprintf("%t", *value.Formula.Boolean)
				}
			}
		}
	}
	return ""
}

func trimFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", f), "0"), ".")
}

// optionKeys resolves select/status/people values to option source keys,
// emitting options unseen so far (page values may extend the schema set).
func (c *Converter) optionKeys(ctx context.Context, def *relationDef, options []selectOption, sink importv2.Sink) ([]string, error) {
	keys := make([]string, 0, len(options))
	for _, option := range options {
		if option.Name == "" {
			continue
		}
		sourceKey, created := c.properties.resolveOption(def.key, option.Name)
		if created {
			if err := sink.Object(ctx, optionObject(def.key, sourceKey, option.Name, option.Color)); err != nil {
				return nil, err
			}
		}
		keys = append(keys, sourceKey)
	}
	return keys, nil
}

func optionalString(value *string) domain.Value {
	if value == nil || *value == "" {
		return domain.Invalid()
	}
	return domain.String(*value)
}

func userName(user *userValue) domain.Value {
	if user == nil || user.Name == "" {
		return domain.Invalid()
	}
	return domain.String(user.Name)
}

// propertyItemsThreshold is the page-object truncation point documented by
// the API: rich_text/people/relation values with 25 items may be truncated
// and need the property-item endpoint.
const propertyItemsThreshold = 25

func (c *Converter) completeRichText(ctx context.Context, pageId, name string, value propertyValue, sink importv2.Sink) ([]richText, error) {
	if len(value.RichText) < propertyItemsThreshold {
		return value.RichText, nil
	}
	items, err := c.fetchPropertyItems(ctx, pageId, value.Id)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		sink.Issue(truncationWarning(pageId, name, err))
		return value.RichText, nil
	}
	runs := make([]richText, 0, len(items))
	for _, item := range items {
		runs = append(runs, item.RichText)
	}
	return runs, nil
}

func (c *Converter) completeRelationRefs(ctx context.Context, pageId, name string, value propertyValue, sink importv2.Sink) ([]idRef, error) {
	if !value.HasMore && len(value.Relation) < propertyItemsThreshold {
		return value.Relation, nil
	}
	items, err := c.fetchPropertyItems(ctx, pageId, value.Id)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		sink.Issue(truncationWarning(pageId, name, err))
		return value.Relation, nil
	}
	refs := make([]idRef, 0, len(items))
	for _, item := range items {
		refs = append(refs, item.Relation)
	}
	return refs, nil
}

// truncationWarning reports a failed >25-item property completion: the
// truncated prefix from the page object is kept, but never silently.
func truncationWarning(pageId, name string, err error) importv2.Issue {
	return importv2.Warning(importv2.IssueDataLoss, pageId,
		fmt.Sprintf("property %q: completing the value past %d items failed (%s); the truncated value was kept", name, propertyItemsThreshold, err))
}

type propertyItem struct {
	Type     string    `json:"type"`
	RichText richText  `json:"rich_text"`
	Relation idRef     `json:"relation"`
	People   userValue `json:"people"`
}

// completePeople de-truncates a people property past the page object's
// 25-item cap via the property-item endpoint.
func (c *Converter) completePeople(ctx context.Context, pageId, name string, value propertyValue, sink importv2.Sink) ([]userValue, error) {
	if len(value.People) < propertyItemsThreshold {
		return value.People, nil
	}
	items, err := c.fetchPropertyItems(ctx, pageId, value.Id)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		sink.Issue(truncationWarning(pageId, name, err))
		return value.People, nil
	}
	people := make([]userValue, 0, len(items))
	for _, item := range items {
		people = append(people, item.People)
	}
	return people, nil
}

type propertyItemsResponse struct {
	Results    []propertyItem `json:"results"`
	HasMore    bool           `json:"has_more"`
	NextCursor *string        `json:"next_cursor"`
}

// fetchPropertyItems paginates the property-item endpoint (bounded by the
// client's retry policy per request; v1 retried these forever).
func (c *Converter) fetchPropertyItems(ctx context.Context, pageId, propertyId string) ([]propertyItem, error) {
	var items []propertyItem
	cursor := ""
	for {
		// Property ids arrive already percent-encoded in page JSON (e.g.
		// "qK%7C%5E") and the endpoint expects them verbatim; escaping again
		// would double-encode the '%' and address a nonexistent property.
		path := fmt.Sprintf("/pages/%s/properties/%s?page_size=100", pageId, propertyId)
		if cursor != "" {
			// Cursors are opaque: escape rather than assume URL-safety.
			path += "&start_cursor=" + url.QueryEscape(cursor)
		}
		var response propertyItemsResponse
		if err := c.client.Request(ctx, http.MethodGet, path, nil, &response); err != nil {
			return nil, fmt.Errorf("fetch property items: %w", err)
		}
		items = append(items, response.Results...)
		if !response.HasMore {
			return items, nil
		}
		if response.NextCursor == nil || *response.NextCursor == "" {
			return nil, fmt.Errorf("property items: has_more with empty next_cursor")
		}
		cursor = *response.NextCursor
	}
}

// flattenBlocks turns the mapped tree into the flat block list a snapshot
// carries, preserving parent/child wiring via ChildrenIds.
func flattenBlocks(tree []*mappedBlock) []*model.Block {
	var flat []*model.Block
	var walk func(nodes []*mappedBlock) []string
	walk = func(nodes []*mappedBlock) []string {
		ids := make([]string, 0, len(nodes))
		for _, node := range nodes {
			node.block.ChildrenIds = walk(node.children)
			flat = append(flat, node.block)
			ids = append(ids, node.block.Id)
		}
		return ids
	}
	walk(tree)
	return flat
}
