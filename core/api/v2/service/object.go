package v2service

// object.go implements the object read surface:
// GET /v2/spaces/{space_id}/objects/{object_id} with include/outline/block/
// ids/format, and the C5 minimal-row object list.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

// Query parameter values.
const (
	V2IdsCompact = "compact"
	V2IdsFull    = "full"

	V2FormatAnyblock = "anyblock"
	V2FormatMd       = "md"

	V2IncludeProperties = "properties"
	V2IncludeBlocks     = "blocks"
)

// ObjectQuery carries the GET object query parameters as received.
type ObjectQuery struct {
	Include string // comma-separated subset of properties,blocks; "" = both
	Outline bool
	Block   string
	Ids     string // compact (default) = the edit shape | full = the export shape (C4)
	Format  string // anyblock (default) | md
}

// objectReadPlan is the validated form of a ObjectQuery.
//
// One id axis moves per shape: block-id relabeling. Object refs stay full
// inline on EVERY shape — the `refs` legend is a measured net loss on every
// corpus document (§8.25's legend axis has it costing
// 0.9–11.5 % per document, 5.3 % on the ref-heaviest row), and the
// indirection traps write-back of object-valued properties.
// Legend RESOLUTION on input is untouched and total (SPEC §9a), so a
// document arriving with a legend still writes back.
type objectReadPlan struct {
	wantProperties bool
	wantBlocks     bool
	outline        bool
	block          string
	// compactBlockLabels relabels machine-minted doc-local block/row/column/
	// view ids (24-hex, view UUIDs — anyblockjson.isMintedLocalId) to short
	// suffixes — legend-less and LOSSY (the originals are not recoverable
	// from the document), which is why the export shape does not use it.
	// Meaningful ids never relabel. Every write channel resolves a label
	// back by unique suffix (matchBlockRef), so a labeled read stays
	// addressable by PATCH, `?block=` and `?view=`.
	compactBlockLabels bool
	markdown           bool
}

// validate applies the query-parameter legality matrix: outline and block are
// mutually exclusive with each other and with format=md; illegal
// combinations → 400 ambiguous_input naming the conflicting params.
func (q ObjectQuery) validate() (objectReadPlan, error) {
	plan := objectReadPlan{wantProperties: true, wantBlocks: true, compactBlockLabels: true}

	if q.Outline && q.Block != "" {
		return plan, v2model.AmbiguousInput("outline and block are mutually exclusive — request the outline or one subtree, not both",
			v2model.Issue{Path: "outline", Message: "conflicts with block"},
			v2model.Issue{Path: "block", Message: "conflicts with outline"})
	}
	if q.Format == V2FormatMd && q.Outline {
		return plan, v2model.AmbiguousInput("format=md and outline are mutually exclusive — the outline shape is AnyBlock-only",
			v2model.Issue{Path: "format", Message: "conflicts with outline"},
			v2model.Issue{Path: "outline", Message: "conflicts with format=md"})
	}
	if q.Format == V2FormatMd && q.Block != "" {
		return plan, v2model.AmbiguousInput("format=md and block are mutually exclusive — subtree reads are AnyBlock-only",
			v2model.Issue{Path: "format", Message: "conflicts with block"},
			v2model.Issue{Path: "block", Message: "conflicts with format=md"})
	}

	// `?ids=` selects one of TWO document shapes.
	//
	// compact = the edit shape: short labels for minted block ids.
	// full = the export shape: full block ids everywhere, so a GET body PUTs
	// back as a minimal diff (APIV2.md §3(b)). No shape serves the refs
	// legend — this used to be where it lived, until its own measurement
	// showed it a pure loss (§8.26), which left no shape offering full block
	// ids without the write-back-trapping indirection.
	//
	// The parse is ParseIdsShape (idshape.go), the same one the route
	// middleware runs to decide how SPACE ids are spelled in the same
	// response (§8.36): one parameter, one list of legal values, one 400.
	fullIds, err := ParseIdsShape(q.Ids)
	if err != nil {
		return plan, err
	}
	plan.compactBlockLabels = !fullIds

	switch q.Format {
	case "", V2FormatAnyblock:
	case V2FormatMd:
		plan.markdown = true
	default:
		return plan, v2model.ValidationFailed("invalid format value",
			v2model.Issue{Path: "format", Message: fmt.Sprintf("unknown value %q", q.Format), Hint: "allowed: anyblock, md"})
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
				return plan, v2model.ValidationFailed("invalid include value",
					v2model.Issue{Path: "include", Message: fmt.Sprintf("unknown value %q", strings.TrimSpace(part)), Hint: "allowed: properties, blocks"})
			}
		}
	}

	plan.outline = q.Outline
	// the outline shape carries properties only when include=properties
	// accompanies it ("an accompanying include=properties adds the
	// properties map")
	if plan.outline && q.Include == "" {
		plan.wantProperties = false
	}
	plan.block = q.Block
	// a subtree read implies blocks, like outline does — include=properties
	// alongside block= adds the properties map instead of emptying the read
	if plan.block != "" {
		plan.wantBlocks = true
	}
	// the outline shape fixes the axis and ignores `?ids=` (C4 T7): it is
	// read-only, so lossy block labels are free.
	if plan.outline {
		plan.compactBlockLabels = true
	}
	return plan, nil
}

