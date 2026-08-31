package anyblockjson

// widgetobject.go — the sidebar's object, and why a bundle does not carry
// one (§2c).
//
// `kind: "widget"` is a hidden per-space object whose BLOCKS encode the
// sidebar: one widget wrapper block per widget, each with exactly one
// indented link child naming the target. index.json has a first-class
// `widgets` array for exactly this, so the document restates the index the
// way the space document restated it earlier — except the index used to
// say less than the blocks. It says everything now, so the document can go.
//
// That is not an assumption. Measured over a 77-space export: 218 wrapper
// blocks, 218 link children, in 218 perfectly regular pairs — no wrapper
// without its link, no link outside a wrapper, and no other content besides
// the root and, in 11 spaces, the editor's header scaffolding (§7: a Header
// layout holding one EMPTY title block, which export drops from every
// document). The details reduce the same way:
//
//	createdDate         77 of 77   → dropped: 0 on every one of them, and a
//	                                 restored sidebar is created when it is
//	                                 restored (the space-document rule)
//	isHidden            77 of 77   → constant true; the rebuild restates it
//	layout              77 of 77   → constant `dashboard`; restated
//	resolvedLayout      77 of 77   → constant `dashboard`; restated
//	lastModifiedDate    49 of 77   → dropped, like createdDate
//	autoWidgetTargets   21 of 77   → index.auto_widget_targets
//	name                15 of 77   → "" on every one; a non-empty name keeps
//	                                 the document
//	autoWidgetDisabled   2 of 77   → index.auto_widget_disabled
//
// (everything else the raw snapshots hold — backlinks, links, mentions,
// restrictions, snippet, creator, lastModifiedBy, lastOpenedDate,
// internalFlags, id, spaceId, type — is already stripped or transient §3.)
//
// So export omits it, `IndexFromWidgetObject` is the one place that says
// which block member becomes which index field, and `WidgetsSnapshot` is the
// one function that builds the object back — shared by cmd/anyblockconvert
// (the archive the importer installs) and the round-trip verifier (the
// reconstruction check), so the lift and the rebuild cannot drift apart
// silently.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// WidgetsObjectId is the rebuilt widget snapshot's own id. Nothing derives
// from it: the importer replaces it with the space's derived Widgets id
// (objectid.widget.GetIDAndPayload returns spc.DerivedIDs().Widgets), and the
// root block is renamed to match before the state is built. It only has to
// be stable and impossible for a bundle object to claim — `widgets` is a
// reserved bundle id already (IsReservedBundleId, via the homepage table).
const WidgetsObjectId = "widgets"

// widgetWrapperSuffix is the convention core/block/editor/widget uses when a
// wrapper's id has to be derived from its link's rather than random
// (widget.createBlock), so that two devices creating the same widget do not
// end up with two wrappers.
const widgetWrapperSuffix = "-wrapper"

// the widget object's own preference details. The keys are the clients'
// (anytype-ts writes them); no bundle constant exists for either.
const (
	detailKeyAutoWidgetTargets  = "autoWidgetTargets"
	detailKeyAutoWidgetDisabled = "autoWidgetDisabled"
)

// widgetObjectResidualKeys are the two timestamps a widget document carries
// about ITSELF: when the hidden object was minted and last touched. A bundle
// is not that object — a restored sidebar is created when it is restored —
// so they are dropped exactly the way the space document's were (§2c). In
// the corpus every one of the 77 createdDate values is 0 anyway.
var widgetObjectResidualKeys = map[string]bool{
	bundle.RelationKeyCreatedDate.String():      true,
	bundle.RelationKeyLastModifiedDate.String(): true,
}

// WidgetObjectResidualKey reports a stored detail the widget-object omission
// drops without an index field to land in: the two object timestamps, and an
// EMPTY name — the widget object is hidden, nothing renders its name, and
// all 15 corpus documents that carry one carry "". A non-empty name is NOT
// residual; the omission predicate keeps the whole document for it. Exported
// for the round-trip comparator, which must apply the very predicate export
// applies (§11).
func WidgetObjectResidualKey(key string, v *types.Value) bool {
	if widgetObjectResidualKeys[key] {
		return true
	}
	return key == bundle.RelationKeyName.String() && v.GetStringValue() == ""
}

