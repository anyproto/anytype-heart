package service

// v2_object.go implements the Phase-1 object read surface (APIV2.md):
// GET /v2/spaces/{spaceId}/objects/{objectId} with include/outline/block/
// ids/format, and the C5 minimal-row object list.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

// Query parameter values (APIV2.md Phase 1).
const (
	V2IdsCompact = "compact"
	V2IdsFull    = "full"

	V2FormatAnyblock = "anyblock"
	V2FormatMd       = "md"

	V2IncludeProperties = "properties"
	V2IncludeBlocks     = "blocks"
)

// V2ObjectQuery carries the GET object query parameters as received.
type V2ObjectQuery struct {
	Include string // comma-separated subset of properties,blocks; "" = both
	Outline bool
	Block   string
	Ids     string // compact (default) | full — object ids only (C4)
	Format  string // anyblock (default) | md
}

// objectReadPlan is the validated form of a V2ObjectQuery.
type objectReadPlan struct {
	wantProperties bool
	wantBlocks     bool
	outline        bool
	block          string
	compactRefs    bool
	markdown       bool
}

// validate applies the Phase-1 param legality matrix: outline and block are
// mutually exclusive with each other and with format=md; illegal
// combinations → 400 ambiguous_input naming the conflicting params.
func (q V2ObjectQuery) validate() (objectReadPlan, error) {
	plan := objectReadPlan{wantProperties: true, wantBlocks: true, compactRefs: true}

	if q.Outline && q.Block != "" {
		return plan, apimodel.V2AmbiguousInput("outline and block are mutually exclusive — request the outline or one subtree, not both",
			apimodel.V2Issue{Path: "outline", Message: "conflicts with block"},
			apimodel.V2Issue{Path: "block", Message: "conflicts with outline"})
	}
	if q.Format == V2FormatMd && q.Outline {
		return plan, apimodel.V2AmbiguousInput("format=md and outline are mutually exclusive — the outline shape is AnyBlock-only",
			apimodel.V2Issue{Path: "format", Message: "conflicts with outline"},
			apimodel.V2Issue{Path: "outline", Message: "conflicts with format=md"})
	}
	if q.Format == V2FormatMd && q.Block != "" {
		return plan, apimodel.V2AmbiguousInput("format=md and block are mutually exclusive — subtree reads are AnyBlock-only",
			apimodel.V2Issue{Path: "format", Message: "conflicts with block"},
			apimodel.V2Issue{Path: "block", Message: "conflicts with format=md"})
	}

	switch q.Ids {
	case "", V2IdsCompact:
		plan.compactRefs = true
	case V2IdsFull:
		plan.compactRefs = false
	default:
		return plan, apimodel.V2ValidationFailed("invalid ids value",
			apimodel.V2Issue{Path: "ids", Message: fmt.Sprintf("unknown value %q", q.Ids), Hint: "allowed: compact, full"})
	}

	switch q.Format {
	case "", V2FormatAnyblock:
	case V2FormatMd:
		plan.markdown = true
	default:
		return plan, apimodel.V2ValidationFailed("invalid format value",
			apimodel.V2Issue{Path: "format", Message: fmt.Sprintf("unknown value %q", q.Format), Hint: "allowed: anyblock, md"})
	}

	if q.Include != "" {
		plan.wantProperties, plan.wantBlocks = false, false
		for _, part := range strings.Split(q.Include, ",") {
			switch strings.TrimSpace(part) {
			case V2IncludeProperties:
				plan.wantProperties = true
			case V2IncludeBlocks:
				plan.wantBlocks = true
			case "":
			default:
				return plan, apimodel.V2ValidationFailed("invalid include value",
					apimodel.V2Issue{Path: "include", Message: fmt.Sprintf("unknown value %q", strings.TrimSpace(part)), Hint: "allowed: properties, blocks"})
			}
		}
	}

	plan.outline = q.Outline
	// the outline shape carries properties only when include=properties
	// accompanies it ("an accompanying include=properties adds the
	// properties map" — Phase 1 param matrix)
	if plan.outline && q.Include == "" {
		plan.wantProperties = false
	}
	plan.block = q.Block
	// a subtree read implies blocks, like outline does — include=properties
	// alongside block= adds the properties map instead of emptying the read
	if plan.block != "" {
		plan.wantBlocks = true
	}
	return plan, nil
}