// mapReadError converts live-read failures into C6 errors.
func mapReadError(spaceId, objectId string, err error) error {
	if errors.Is(err, treestorage.ErrUnknownTreeId) {
		return v2model.NotFound(fmt.Sprintf("object %q not found in space %q", objectId, spaceId))
	}
	if errors.Is(err, space.ErrSpaceNotExists) || errors.Is(err, space.ErrSpaceDeleted) {
		return v2model.NotFound(fmt.Sprintf("space %q not found", spaceId))
	}
	return fmt.Errorf("read object %s: %w", objectId, err)
}

// restrictionForbidden is the C6 403 for an object-restriction refusal:
// permanent for this object, so the message says not to retry.
func restrictionForbidden(objectId string, err error) *v2model.Error {
	return v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
		fmt.Sprintf("edit of object %q refused by its restrictions: %s — the refusal is permanent for this object, do not retry", objectId, err))
}

// mapWriteError classifies mutation-path failures (PATCH/PUT). A
// restriction refusal — the adapter's in-lock checkObjectEditable re-check
// or Apply's per-block restrictions, both wrapping
// restriction.ErrRestricted — is a PERMANENT 403: falling through to
// mapReadError dressed it as a read-shaped 500 and sent retrying agents
// into a loop (surface review M2a). Everything else takes the read
// classification.
func mapWriteError(spaceId, objectId string, err error) error {
	if errors.Is(err, restriction.ErrRestricted) {
		return restrictionForbidden(objectId, err)
	}
	return mapReadError(spaceId, objectId, err)
}