// widgetObjectConstantKeys are the details every widget document carries
// with one distinct value across the 77-document corpus, restated verbatim
// by WidgetsSnapshot — so the document says nothing the rebuild does not.
// Value-checked, unlike the space document's key-only list, because the
// rebuild writes these VALUES back: a widget object storing a layout other
// than `dashboard` would be silently re-laid-out by the omission, so it
// keeps its document instead.
func widgetObjectConstantDetail(key string, v *types.Value) bool {
	switch key {
	case bundle.RelationKeyIsHidden.String():
		return v.GetBoolValue()
	case bundle.RelationKeyLayout.String(), bundle.RelationKeyResolvedLayout.String():
		return v.GetNumberValue() == float64(model.ObjectType_dashboard)
	}
	return false
}

// liftableWidgetTarget reports whether a stored target-shaped value — a link
// child's target or an autoWidgetTargets entry — is one the index can spell:
// a wire listing name the importer knows (translated into the `_` namespace
// by FormatWidgetTarget), or a CID-shaped object id, which passes verbatim.
//
// The gate is what keeps the omission honest about the corpus's two strays:
// one space stores a widget targeting `bookmark` and one targeting `lists`,
// words no client constant defines and widget.IsPredefinedWidgetTargetId
// does not know. Written into an index they would read as object ids naming
// nothing, and the widget they mean would be dropped on install without an
// error — so a document holding one KEEPS the document, and the stray
// travels the way it always has.
// maxIndexWidgetLimit is the product cap on how many entries a sidebar
// listing widget shows — the ONE number behind three statements:
// `$defs/widgetListingLimit` in the object schema (which the index schema
// references), the authoring index's self-contained copy, and the lift check
// below. TestWidgetLimit_OneStatementOfTheCap chains all of them to this
// constant. The widget BLOCK's own `limit` deliberately takes the whole
// int32 range instead — the block is the fidelity fallback for exactly the
// widget this cap refuses, so the cap must not bind there.
const maxIndexWidgetLimit = 100

func liftableWidgetTarget(stored string) bool {
	format := FormatWidgetTarget(stored)
	if IsReservedWidgetTarget(format) {
		return IsImportableWidgetTarget(format)
	}
	return isObjectIdShaped(stored)
}

// widgetObjectWidgets reads the block graph into index widgets, reporting
// whether every block was accounted for. ok is false — and the caller must
// keep the document — for any shape beyond the measured one: an unpaired
// wrapper, a wrapper with extra children, a link with children of its own, a
// block attribute the pair cannot carry, an enum value outside the §5
// vocabulary, a limit outside the index schema's range, a target the index
// cannot spell, or any block that is not the root, the header scaffolding,
// or half of a pair.
func widgetObjectWidgets(base *model.SmartBlockSnapshotBase) (widgets []Widget, ok bool) {
	blocks := base.GetBlocks()
	if len(blocks) == 0 {
		return nil, true
	}
	byId := make(map[string]*model.Block, len(blocks))
	isChild := map[string]bool{}
	for _, b := range blocks {
		if b == nil || b.Id == "" || byId[b.Id] != nil {
			return nil, false
		}
		byId[b.Id] = b
		for _, c := range b.ChildrenIds {
			isChild[c] = true
		}
	}
	var root *model.Block
	for _, b := range blocks {
		if isChild[b.Id] {
			continue
		}
		if root != nil {
			return nil, false // two roots: not the shape this rule measured
		}
		root = b
	}
	if root == nil {
		return nil, false // a cycle; nothing to walk
	}
	if _, isRoot := root.Content.(*model.BlockContentOfSmartblock); !isRoot {
		return nil, false
	}
	if !plainBlock(root) {
		return nil, false
	}
	accounted := map[string]bool{root.Id: true}
	for _, id := range root.ChildrenIds {
		b := byId[id]
		if b == nil {
			return nil, false
		}
		switch c := b.Content.(type) {
		case *model.BlockContentOfLayout:
			// the editor's header scaffolding (§7), present in 11 of 77
			// corpus documents: a Header layout over one EMPTY title block.
			// Export drops it from every document, so the omission loses
			// nothing by accepting it — and accepts nothing more.
			if c.Layout.GetStyle() != model.BlockContentLayout_Header || !plainBlock(b) {
				return nil, false
			}
			accounted[b.Id] = true
			for _, cid := range b.ChildrenIds {
				t := byId[cid]
				// judged the way pageIsEmpty judges the space document's
				// scaffolding: on the text alone. The editor stamps a
				// `_detailsKey` binding into every title block's fields —
				// 11 of 11 in the corpus — and §7 drops the block, binding
				// and all, so the binding is not content to fail closed on.
				if t == nil || !emptyStructuralText(t) || len(t.ChildrenIds) > 0 {
					return nil, false
				}
				accounted[t.Id] = true
			}
		case *model.BlockContentOfWidget:
			w, link, admitted := widgetPair(b, byId)
			if !admitted {
				return nil, false
			}
			accounted[b.Id] = true
			accounted[link] = true
			widgets = append(widgets, w)
		default:
			return nil, false
		}
	}
	for id := range byId {
		if !accounted[id] {
			return nil, false // unreachable from the root: real content, kept
		}
	}
	return widgets, true
}