// mapReadError converts live-read failures into C6 errors.
func mapReadError(spaceId, objectId string, err error) error {
	if errors.Is(err, treestorage.ErrUnknownTreeId) {
		return apimodel.V2NotFound(fmt.Sprintf("object %q not found in space %q", objectId, spaceId))
	}
	if errors.Is(err, space.ErrSpaceNotExists) || errors.Is(err, space.ErrSpaceDeleted) {
		return apimodel.V2NotFound(fmt.Sprintf("space %q not found", spaceId))
	}
	return fmt.Errorf("read object %s: %w", objectId, err)
}

// GetObject reads one object via the live smartblock state → snapshot →
// anyblockjson.Marshal, and derives the etag from the same read (§8). The
// returned body is the flat AnyBlock document with the envelope etag; the
// caller sets the ETag header from the second return.
func (s *V2Service) GetObject(ctx context.Context, spaceId, objectId string, q V2ObjectQuery) ([]byte, string, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, "", err
	}
	plan, err := q.validate()
	if err != nil {
		return nil, "", err
	}

	if plan.markdown {
		return s.markdownEnvelope(ctx, spaceId, objectId)
	}

	read, err := s.reader.ReadObject(ctx, spaceId, objectId)
	if err != nil {
		return nil, "", mapReadError(spaceId, objectId, err)
	}
	etag := ComputeEtag(read.Heads)

	opts := storeresolver.New(s.store.SpaceIndex(spaceId)).Options()
	// object refs compact via the refs legend (C4) — but the outline drops the
	// legend when it drops the properties map, so a compacted object ref left
	// in a heading's text would become an unresolvable label. Keep object refs
	// full in the outline shape (T7).
	opts.CompactObjectRefs = plan.compactRefs && !plan.outline
	// block ids stay full on default reads (C4); the outline shape is the
	// read-only exception and uses compact block labels
	opts.CompactBlockLabels = plan.outline
	// C11 (M3): a read never fails on content the format can't represent —
	// unmapped/over-deep blocks degrade to warnings that ride the envelope.
	var warnings []apimodel.V2Issue
	opts.OnWarning = func(iss anyblockjson.Issue) {
		warnings = append(warnings, apimodel.V2Issue{Path: iss.Path, Message: iss.Message})
	}

	doc, err := anyblockjson.Marshal(read.SbType, read.Snapshot, opts)
	if err != nil {
		return nil, "", fmt.Errorf("marshal object %s: %w", objectId, err)
	}
	fields, err := parseEnvelope(doc)
	if err != nil {
		return nil, "", fmt.Errorf("object %s: %w", objectId, err)
	}

	if fields["etag"], err = rawJSON(etag); err != nil {
		return nil, "", err
	}

	if plan.outline {
		if err := buildOutlineEnvelope(fields, plan.wantProperties); err != nil {
			return nil, "", fmt.Errorf("object %s: %w", objectId, err)
		}
	} else {
		if !plan.wantProperties {
			delete(fields, "properties")
		}
		if !plan.wantBlocks {
			delete(fields, "blocks")
		}
		if plan.block != "" {
			if err := filterBlockSubtree(fields, plan.block); err != nil {
				return nil, "", err
			}
		}
	}

	if len(warnings) > 0 {
		if fields["warnings"], err = rawJSON(warnings); err != nil {
			return nil, "", err
		}
	}

	body, err := encodeEnvelope(fields)
	if err != nil {
		return nil, "", fmt.Errorf("object %s: %w", objectId, err)
	}
	return body, etag, nil
}

// markdownEnvelope builds the format=md response. The etag is read AFTER the
// export (§8, M2): a concurrent edit between the two can only make the etag
// reflect a state at or after the markdown, never before it, so the returned
// markdown is never newer than the etag advertises (over-reporting freshness
// is the safe direction for a later If-Match).
// The markdown converter has no loss channel today, so the response carries no
// warnings. TODO(GO-7383): surface C11 warnings for nodes the markdown export
// drops once core/converter/md grows a loss channel (APIV2.md §3 build item
// "md-export loss detector").
func (s *V2Service) markdownEnvelope(ctx context.Context, spaceId, objectId string) ([]byte, string, error) {
	resp := s.mw.ObjectExport(ctx, &pb.RpcObjectExportRequest{
		SpaceId:  spaceId,
		ObjectId: objectId,
		Format:   model.Export_Markdown,
	})
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectExportResponseError_NULL {
		return nil, "", fmt.Errorf("export markdown for object %s: %s", objectId, resp.Error.Description)
	}

	read, err := s.reader.ReadObject(ctx, spaceId, objectId)
	if err != nil {
		return nil, "", mapReadError(spaceId, objectId, err)
	}
	etag := ComputeEtag(read.Heads)

	fields := map[string]json.RawMessage{}
	if fields["id"], err = rawJSON(objectId); err != nil {
		return nil, "", err
	}
	if typeKey := objectTypeKey(read); typeKey != "" {
		if fields["type"], err = rawJSON(typeKey); err != nil {
			return nil, "", err
		}
	}
	if fields["etag"], err = rawJSON(etag); err != nil {
		return nil, "", err
	}
	if fields["markdown"], err = rawJSON(resp.Result); err != nil {
		return nil, "", err
	}
	body, err := encodeEnvelope(fields)
	return body, etag, err
}