// GetObject reads one object via the live smartblock state → snapshot →
// anyblockjson.Marshal, and derives the etag from the same read (§8). The
// returned body is the flat AnyBlock document with the envelope etag; the
// caller sets the ETag header from the second return.
func (s *Service) GetObject(ctx context.Context, spaceId, objectId string, q ObjectQuery) ([]byte, string, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
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

	reads := storeresolver.New(s.store.SpaceIndex(spaceId))
	if read.SbType == model.SmartBlockType_STType {
		// tombstone window (§8.41): a just-deleted relation's index row is
		// {id, isDeleted} only, so the by-id resolve behind typeProperties
		// fails and the entry would silently VANISH from the served list —
		// and the documented read-modify-write loop would then delete the
		// type's reference to it. The surviving tree still knows everything;
		// read it and seed the resolver so all three store shapes serve the
		// same bytes.
		s.seedTombstonedTypeProperties(ctx, spaceId, reads, read.Snapshot)
	}
	opts := reads.Options()
	// A served body spells keys as the API slug. The vocabulary decision is
	// made HERE, in Options, never by re-spelling
	// a marshaled document. `?keys=name` (§4.2) keeps the resolver's own
	// raw-name vocabulary instead.
	if !nameKeysRequested(ctx) {
		opts.Keys = s.apiKeys(spaceId, opts.Keys)
	}
	// the shape comes pre-composed by validate() — see objectReadPlan;
	// CompactObjectRefs stays at its zero value on every shape (no legend)
	opts.CompactBlockLabels = plan.compactBlockLabels
	// every read shape annotates table columns with their header text: a
	// column id is the one doc-local id compact relabeling can never shorten
	// (its suffix is shared by every derived <rowId>-<colId> cell id), so
	// without the annotation the only link between the header word a caller
	// knows and the id set_cell needs was POSITION in the header row. The
	// member is API-read-only — the format neither emits nor stores it, and
	// the schema admits it as x-output-only so a read body writes back.
	opts.TableColumnHeaders = true
	// C11 (M3): a read never fails on content the format can't represent —
	// unmapped/over-deep blocks degrade to warnings that ride the envelope.
	var warnings []v2model.Issue
	opts.OnWarning = func(iss anyblockjson.Issue) {
		warnings = append(warnings, v2model.Issue{Path: iss.Path, Message: iss.Message})
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
			storedIds := func() ([]string, error) { return s.exportShapeBlockIds(spaceId, read) }
			if err := filterBlockSubtree(fields, plan.block, storedIds); err != nil {
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

// seedTombstonedTypeProperties makes a type read serve the SAME
// typeProperties in the tombstone window as before the delete and after the
// next space load (§8.41). For every recommended-relation id the store
// resolver cannot answer (GetRelationById needs a relationKey the tombstone
// row lost), it confirms the row exists as a tombstone and reads the LIVE
// object — the tree survives a UI delete by design — to
// recover key, name and format, then seeds the resolver. Every miss degrades
// to the pre-§8.41 behavior for that entry (dropped), never to an error: a
// dangling id in a recommended list has always been dropped, and the read
// must not fail on it.
func (s *Service) seedTombstonedTypeProperties(ctx context.Context, spaceId string, reads *storeresolver.Resolvers, snapshot *model.SmartBlockSnapshotBase) {
	if snapshot == nil || snapshot.Details == nil {
		return
	}
	index := s.store.SpaceIndex(spaceId)
	details := domain.NewDetailsFromProto(snapshot.Details)
	for _, listKey := range typeRecommendedListKeys {
		for _, id := range details.GetStringList(listKey) {
			if _, ok := reads.PropertyById(id); ok {
				continue
			}
			row, err := index.GetDetails(id)
			if err != nil || row.GetString(bundle.RelationKeyId) == "" || !row.GetBool(bundle.RelationKeyIsDeleted) {
				continue // no row, or not a tombstone — the drop stands
			}
			relRead, err := s.reader.ReadObject(ctx, spaceId, id)
			if err != nil || relRead.Snapshot == nil || relRead.Snapshot.Details == nil {
				continue
			}
			live := domain.NewDetailsFromProto(relRead.Snapshot.Details)
			key := live.GetString(bundle.RelationKeyRelationKey)
			if key == "" {
				continue
			}
			reads.SeedProperty(id, anyblockjson.PropertyDefinition{
				Key:    domain.RelationKey(key),
				Name:   live.GetString(bundle.RelationKeyName),
				Format: model.RelationFormat(live.GetInt64(bundle.RelationKeyRelationFormat)),
			})
		}
	}
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
func (s *Service) markdownEnvelope(ctx context.Context, spaceId, objectId string) ([]byte, string, error) {
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

// outlineTextRunes caps the text one outline entry serves. The cap counts
// RUNES, not bytes — a byte cut through multibyte text splits a code point
// and serves mojibake as the block's text.
const outlineTextRunes = 80

// outlineText renders a block's text for the outline: complete when it fits
// outlineTextRunes, otherwise cut there with a single … marking the loss.
func outlineText(text string) string {
	runes := []rune(text)
	if len(runes) <= outlineTextRunes {
		return text
	}
	return string(runes[:outlineTextRunes]) + "…"
}

// buildOutlineEnvelope replaces the blocks array with the outline shape:
// every block's {indent, id, type} plus a bounded text snippet — so every
// block id is addressable for a follow-up ?block= read or a PATCH, and
// every block is readable enough to pick the right one. The outline used to
// serve text on headings only, and that was a measured trap: an agent whose
// first read was the outline then edited paragraphs it had never seen
// (task pass rate 33% after an outline first read vs 94% without one). The
// snippet cap keeps the outline the cheap survey shape — the skeleton plus
// a snippet, never the whole document. One truncation rule for every block
// type, headings included: headings are short, so the cap virtually never
// fires on the navigation spine, and a uniform rule keeps the outline's
// size bound independent of block type.
func buildOutlineEnvelope(fields map[string]json.RawMessage, keepProperties bool) error {
	var blocks []json.RawMessage
	if raw, ok := fields["blocks"]; ok {
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return fmt.Errorf("decode blocks for outline: %w", err)
		}
	}
	outline := make([]v2model.OutlineEntry, 0, len(blocks))
	for i, raw := range blocks {
		var src outlineSource
		if err := json.Unmarshal(raw, &src); err != nil {
			return fmt.Errorf("decode block %d for outline: %w", i, err)
		}
		outline = append(outline, v2model.OutlineEntry{
			Indent: src.Indent, Id: src.Id, Type: src.Type,
			Text: outlineText(src.Text),
		})
	}

	raw, err := rawJSON(outline)
	if err != nil {
		return err
	}
	fields["outline"] = raw
	delete(fields, "blocks")
	if !keepProperties {
		delete(fields, "properties")
		delete(fields, "refs") // backstop: no shape serves a legend anymore
	}
	return nil
}

// filterBlockSubtree keeps only the addressed block and its contiguous
// indent-run of descendants. Indents stay absolute so ids and depths are
// stable across full and subtree reads. The block reference resolves by
// exact id or by unique suffix (§9a) against the SERVED ids first, then
// against the stored ids (storedIds) — so BOTH spellings of a relabeled
// block address it: the short label a default read shows, and the full
// stored id a `?ids=full` read, PATCH `created_blocks` or another client
// holds. Without the stored-id fallback the full id 404ed on the default
// shape — an addressability hole between the two vocabularies.
//
// The envelope is marked partial with "subtree": true — the way the outline
// shape is partial by construction. Without the marker the subtree body was
// schema-valid, and PUT of that exact body silently deleted every block
// outside the subtree (reproduced: 6-block page, GET ?block= then PUT →
// blocks_removed: 5). The marker makes every write path refuse it — the
// AnyBlock envelope is additionalProperties:false, so Validate rejects it
// structurally, and PUT/create name it precisely before that.
//
// storedIds() lists the stored spelling of each SERVED block, index-aligned
// with the served blocks array (exportShapeBlockIds), and is only invoked on
// the fallback path.
func filterBlockSubtree(fields map[string]json.RawMessage, blockRef string, storedIds func() ([]string, error)) error {
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
		// the stored-id vocabulary fallback. Only a not-found falls through:
		// a served-vocabulary AMBIGUITY stays a refusal — resolving it
		// against the other vocabulary would silently pick one of the blocks
		// the 400 exists to make the caller disambiguate.
		var refErr *v2model.Error
		if !errors.As(err, &refErr) || refErr.Code != v2model.CodeNotFound {
			return err
		}
		stored, storedErr := storedIds()
		if storedErr != nil {
			return storedErr
		}
		// stored[i] is the stored spelling of blocks[i] — the same marshal
		// modulo relabeling — so a resolved ref maps to its served block by
		// POSITION. (A suffix scan here served the WRONG block: any earlier
		// served id that happened to tail the matched stored id won — "b1"
		// tails "…9ab1".) A stored id that is never served (the root, table
		// wrappers, cells) is simply absent from the list and stays a 404.
		if len(stored) != len(ids) {
			return fmt.Errorf("subtree stored-id fallback: %d stored ids for %d served blocks", len(stored), len(ids))
		}
		if anchor, err = resolveBlockRef(stored, blockRef); err != nil {
			return err
		}
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
	if fields["subtree"], err = rawJSON(true); err != nil {
		return err
	}
	return nil
}

// exportShapeBlockIds re-marshals a read WITHOUT block-id relabeling and
// returns the top-level block ids in document order — the second resolution
// vocabulary for ?block= (the first is the served ids). Relabeling changes
// id spellings only, never the block set or order, so index i here is the
// STORED spelling of the served document's block i: the fallback maps a
// resolved stored id to its served block positionally. The discarding
// warning sink keeps degradation identical to the served marshal (without a
// sink, C11-degradable content fails the marshal instead). Blocks that never
// render as flat blocks — the root, table wrappers, cells — are absent here
// exactly as they are absent from the served array.
func (s *Service) exportShapeBlockIds(spaceId string, read apicore.ObjectRead) ([]string, error) {
	opts := storeresolver.New(s.store.SpaceIndex(spaceId)).Options()
	opts.OnWarning = func(anyblockjson.Issue) {}
	doc, err := anyblockjson.Marshal(read.SbType, read.Snapshot, opts)
	if err != nil {
		return nil, fmt.Errorf("marshal stored-id shape: %w", err)
	}
	var probe struct {
		Blocks []struct {
			Id string `json:"id"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(doc, &probe); err != nil {
		return nil, fmt.Errorf("decode stored-id shape: %w", err)
	}
	ids := make([]string, len(probe.Blocks))
	for i, b := range probe.Blocks {
		ids[i] = b.Id
	}
	return ids, nil
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
		return -1, v2model.AmbiguousInput(
			fmt.Sprintf("block label %q matches more than one block — use the full block id", ref),
			v2model.Issue{Path: "block", Message: "the label is a suffix of several block ids"})
	default:
		return -1, v2model.NotFound(fmt.Sprintf("block %q not found", ref),
			v2model.Issue{Path: "block", Message: v2AddressableBlocksMessage, Hint: v2AddressableBlocksHint})
	}
}

// v2AddressableBlocksMessage / v2AddressableBlocksHint are the shared
// not-found repair loop for a block reference, on the read side (?block=)
// and the write side (the PATCH ops). They have to be TRUE: both messages
// used to say "GET the object with ?outline=true to list block ids", and a
// caller who had just been served a block id the outline does not list was
// sent round that loop forever.
//
// What ?outline=true lists is exactly the document's blocks array — the set
// a block reference resolves against, on every channel. A default read also
// serves ids that live INSIDE a block: table row and column ids, the ids of
// blocks nested in table cells, dataview view ids. Those are real stored ids
// and they relabel like any other, but they are not block references: they
// are addressed through the slot that owns them (set_cell's row/col, the view
// ops' view), and a block nested in a cell has no addressing slot at all —
// it is reached by rewriting its cell.
//
// TODO(GO-7383): make cell descendants addressable. Ticketed, not taken —
// see APIV2.md §8.29 for what it would cost (a second addressing mode in
// every ref-taking op, a served shape for a partial cell run, and an outline
// entry that says "this is not a sibling of the top-level run").
const (
	v2AddressableBlocksMessage = "the addressable blocks are the entries of the document's blocks array"
	v2AddressableBlocksHint    = "GET the object with ?outline=true to list them. Ids nested inside a block are served but are not block references: a table's rows and columns are addressed by set_cell's row/col, a dataview's views by the view ops, and a block inside a table cell is not individually addressable — rewrite its cell with set_cell."
)

//
// ---- object list (C5 minimal rows) ----
//

// ListObjects returns minimal rows (id, name, type + requested fields) for
// the space's objects, newest-modified first.
func (s *Service) ListObjects(ctx context.Context, spaceId string, fields []string, offset, limit int) ([]v2model.ObjectRow, int, bool, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, 0, false, err
	}
	if err := s.validateListFields(spaceId, fields); err != nil {
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

	builder, err := s.newObjectRowBuilder(spaceId, fields)
	if err != nil {
		return nil, 0, false, err
	}
	rows := make([]v2model.ObjectRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, builder.row(record))
	}
	return rows, total, hasMore, nil
}

// v2FieldAliases maps the file vocabulary of the v2 query surface onto the
// store properties backing it: `mimeType` and `size` are the
// format's OWN names for a file's mime and byte size (the SPEC §5 file-block
// fields, and the POST /files result) — the store relations are named
// fileMimeType/sizeInBytes. An ACTIVE alias (activeFieldAliases) is live in
// every channel — fields=, filters and sorts — translated to the backing
// relation, so the one advertised spelling works everywhere (C2).
var v2FieldAliases = map[string]domain.RelationKey{
	"mimeType": bundle.RelationKeyFileMimeType,
	"size":     bundle.RelationKeySizeInBytes,
}

// activeFieldAliasesIn resolves the aliases against one primed live set:
// an alias is active only when no LIVE property claims its spelling —
// through the same chain everything else resolves, so a stored key OR a
// slug claims it, and a UI-deleted (uninstalled) relation no longer
// deactivates the alias space-wide (the review's corpse-blind finding:
// one uninstalled mimeType relation silently dropped the field from every
// file row and filter). The decision is per SPACE, never per row.
func (s *Service) activeFieldAliasesIn(entries []propertyEntry) map[string]domain.RelationKey {
	active := make(map[string]domain.RelationKey, len(v2FieldAliases))
	for alias, backing := range v2FieldAliases {
		if entry, ok, ambiguous := s.resolvePropertyInput(alias, entries); len(ambiguous) > 0 || (ok && entry.Id != "") {
			continue // a real live property claims the spelling
		}
		active[alias] = backing
	}
	return active
}

// activeFieldAliases is the load-owning form; on a store error no alias is
// served (conservative — the real property, if any, must win).
func (s *Service) activeFieldAliases(spaceId string) map[string]domain.RelationKey {
	entries, err := s.liveProperties(spaceId)
	if err != nil {
		return nil
	}
	return s.activeFieldAliasesIn(entries)
}

// objectRowBuilder assembles C5 minimal rows (id, name, type + requested
// property values) for one space, caching the type-key map and the property
// resolvers across rows. Shared by the object list, the query surface and
// the set/collection reads.
type objectRowBuilder struct {
	index    spaceindex.Store
	typeKeys map[string]string
	fields   []string
	opts     anyblockjson.Options
	spaceId  string // the store-facing full id
	// spaceRef is what a row's space_id FIELD carries when includeSpaceId
	// (global search): the §8.35 short reference by default, the full id
	// when its tail collides with another visible space's. Defaults to
	// spaceId so a builder constructed without a census still serves a
	// working value.
	spaceRef string
	// aliases is the builder's per-space alias resolution, computed ONCE at
	// construction (activeFieldAliases) — never per row
	aliases map[string]domain.RelationKey

	includeSpaceId bool
}

func (s *Service) newObjectRowBuilder(spaceId string, fields []string) (*objectRowBuilder, error) {
	typeKeys, err := s.typeKeysById(spaceId)
	if err != nil {
		return nil, err
	}
	index := s.store.SpaceIndex(spaceId)
	b := &objectRowBuilder{index: index, typeKeys: typeKeys, fields: fields, spaceId: spaceId, spaceRef: spaceId}
	if len(fields) > 0 {
		b.opts = storeresolver.New(index).Options()
		b.opts.Keys = s.apiKeys(spaceId, b.opts.Keys)
		// requested fields canonicalize through the one chain (file aliases
		// + §7.5a-5): the value is read from the STORED key and emitted
		// under the REQUESTED spelling — the listing's slug spelling works
		// in fields= exactly as advertised (review cause 3)
		kc, err := s.newKeyCanon(spaceId)
		if err != nil {
			return nil, err
		}
		for _, field := range fields {
			if canonical, ambiguous := kc.canon(field); len(ambiguous) == 0 && canonical != field {
				if b.aliases == nil {
					b.aliases = map[string]domain.RelationKey{}
				}
				b.aliases[field] = domain.RelationKey(canonical)
			}
		}
	}
	return b, nil
}

func (b *objectRowBuilder) row(record database.Record) v2model.ObjectRow {
	typeId := record.Details.GetString(bundle.RelationKeyType)
	typeKey, cached := b.typeKeys[typeId]
	if !cached && typeId != "" {
		// the bulk map misses edge type ids (hidden/bundled); resolve the
		// one type object directly so no row carries an empty type (C5).
		if det, err := b.index.GetDetails(typeId); err == nil {
			if k, err := domain.GetTypeKeyFromRawUniqueKey(det.GetString(bundle.RelationKeyUniqueKey)); err == nil {
				typeKey = string(k)
			}
		}
		b.typeKeys[typeId] = typeKey // memoize (including "" to avoid re-querying)
	}
	row := v2model.ObjectRow{
		Id:   record.Details.GetString(bundle.RelationKeyId),
		Name: record.Details.GetString(bundle.RelationKeyName),
		Type: typeKey,
	}
	if b.includeSpaceId {
		row.SpaceId = b.spaceRef
	}
	if len(b.fields) > 0 {
		values := map[string]any{}
		proto := record.Details.ToProto()
		for _, key := range b.fields {
			// MarshalPropertyValue also returns this key's option-id legend
			// (name -> stored option id). A ROW is not a document: it has no
			// envelope to hang a legend on, and select values have always been
			// served here as bare names. Dropping it knowingly keeps that
			// contract; serving option ids on rows is a surface decision, not
			// a consequence of the format change.
			if v, ok := proto.Fields[key]; ok {
				values[key], _ = anyblockjson.MarshalPropertyValue(key, v, b.opts)
				continue
			}
			// Read the backing store property for the file aliases,
			// emit under the requested name. b.aliases resolved per SPACE at
			// construction — a real property keyed mimeType/size deactivates
			// the alias for every row, never per record
			if backing, ok := b.aliases[key]; ok {
				if v, ok := proto.Fields[string(backing)]; ok {
					values[key], _ = anyblockjson.MarshalPropertyValue(string(backing), v, b.opts)
				}
			}
		}
		if len(values) > 0 {
			row.Properties = values
		}
	}
	return row
}

// typeKeysById maps type object ids to type keys — rows carry the type key
// (C2), never the type object (C5). Live types are spelled as their served
// key (the slug for a BSON-keyed type, §7.5a — the spelling the search
// type filter resolves right back); removed types (uninstalled, archived —
// or the prod corpse shape carrying isDeleted) stay in the map so their
// objects' rows keep a type, spelled by the honest internal key.
//
// The query suppresses both injected defaults DELIBERATELY (§8.41): a
// production corpse carries isDeleted, so the plain query this used to be
// never returned one and the corpse branch below was dead outside flag-only
// fixtures — production rows fell through to the per-row GetDetails fallback
// instead. Tombstoned rows ({id, isDeleted} only) still land here but carry
// no uniqueKey and are skipped; their objects' rows serve an empty type for
// the window, the only honest answer a keyless row allows.
func (s *Service) typeKeysById(spaceId string) (map[string]string, error) {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{RelationKey: bundle.RelationKeyIsArchived, Condition: model.BlockContentDataviewFilter_None},
			{RelationKey: bundle.RelationKeyIsDeleted, Condition: model.BlockContentDataviewFilter_None},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query types in space %s: %w", spaceId, err)
	}
	liveEntries, err := s.liveTypes(spaceId)
	if err != nil {
		return nil, err
	}
	keyTaken, slugHolders := servedTypeKeySets(liveEntries)
	out := make(map[string]string, len(records))
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		uniqueKey := record.Details.GetString(bundle.RelationKeyUniqueKey)
		key, err := domain.GetTypeKeyFromRawUniqueKey(uniqueKey)
		if err != nil {
			continue
		}
		if corpseFlagged(record.Details) {
			out[id] = string(key) // a corpse's slug vacated the namespace
			continue
		}
		out[id] = servedTypeKeyOf(string(key), record.Details.GetString(bundle.RelationKeyApiObjectKey), keyTaken, slugHolders)
	}
	return out, nil
}