// widgetPair reads one wrapper-and-link pair into the flat index widget, or
// refuses. linkId names the accounted link block on success.
func widgetPair(wrapper *model.Block, byId map[string]*model.Block) (w Widget, linkId string, ok bool) {
	wc := wrapper.GetWidget()
	if !plainBlock(wrapper) || len(wrapper.ChildrenIds) != 1 {
		return w, "", false
	}
	link := byId[wrapper.ChildrenIds[0]]
	if link == nil || !plainBlock(link) || len(link.ChildrenIds) != 0 {
		return w, "", false
	}
	lc := link.GetLink()
	if lc == nil {
		return w, "", false
	}
	if !liftableWidgetTarget(lc.TargetBlockId) {
		return w, "", false
	}
	w.Target = FormatWidgetTarget(lc.TargetBlockId)
	// the wrapper's §5 members, through the same name tables the block
	// export uses; an enum value outside the vocabulary has no spelling
	if wc.Layout != model.BlockContentWidget_Link {
		if w.Layout = widgetLayoutNames.name(wc.Layout); w.Layout == "" {
			return w, "", false
		}
	}
	// the index schema bounds a limit ($defs/widgetListingLimit — the
	// deliberate product cap on sidebar listings; the corpus maximum is 50)
	// where the block schema takes the whole int32 range on purpose: a
	// widget this check refuses travels as a full document, and that
	// document's widget block must stay valid
	if wc.Limit < 0 || wc.Limit > maxIndexWidgetLimit {
		return w, "", false
	}
	w.Limit = wc.Limit
	w.ViewId = wc.ViewId
	w.AutoAdded = wc.AutoAdded
	// the link's §5 display members. Its deprecated `style` and legacy
	// `fields` are not checked: export drops both from every link block by
	// design (§5), so the document would not have carried them either.
	if lc.CardStyle != model.BlockContentLink_Text {
		if w.CardStyle = cardStyleNames.name(lc.CardStyle); w.CardStyle == "" {
			return w, "", false
		}
	}
	if lc.IconSize != model.BlockContentLink_SizeNone {
		if w.IconSize = iconSizeNames.name(lc.IconSize); w.IconSize == "" {
			return w, "", false
		}
	}
	if lc.Description != model.BlockContentLink_None {
		if w.Description = linkDescriptionNames.name(lc.Description); w.Description == "" {
			return w, "", false
		}
	}
	for _, key := range lc.Relations {
		// the same writable-key admission every §5 key slot runs (§3): an
		// empty or unwritable key cannot be spelled in the index, so the
		// document travels whole — where the link block's own slot rule
		// drops the entry with a warning
		if !isWritablePropertyKey(key) {
			return w, "", false
		}
	}
	w.Properties = lc.Relations
	return w, link.Id, true
}

// plainBlock reports a block carrying none of the generic attributes the
// flat widget cannot express — alignment, background, fields. The corpus
// holds zero widget-object blocks with any of them, and a document that
// grows one keeps travelling as a document.
func plainBlock(b *model.Block) bool {
	return b.Align == model.Block_AlignLeft &&
		b.VerticalAlign == model.Block_VerticalAlignTop &&
		b.BackgroundColor == "" &&
		(b.Fields == nil || len(b.Fields.Fields) == 0)
}

// emptyStructuralText reports the one text block the header scaffolding may
// hold: an EMPTY title or description, the same two styles pageIsEmpty
// admits on the space document.
func emptyStructuralText(b *model.Block) bool {
	t := b.GetText()
	if t == nil || t.GetText() != "" || len(t.GetMarks().GetMarks()) > 0 {
		return false
	}
	return t.GetStyle() == model.BlockContentText_Title ||
		t.GetStyle() == model.BlockContentText_Description
}

// liftedAutoWidgetTargets reads the client's auto-widget ledger into index
// spellings, refusing an entry the index cannot spell — the same gate the
// link targets pass.
func liftedAutoWidgetTargets(det map[string]*types.Value) (targets []string, ok bool) {
	v := det[detailKeyAutoWidgetTargets]
	if v == nil {
		return nil, true
	}
	for _, entry := range valueStringList(v) {
		if !liftableWidgetTarget(entry) {
			return nil, false
		}
		targets = append(targets, FormatWidgetTarget(entry))
	}
	return targets, true
}