// objectTypeKey extracts the object's type key from the snapshot.
func objectTypeKey(read apicore.ObjectRead) string {
	if read.Snapshot == nil || len(read.Snapshot.ObjectTypes) == 0 {
		return ""
	}
	return strings.TrimPrefix(read.Snapshot.ObjectTypes[0], domain.TypeKey("").URL())
}

// outlineSource is the subset of a marshaled block relevant to the outline.
type outlineSource struct {
	Indent int    `json:"indent"`
	Id     string `json:"id"`
	Type   string `json:"type"`
	Text   string `json:"text"`
}

// outlineHeadingTypes are the block types whose text appears in the outline.
var outlineHeadingTypes = map[string]bool{
	"heading1": true, "heading2": true, "heading3": true,
	"toggleHeading1": true, "toggleHeading2": true, "toggleHeading3": true,
}

// buildOutlineEnvelope replaces the blocks array with the outline shape:
// every block's {indent, id, type}, text only on headings — so every block
// id is addressable for a follow-up ?block= read or a PATCH.
func buildOutlineEnvelope(fields map[string]json.RawMessage, keepProperties bool) error {
	var blocks []json.RawMessage
	if raw, ok := fields["blocks"]; ok {
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return fmt.Errorf("decode blocks for outline: %w", err)
		}
	}
	outline := make([]apimodel.V2OutlineEntry, 0, len(blocks))
	for i, raw := range blocks {
		var src outlineSource
		if err := json.Unmarshal(raw, &src); err != nil {
			return fmt.Errorf("decode block %d for outline: %w", i, err)
		}
		entry := apimodel.V2OutlineEntry{Indent: src.Indent, Id: src.Id, Type: src.Type}
		if outlineHeadingTypes[src.Type] {
			entry.Text = src.Text
		}
		outline = append(outline, entry)
	}

	raw, err := rawJSON(outline)
	if err != nil {
		return err
	}
	fields["outline"] = raw
	delete(fields, "blocks")
	if !keepProperties {
		delete(fields, "properties")
		delete(fields, "refs") // the legend serves the properties map here
	}
	return nil
}

// filterBlockSubtree keeps only the addressed block and its contiguous
// indent-run of descendants. Indents stay absolute so ids and depths are
// stable across full and subtree reads. The block reference resolves by exact
// id or by unique suffix (§9a), so a short outline label round-trips to
// ?block= (M1).
func filterBlockSubtree(fields map[string]json.RawMessage, blockRef string) error {
	var blocks []json.RawMessage
	if raw, ok := fields["blocks"]; ok {
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return fmt.Errorf("decode blocks for subtree read: %w", err)
		}
	}
	type blockProbe struct {
		Indent int    `json:"indent"`
		Id     string `json:"id"`
	}
	probes := make([]blockProbe, len(blocks))
	ids := make([]string, len(blocks))
	for i, raw := range blocks {
		if err := json.Unmarshal(raw, &probes[i]); err != nil {
			return fmt.Errorf("decode block %d for subtree read: %w", i, err)
		}
		ids[i] = probes[i].Id
	}

	anchor, err := resolveBlockRef(ids, blockRef)
	if err != nil {
		return err
	}

	anchorIndent := probes[anchor].Indent
	run := []json.RawMessage{blocks[anchor]}
	for i := anchor + 1; i < len(blocks); i++ {
		if probes[i].Indent <= anchorIndent {
			break
		}
		run = append(run, blocks[i])
	}
	raw, err := rawJSON(run)
	if err != nil {
		return err
	}
	fields["blocks"] = raw
	return nil
}

