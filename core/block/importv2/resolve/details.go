package resolve

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// adoptKeys rewrites detail keys, relation-link keys and the object type key
// when identity matching adopted an existing object's internal key (v1's
// updateKeys). Runs before value rewriting so formats resolve on final keys.
func (r *Resolver) adoptKeys(st *state.State) {
	adopted := map[string]string{}
	for key := range st.Details().Iterate() {
		if finalKey, ok := r.keys.FinalKey(string(key)); ok && finalKey != string(key) {
			adopted[string(key)] = finalKey
		}
	}
	for _, link := range st.PickRelationLinks() {
		if finalKey, ok := r.keys.FinalKey(link.Key); ok && finalKey != link.Key {
			adopted[link.Key] = finalKey
		}
	}
	for oldKey, finalKey := range adopted {
		if value, ok := st.Details().TryGet(domain.RelationKey(oldKey)); ok {
			st.SetDetail(domain.RelationKey(finalKey), value)
		}
		if format, ok := r.relationFormatOf(st, domain.RelationKey(oldKey)); ok {
			st.AddRelationLinks(&model.RelationLink{Key: finalKey, Format: format})
		}
		st.RemoveRelation(domain.RelationKey(oldKey))
	}
	if finalKey, ok := r.keys.FinalKey(st.ObjectTypeKey().String()); ok && finalKey != st.ObjectTypeKey().String() {
		st.SetObjectTypeKey(domain.TypeKey(finalKey))
	}
}

// relationFormatOf resolves a relation's format: the object's own relation
// links first (per-page truth — the same property name can carry different
// inferred formats on different pages), then the run-wide registry.
func (r *Resolver) relationFormatOf(st *state.State, key domain.RelationKey) (model.RelationFormat, bool) {
	for _, link := range st.PickRelationLinks() {
		if link.Key == key.String() {
			return link.Format, true
		}
	}
	if format, ok := r.formats.RelationFormat(key); ok {
		return format, true
	}
	return 0, false
}

// rewriteDetailValues remaps object-format detail values (object, tag,
// status, file formats plus the coverId special case, excluding
// featuredRelations which hold keys). Unresolved values keep v1's leniency:
// bundled/date targets pass through, everything else is kept as-is — detail
// values are frequently plain scalars (colors, names), not references.
func (r *Resolver) rewriteDetailValues(ctx context.Context, st *state.State, report func(importv2.Issue)) error {
	for key, value := range st.Details().Iterate() {
		if !r.isObjectValued(st, key) {
			continue
		}
		if single, ok := value.TryString(); ok {
			resolved, err := r.resolveDetailValue(ctx, single)
			if err != nil {
				return fmt.Errorf("detail %q: %w", key, err)
			}
			if resolved != single {
				st.SetDetail(key, domain.String(resolved))
			}
			continue
		}
		values := value.StringList()
		changed := false
		for i, item := range values {
			resolved, err := r.resolveDetailValue(ctx, item)
			if err != nil {
				return fmt.Errorf("detail %q: %w", key, err)
			}
			if resolved != item {
				values[i] = resolved
				changed = true
			}
		}
		if changed {
			st.SetDetail(key, domain.StringList(values))
		}
	}
	return nil
}

// resolveDetailValue maps one detail value: resolved reference → final id;
// failed file reference → missing marker; unknown value → kept as-is.
func (r *Resolver) resolveDetailValue(ctx context.Context, value string) (string, error) {
	if value == "" || isPassthroughTarget(value) {
		return value, nil
	}
	id, found, err := r.refs.ResolveRef(ctx, value)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("resolve %q: %w", value, ctx.Err())
		}
		return addr.MissingObject, nil
	}
	if !found {
		return value, nil
	}
	return id, nil
}

// rewriteCreatedInContext maps the source key a converter recorded as the
// object's parent onto the parent's final id. Unlike other object-valued
// details, a parent that does not resolve takes the pair with it: object GC
// matches createdInContext by equality, so a leftover source key or the
// missing-object marker would be a context no object can ever have.
func (r *Resolver) rewriteCreatedInContext(ctx context.Context, st *state.State) error {
	parent := st.Details().GetString(bundle.RelationKeyCreatedInContext)
	if parent == "" {
		st.RemoveDetail(bundle.RelationKeyCreatedInContext, bundle.RelationKeyCreatedInContextRef)
		return nil
	}
	id, found, err := r.refs.ResolveRef(ctx, parent)
	if err != nil && ctx.Err() != nil {
		return fmt.Errorf("resolve created-in context %q: %w", parent, ctx.Err())
	}
	if err != nil || !found || id == st.RootId() {
		st.RemoveDetail(bundle.RelationKeyCreatedInContext, bundle.RelationKeyCreatedInContextRef)
		return nil
	}
	st.SetDetail(bundle.RelationKeyCreatedInContext, domain.String(id))
	if matched, ok := r.refs.(MatchReporter); ok && matched.IsMatched(parent) {
		// A matched parent is updated in place, and that update may never
		// land (revision guard, a failed reset degrading to skipped): the
		// imported link block the ref names is not guaranteed to exist. A
		// ref is ownership object GC acts on, so it must not outrun its block.
		st.RemoveDetail(bundle.RelationKeyCreatedInContextRef)
	}
	return nil
}

// MatchReporter is optionally implemented by the RefResolver
// (identity.Service): whether a source key matched a pre-existing object.
type MatchReporter interface {
	IsMatched(sourceKey string) bool
}

func (r *Resolver) isObjectValued(st *state.State, key domain.RelationKey) bool {
	if key == bundle.RelationKeyFeaturedRelations || key == bundle.RelationKeyCreatedInContext {
		// featuredRelations holds keys, not references; createdInContext is
		// rewritten by rewriteCreatedInContext.
		return false
	}
	if key == bundle.RelationKeyCoverId {
		return true // cover is either a color name or an image reference
	}
	format, ok := r.relationFormatOf(st, key)
	if !ok {
		return false
	}
	switch format {
	case model.RelationFormat_object, model.RelationFormat_tag, model.RelationFormat_status, model.RelationFormat_file:
		return true
	default:
		return false
	}
}

// rewriteCollectionStore remaps collection membership from source keys to
// final ids. Unresolved members are dropped with an issue: replace semantics
// mean the list must contain only real ids.
func (r *Resolver) rewriteCollectionStore(ctx context.Context, st *state.State, report func(importv2.Issue)) error {
	if st.Store() == nil {
		return nil
	}
	members := st.GetStoreSlice(template.CollectionStoreKey)
	if len(members) == 0 {
		return nil
	}
	resolved := make([]string, 0, len(members))
	for _, member := range members {
		id, found, err := r.refs.ResolveRef(ctx, member)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("collection member %q: %w", member, ctx.Err())
			}
			found = false
		}
		if !found {
			report(missingTarget(member, st.RootId()))
			continue
		}
		resolved = append(resolved, id)
	}
	st.UpdateStoreSlice(template.CollectionStoreKey, resolved)
	return nil
}