// IndexFromWidgetObject reads the widget object into the index fields it is
// the source of (§2c): the widgets, the auto-widget ledger, and the
// auto-widget switch. It is the composer's half of the omission — a bundle
// that drops the document MUST write these, or the space loses its sidebar —
// and it fills only what the object states, like IndexFromSpaceSettings.
func IndexFromWidgetObject(idx *Index, base *model.SmartBlockSnapshotBase) {
	if idx == nil || base == nil {
		return
	}
	if widgets, ok := widgetObjectWidgets(base); ok && len(widgets) > 0 {
		idx.Widgets = widgets
	}
	det := base.GetDetails().GetFields()
	if targets, ok := liftedAutoWidgetTargets(det); ok && len(targets) > 0 {
		idx.AutoWidgetTargets = targets
	}
	if det[detailKeyAutoWidgetDisabled].GetBoolValue() {
		idx.AutoWidgetDisabled = true
	}
}

// OmittedWidgetObject reports a widget document a bundle does not write,
// because `index.json` states everything it holds (§2c).
//
// Fail-closed, like the space-document omission beside it: a member this
// package cannot account for — a block shape beyond the wrapper-and-link
// pair, a target the index cannot spell, a non-empty name, an unforeseen
// detail — keeps the document, so a widget object carrying something
// unforeseen travels rather than vanishing. Measured, that keeps 2 of 77
// corpus documents: the two whose link targets are the stray words
// `bookmark` and `lists` (see liftableWidgetTarget).
func OmittedWidgetObject(sbType model.SmartBlockType, base *model.SmartBlockSnapshotBase) bool {
	if sbType != model.SmartBlockType_Widget || base == nil {
		return false
	}
	if _, ok := widgetObjectWidgets(base); !ok {
		return false
	}
	det := base.GetDetails().GetFields()
	if _, ok := liftedAutoWidgetTargets(det); !ok {
		return false
	}
	stripped := strippedDetailKeys()
	for k, v := range det {
		switch {
		case k == detailKeyAutoWidgetTargets, k == detailKeyAutoWidgetDisabled:
			// index.json carries them — IndexFromWidgetObject is the proof
		case isTransientProperty(k), stripped[k]:
			// already refused or dropped by a rule of its own
		case WidgetObjectResidualKey(k, v):
			// the object's own timestamps, and an empty name (a NON-empty
			// name falls through to the default and keeps the document)
		case widgetObjectConstantDetail(k, v):
			// one distinct value across all 77 corpus documents, restated
			// verbatim by WidgetsSnapshot
		default:
			return false // unaccounted: keep the document
		}
	}
	return true
}

