// Package resolve rewrites source-keyed references inside a built state to
// final object ids: link blocks, bookmark targets, text marks and icons,
// dataview targets, object-format detail values, collection membership and
// adopted relation/type keys. Unresolvable references become the explicit
// missing-object marker plus a structured issue — never a silent drop.
package resolve

import (
	"context"
	"fmt"
	"strings"

	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// RefResolver resolves a source key to a final object id, waiting on file
// futures. found=false for unknown keys; err means cancellation or a failed
// file upload. Implemented by identity.Service.ResolveRef.
type RefResolver interface {
	ResolveRef(ctx context.Context, sourceKey string) (id string, found bool, err error)
}

// KeyResolver maps a converter-emitted relation/type internal key onto the
// final key when identity matching adopted an existing object's key.
type KeyResolver interface {
	FinalKey(sourceKey string) (finalKey string, ok bool)
}

// FormatResolver reports the relation format for a key (streamed definitions
// first, bundled relations as fallback).
type FormatResolver interface {
	RelationFormat(key domain.RelationKey) (model.RelationFormat, bool)
}

type Resolver struct {
	refs    RefResolver
	keys    KeyResolver
	formats FormatResolver
}

func New(refs RefResolver, keys KeyResolver, formats FormatResolver) *Resolver {
	return &Resolver{refs: refs, keys: keys, formats: formats}
}

// RewriteState resolves every reference in st in place. Issues (missing
// targets, failed file references) go to report; the returned error is
// reserved for cancellation — a partial rewrite must not be persisted.
func (r *Resolver) RewriteState(ctx context.Context, st *state.State, report func(importv2.Issue)) error {
	r.adoptKeys(st)
	if err := r.rewriteBlocks(ctx, st, report); err != nil {
		return err
	}
	if err := r.rewriteDetailValues(ctx, st, report); err != nil {
		return err
	}
	return r.rewriteCollectionStore(ctx, st, report)
}

// resolveTarget implements the uniform reference policy: passthrough for
// bundled/date/predefined targets, index lookup (future-aware) otherwise,
// explicit missing marker plus issue for the rest.
func (r *Resolver) resolveTarget(ctx context.Context, target string, st *state.State, report func(importv2.Issue)) (string, error) {
	if target == "" || isPassthroughTarget(target) {
		return target, nil
	}
	id, found, err := r.refs.ResolveRef(ctx, target)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("resolve %q: %w", target, ctx.Err())
		}
		// The referenced file object failed: degrade this reference to the
		// missing marker, the file's own object error carries the cause.
		report(importv2.Issue{
			Severity:  importv2.SeverityWarning,
			Code:      importv2.IssueMissingTarget,
			SourceKey: target,
			ObjectId:  st.RootId(),
			Message:   "referenced file failed to import",
			Err:       err,
		})
		return addr.MissingObject, nil
	}
	if !found {
		report(missingTarget(target, st.RootId()))
		return addr.MissingObject, nil
	}
	return id, nil
}

func missingTarget(target, objectId string) importv2.Issue {
	return importv2.Issue{
		Severity:  importv2.SeverityWarning,
		Code:      importv2.IssueMissingTarget,
		SourceKey: target,
		ObjectId:  objectId,
		Message:   "reference target was not part of the import",
	}
}

func isPassthroughTarget(target string) bool {
	if widget.IsPredefinedWidgetTargetId(target) {
		return true
	}
	if typeKey, err := bundle.TypeKeyFromUrl(target); err == nil && bundle.HasObjectTypeByKey(typeKey) {
		return true
	}
	if relKey, err := pbtypes.RelationIdToKey(target); err == nil && bundle.HasRelation(domain.RelationKey(relKey)) {
		return true
	}
	return strings.HasPrefix(target, addr.DatePrefix)
}

func (r *Resolver) rewriteBlocks(ctx context.Context, st *state.State, report func(importv2.Issue)) error {
	var rewriteErr error
	iterErr := st.Iterate(func(bl simple.Block) bool {
		blockModel := bl.Model()
		var err error
		switch {
		case blockModel.GetLink() != nil:
			err = r.rewriteLinkBlock(ctx, blockModel, st, report)
		case blockModel.GetBookmark() != nil:
			err = r.rewriteBookmarkBlock(ctx, blockModel, st, report)
		case blockModel.GetText() != nil:
			err = r.rewriteTextBlock(ctx, blockModel, st, report)
		case blockModel.GetDataview() != nil:
			err = r.rewriteDataviewBlock(ctx, blockModel, st, report)
		case blockModel.GetFile() != nil:
			err = r.rewriteFileBlock(ctx, blockModel, st, report)
		}
		if err != nil {
			rewriteErr = err
			return false
		}
		return true
	})
	if rewriteErr != nil {
		return rewriteErr
	}
	if iterErr != nil {
		return fmt.Errorf("iterate blocks: %w", iterErr)
	}
	return nil
}

func (r *Resolver) rewriteLinkBlock(ctx context.Context, block *model.Block, st *state.State, report func(importv2.Issue)) error {
	target := block.GetLink().TargetBlockId
	resolved, err := r.resolveTarget(ctx, target, st, report)
	if err != nil {
		return err
	}
	if resolved != target {
		block.GetLink().TargetBlockId = resolved
		st.Set(simple.New(block))
	}
	return nil
}