// matchBlockRef maps a block reference to an index into ids: an exact id
// match wins (matches = 1); otherwise ids whose full value ends with ref are
// counted (a compact outline label is the id's last few characters, §9a) and
// idx points at the last suffix match. Shared by the ?block= read and the
// PATCH ops (C4).
func matchBlockRef(ids []string, ref string) (idx, matches int) {
	suffix, suffixCount := -1, 0
	for i, id := range ids {
		if id == ref {
			return i, 1
		}
		if ref != "" && strings.HasSuffix(id, ref) {
			suffix, suffixCount = i, suffixCount+1
		}
	}
	return suffix, suffixCount
}

// resolveBlockRef wraps matchBlockRef with the ?block= read errors: zero
// matches → 404; an ambiguous suffix → 400 steering to the full id.
func resolveBlockRef(ids []string, ref string) (int, error) {
	idx, matches := matchBlockRef(ids, ref)
	switch {
	case matches == 1:
		return idx, nil
	case matches > 1:
		return -1, apimodel.V2AmbiguousInput(
			fmt.Sprintf("block label %q matches more than one block — use the full block id", ref),
			apimodel.V2Issue{Path: "block", Message: "the label is a suffix of several block ids"})
	default:
		return -1, apimodel.V2NotFound(fmt.Sprintf("block %q not found — read the object without ?block= or with ?outline=true to list block ids", ref))
	}
}

//
// ---- object list (C5 minimal rows) ----
//

// ListObjects returns minimal rows (id, name, type + requested fields) for
// the space's objects, newest-modified first.
func (s *V2Service) ListObjects(ctx context.Context, spaceId string, fields []string, offset, limit int) ([]apimodel.V2ObjectRow, int, bool, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, 0, false, err
	}
	index := s.store.SpaceIndex(spaceId)
	records, total, err := index.QueryAndCount(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(util.LayoutsToIntArgs(util.ObjectLayouts)),
			},
			{
				RelationKey: "type.uniqueKey",
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.String(bundle.TypeKeyTemplate.URL()),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
		},
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyLastModifiedDate,
			Type:        model.BlockContentDataviewSort_Desc,
			IncludeTime: true,
		}},
		Offset: offset,
		Limit:  limit + 1, // one extra record detects has_more without a second scan
	})
	if err != nil {
		return nil, 0, false, fmt.Errorf("query objects in space %s: %w", spaceId, err)
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}

	typeKeys, err := s.typeKeysById(spaceId)
	if err != nil {
		return nil, 0, false, err
	}
	var opts anyblockjson.Options
	if len(fields) > 0 {
		opts = storeresolver.New(index).Options()
	}

	rows := make([]apimodel.V2ObjectRow, 0, len(records))
	for _, record := range records {
		typeId := record.Details.GetString(bundle.RelationKeyType)
		typeKey := typeKeys[typeId]
		if typeKey == "" && typeId != "" {
			// the bulk map misses edge type ids (hidden/bundled); resolve the
			// one type object directly so no row carries an empty type (C5).
			if det, err := index.GetDetails(typeId); err == nil {
				if k, err := domain.GetTypeKeyFromRawUniqueKey(det.GetString(bundle.RelationKeyUniqueKey)); err == nil {
					typeKey = string(k)
				}
			}
			typeKeys[typeId] = typeKey // memoize (including "" to avoid re-querying)
		}
		row := apimodel.V2ObjectRow{
			Id:   record.Details.GetString(bundle.RelationKeyId),
			Name: record.Details.GetString(bundle.RelationKeyName),
			Type: typeKey,
		}
		if len(fields) > 0 {
			values := map[string]any{}
			proto := record.Details.ToProto()
			for _, key := range fields {
				if v, ok := proto.Fields[key]; ok {
					values[key] = anyblockjson.MarshalPropertyValue(key, v, opts)
				}
			}
			if len(values) > 0 {
				row.Properties = values
			}
		}
		rows = append(rows, row)
	}
	return rows, total, hasMore, nil
}

// typeKeysById maps type object ids to type keys — rows carry the type key
// (C2), never the type object (C5).
func (s *V2Service) typeKeysById(spaceId string) (map[string]string, error) {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_objectType)),
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("query types in space %s: %w", spaceId, err)
	}
	out := make(map[string]string, len(records))
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		uniqueKey := record.Details.GetString(bundle.RelationKeyUniqueKey)
		key, err := domain.GetTypeKeyFromRawUniqueKey(uniqueKey)
		if err != nil {
			continue
		}
		out[id] = string(key)
	}
	return out, nil
}