// WidgetsSnapshot builds the widget object back from the index — the exact
// snapshot cmd/anyblockconvert puts in an archive, and the reconstruction
// the round-trip verifier holds against the original, one function so the
// two cannot drift. It returns nil when the index carries no sidebar state
// at all: a bundle declaring nothing gets no snapshot rather than an empty
// one.
//
// Four things about the block graph are load-bearing, none of them obvious:
//
//   - The root block must carry smartblock content. objectcreator.setRootBlock
//     hands the blocks to anymark.AddRootBlock, which renames the first block
//     with that content to the derived widgets id. Without one, AddRootBlock
//     *appends* a second root instead, the state's root becomes that new block,
//     and every wrapper is orphaned.
//   - Every wrapper must be reachable from the root. updateWidgetObject walks
//     state.Blocks(), which is a breadth-first traversal from the root — a block
//     the root does not reach is simply not there.
//   - That traversal is also why root.ChildrenIds order is sidebar order:
//     addWidgetBlock appends each widget to the existing widget object in
//     traversal order (InsertTo with Block_Inner appends, despite the
//     prependChildrenIds it calls). So the order here is index.json's order.
//   - A wrapper gets exactly one child. addWidgetBlock reads ChildrenIds[0] and
//     ignores the rest.
//
// The auto-widget ledger and switch ride along as details. On today's
// experience path they are inert — objectcreator.updateWidgetObject merges
// only the BLOCKS into the space's own widget object — but they are the
// snapshot's truthful state, written so the path that starts reading details
// does not have to change this function. The §2c table records the inertness.
func WidgetsSnapshot(idx *Index) (*model.SmartBlockSnapshotBase, error) {
	if idx == nil {
		return nil, nil
	}
	if len(idx.Widgets) == 0 && len(idx.AutoWidgetTargets) == 0 && !idx.AutoWidgetDisabled {
		return nil, nil
	}

	root := &model.Block{
		Id:      WidgetsObjectId,
		Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
	}
	blocks := make([]*model.Block, 0, 1+2*len(idx.Widgets))
	blocks = append(blocks, root)

	for i, w := range idx.Widgets {
		if w.Layout != "" && !widgetLayoutNames.has(w.Layout) {
			return nil, fmt.Errorf("widgets[%d]: unknown layout %q", i, w.Layout)
		}
		if w.CardStyle != "" && !cardStyleNames.has(w.CardStyle) {
			return nil, fmt.Errorf("widgets[%d]: unknown card_style %q", i, w.CardStyle)
		}
		if w.IconSize != "" && !iconSizeNames.has(w.IconSize) {
			return nil, fmt.Errorf("widgets[%d]: unknown icon_size %q", i, w.IconSize)
		}
		if w.Description != "" && !linkDescriptionNames.has(w.Description) {
			return nil, fmt.Errorf("widgets[%d]: unknown description %q", i, w.Description)
		}
		linkId := widgetBlockId(i, w.Target)
		wrapperId := linkId + widgetWrapperSuffix

		root.ChildrenIds = append(root.ChildrenIds, wrapperId)
		blocks = append(blocks, &model.Block{
			Id:          wrapperId,
			ChildrenIds: []string{linkId},
			Content: &model.BlockContentOfWidget{Widget: &model.BlockContentWidget{
				Layout:    widgetLayoutNames.value(w.Layout),
				Limit:     w.Limit,
				ViewId:    w.ViewId,
				AutoAdded: w.AutoAdded,
			}},
		}, &model.Block{
			Id: linkId,
			// the target is the bundle's own object id, relinked on import like
			// every other reference (common.UpdateLinksToObjects); a reserved
			// listing is translated out of the format's `_` namespace into the
			// bare word the importer knows (WireWidgetTarget) and then passes
			// through untouched, because handleLinkBlock returns early for
			// widget.IsPredefinedWidgetTargetId. Anything else that does not
			// resolve is rewritten to _missing_object and then stripped — link
			// and wrapper both — by WidgetObject.Init, which is why
			// anyblockbatch.CheckIndexTargets rejects such a bundle up front.
			Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: WireWidgetTarget(w.Target),
				// Style's zero value, spelled out because this is the shape an
				// app export writes and the shape a reader will compare against
				Style:       model.BlockContentLink_Page,
				CardStyle:   cardStyleNames.value(w.CardStyle),
				IconSize:    iconSizeNames.value(w.IconSize),
				Description: linkDescriptionNames.value(w.Description),
				Relations:   w.Properties,
			}},
		})
	}

	details := map[string]*types.Value{
		detailKeyId: {Kind: &types.Value_StringValue{StringValue: WidgetsObjectId}},
		bundle.RelationKeyLayout.String(): {Kind: &types.Value_NumberValue{
			NumberValue: float64(model.ObjectType_dashboard)}},
		bundle.RelationKeyResolvedLayout.String(): {Kind: &types.Value_NumberValue{
			NumberValue: float64(model.ObjectType_dashboard)}},
		// the widget object is never listed anywhere as an object
		bundle.RelationKeyIsHidden.String(): {Kind: &types.Value_BoolValue{BoolValue: true}},
	}
	if len(idx.AutoWidgetTargets) > 0 {
		entries := make([]*types.Value, 0, len(idx.AutoWidgetTargets))
		for _, t := range idx.AutoWidgetTargets {
			entries = append(entries, &types.Value{Kind: &types.Value_StringValue{
				StringValue: WireWidgetTarget(t)}})
		}
		details[detailKeyAutoWidgetTargets] = &types.Value{Kind: &types.Value_ListValue{
			ListValue: &types.ListValue{Values: entries}}}
	}
	if idx.AutoWidgetDisabled {
		details[detailKeyAutoWidgetDisabled] = &types.Value{Kind: &types.Value_BoolValue{BoolValue: true}}
	}

	return &model.SmartBlockSnapshotBase{
		Blocks:      blocks,
		Details:     &types.Struct{Fields: details},
		ObjectTypes: []string{bundle.TypeKeyDashboard.URL()},
	}, nil
}

// widgetBlockId mints a link block's id: 24 hex characters, the shape
// bson.NewObjectId().Hex() gives every block id in an app export.
//
// Derived from the widget's position rather than drawn at random, because
// the rebuild is deterministic by design — re-converting an unchanged bundle
// produces identical bytes (see anyblockconvert's batch.optionLocalKey,
// which does the same for options). Seeding on the position rather than the
// target alone is what makes the ids unique even when a bundle lists the
// same target twice.
func widgetBlockId(i int, target string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("widget\x00%d\x00%s", i, target)))
	return hex.EncodeToString(sum[:])[:24]
}