func (r *Resolver) rewriteBookmarkBlock(ctx context.Context, block *model.Block, st *state.State, report func(importv2.Issue)) error {
	target := block.GetBookmark().TargetObjectId
	if target == "" {
		return nil // fetched post-create from the URL
	}
	resolved, err := r.resolveTarget(ctx, target, st, report)
	if err != nil {
		return err
	}
	if resolved != target {
		block.GetBookmark().TargetObjectId = resolved
		st.Set(simple.New(block))
	}
	return nil
}

func (r *Resolver) rewriteTextBlock(ctx context.Context, block *model.Block, st *state.State, report func(importv2.Issue)) error {
	text := block.GetText()
	changed := false
	if iconImage := text.GetIconImage(); iconImage != "" && !isValidCid(iconImage) {
		resolved, err := r.resolveTarget(ctx, iconImage, st, report)
		if err != nil {
			return err
		}
		if resolved != iconImage {
			text.IconImage = resolved
			changed = true
		}
	}
	for _, mark := range text.GetMarks().GetMarks() {
		if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
			continue
		}
		// v1 returned from the whole loop on a bundled target, leaving every
		// later mark in the block unresolved; each mark is independent here.
		resolved, err := r.resolveTarget(ctx, mark.Param, st, report)
		if err != nil {
			return err
		}
		if resolved != mark.Param {
			mark.Param = resolved
			changed = true
		}
	}
	if changed {
		st.Set(simple.New(block))
	}
	return nil
}

func (r *Resolver) rewriteFileBlock(ctx context.Context, block *model.Block, st *state.State, report func(importv2.Issue)) error {
	file := block.GetFile()
	changed := false
	if file.TargetObjectId != "" && !isValidCid(file.TargetObjectId) {
		resolved, err := r.resolveTarget(ctx, file.TargetObjectId, st, report)
		if err != nil {
			return err
		}
		if resolved != file.TargetObjectId {
			file.TargetObjectId = resolved
			changed = true
		}
	}
	if hash := file.GetHash(); hash != "" {
		if id, found, err := r.refs.ResolveRef(ctx, hash); err == nil && found {
			file.TargetObjectId = id
			changed = true
		}
	}
	if changed {
		st.Set(simple.New(block))
	}
	return nil
}

func (r *Resolver) rewriteDataviewBlock(ctx context.Context, block *model.Block, st *state.State, report func(importv2.Issue)) error {
	dataview := block.GetDataview()
	if target := dataview.TargetObjectId; target != "" {
		resolved, err := r.resolveTarget(ctx, target, st, report)
		if err != nil {
			return err
		}
		dataview.TargetObjectId = resolved
	}
	for _, view := range dataview.GetViews() {
		for _, filter := range view.GetFilters() {
			r.rewriteFilterValue(ctx, filter)
			filter.RelationKey = r.finalKey(filter.RelationKey)
		}
		if view.DefaultTemplateId != "" {
			view.DefaultTemplateId = r.resolveOrClear(ctx, view.DefaultTemplateId, st, report)
		}
		if view.DefaultObjectTypeId != "" {
			view.DefaultObjectTypeId = r.resolveOrClear(ctx, view.DefaultObjectTypeId, st, report)
		}
		for _, relation := range view.Relations {
			relation.Key = r.finalKey(relation.Key)
		}
		for _, sort := range view.GetSorts() {
			sort.RelationKey = r.finalKey(sort.RelationKey)
		}
	}
	// GroupOrders concatenate ids and only occur in anytype exports; they are
	// out of scope until the pb converter moves to v2.
	for _, order := range dataview.GetObjectOrders() {
		for i, objectId := range order.ObjectIds {
			if id, found, err := r.refs.ResolveRef(ctx, objectId); err == nil && found {
				order.ObjectIds[i] = id
			}
		}
	}
	for _, relationLink := range dataview.GetRelationLinks() {
		relationLink.Key = r.finalKey(relationLink.Key)
	}
	st.Set(simple.New(block))
	return nil
}

// rewriteFilterValue keeps unresolved filter values as-is (v1 semantics):
// filter values are frequently plain scalars, not references.
func (r *Resolver) rewriteFilterValue(ctx context.Context, filter *model.BlockContentDataviewFilter) {
	if ids := pbtypes.GetStringListValue(filter.Value); len(ids) > 0 {
		newIds := make([]string, 0, len(ids))
		for _, objectId := range ids {
			if id, found, err := r.refs.ResolveRef(ctx, objectId); err == nil && found {
				newIds = append(newIds, id)
			} else {
				newIds = append(newIds, objectId)
			}
		}
		filter.Value = pbtypes.StringList(newIds)
		return
	}
	if objectId := filter.Value.GetStringValue(); objectId != "" {
		if id, found, err := r.refs.ResolveRef(ctx, objectId); err == nil && found {
			filter.Value = pbtypes.String(id)
		}
	}
}

func (r *Resolver) resolveOrClear(ctx context.Context, target string, st *state.State, report func(importv2.Issue)) string {
	id, found, err := r.refs.ResolveRef(ctx, target)
	if err != nil || !found {
		report(missingTarget(target, st.RootId()))
		return ""
	}
	return id
}

func (r *Resolver) finalKey(sourceKey string) string {
	if finalKey, ok := r.keys.FinalKey(sourceKey); ok {
		return finalKey
	}
	return sourceKey
}

func isValidCid(value string) bool {
	_, err := cid.Decode(value)
	return err == nil
}
