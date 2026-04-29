package objectlink

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
)

var log = logging.Logger("objectlink")

type (
	KeyToIDConverter interface {
		GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error)
		GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error)
		Id() string
	}

	linkSource interface {
		FillSmartIds(ids []string) []string
		HasSmartIds() bool
	}

	spaceIdResolver interface {
		ResolveSpaceID(id string) (spaceId string, err error)
	}
)

type Flags struct {
	Blocks,
	Details,
	Relations,
	Types,
	Collection,
	CreatorModifierWorkspace,
	DataviewBlockOnlyTarget,
	NoSystemRelations,
	NoHiddenBundledRelations,
	NoImages,
	RoundDateIdsToDay,
	// FilterPresentationOnly drops targets that appear only via presentation relations
	// (iconImage, picture, fileId, coverId). Targets referenced from any other source
	// (block, non-presentation relation, store slice) are kept.
	FilterPresentationOnly,
	NoBackLinks bool
}

// presentationOnlyRelations are relations whose values are presentation references
// (icon, picture, cover, file binding), not data connections. Targets that appear
// only via these relations are filtered out by FilterPresentationOnly.
var presentationOnlyRelations = []domain.RelationKey{
	bundle.RelationKeyIconImage,
	bundle.RelationKeyPicture,
	bundle.RelationKeyFileId,
	bundle.RelationKeyCoverId,
}

// OutgoingLink represents a link from one object to another, with optional source attribution.
// SourceBlockID is set when the link originates from a block; RelationKey is set when it
// originates from a relation; both are empty for collection-store membership links.
type OutgoingLink struct {
	TargetID      string
	SourceBlockID string
	RelationKey   string
}

// DependentObjectLinks returns outgoing links from a state with per-source attribution
// (SourceBlockID for block-derived links, RelationKey for relation-derived links).
// Same Flags semantics as DependentObjectIDs. Targets are deduplicated by ID — the first
// emitter wins, so block sources take precedence over relations and store slice when an
// ID appears more than once.
func DependentObjectLinks(s *state.State, converter KeyToIDConverter, fetcher relationutils.RelationFormatFetcher, flags Flags) []OutgoingLink {
	var (
		links  []OutgoingLink
		seen   = make(map[string]struct{})
		selfId = s.RootId()
	)
	emit := func(link OutgoingLink) bool {
		if link.TargetID == "" || link.TargetID == selfId {
			return true
		}
		if _, ok := seen[link.TargetID]; ok {
			return true
		}
		seen[link.TargetID] = struct{}{}
		links = append(links, link)
		return true
	}

	if flags.Blocks {
		visitBlockLinks(s, flags, emit)
	}
	if flags.Types {
		visitTypeLinks(s, converter, emit)
	}
	visitRelationLinks(s, converter, fetcher, flags, emit)
	if flags.Collection {
		visitCollectionStoreLinks(s, emit)
	}

	if flags.RoundDateIdsToDay {
		for i := range links {
			rounded := roundDateIds([]string{links[i].TargetID})
			if len(rounded) == 1 {
				links[i].TargetID = rounded[0]
			}
		}
		// Two raw date IDs that round to the same day-id would otherwise survive as
		// duplicates in the result. Match the pre-refactor DependentObjectIDs path,
		// which dedups via lo.Uniq AFTER rounding.
		links = dedupOutgoingLinks(links)
	}
	if flags.FilterPresentationOnly {
		links = filterPresentationOnly(s, links)
	}
	return links
}

// dedupOutgoingLinks keeps the first occurrence of each TargetID. Stable.
func dedupOutgoingLinks(links []OutgoingLink) []OutgoingLink {
	if len(links) <= 1 {
		return links
	}
	seen := make(map[string]struct{}, len(links))
	out := links[:0]
	for _, l := range links {
		if _, ok := seen[l.TargetID]; ok {
			continue
		}
		seen[l.TargetID] = struct{}{}
		out = append(out, l)
	}
	return out
}

// filterPresentationOnly removes target IDs that appear as values of presentation
// relations (icon, picture, cover, fileId), matching the pre-GO-6052 behavior of
// injectLinksDetails: a target referenced by any presentation relation is dropped
// from the link set — even if it is also referenced from a block or another relation.
// This is intentional: presentation references inflate backlinks (1000 objects sharing
// an avatar would all backlink that file).
func filterPresentationOnly(s *state.State, links []OutgoingLink) []OutgoingLink {
	if len(links) == 0 || s.Details() == nil {
		return links
	}

	presentationTargets := map[string]struct{}{}
	for _, key := range presentationOnlyRelations {
		if !s.Details().Has(key) {
			continue
		}
		val := s.Details().Get(key)
		if str, ok := val.TryString(); ok && str != "" {
			presentationTargets[str] = struct{}{}
		}
		if list, ok := val.TryStringList(); ok {
			for _, v := range list {
				if v != "" {
					presentationTargets[v] = struct{}{}
				}
			}
		}
	}
	if len(presentationTargets) == 0 {
		return links
	}

	out := links[:0]
	for _, l := range links {
		if _, drop := presentationTargets[l.TargetID]; drop {
			continue
		}
		out = append(out, l)
	}
	return out
}

func DependentObjectIDs(s *state.State, converter KeyToIDConverter, fetcher relationutils.RelationFormatFetcher, flags Flags) (ids []string) {
	// When FilterPresentationOnly is requested we need attribution to distinguish
	// presentation-only references from genuine data links — route through the attributed
	// walker, project to []string, return.
	if flags.FilterPresentationOnly {
		links := DependentObjectLinks(s, converter, fetcher, flags)
		out := make([]string, 0, len(links))
		for _, l := range links {
			out = append(out, l.TargetID)
		}
		return out
	}

	collect := func(link OutgoingLink) bool {
		ids = append(ids, link.TargetID)
		return true
	}

	if flags.Blocks {
		visitBlockLinks(s, flags, collect)
	}
	if flags.Types {
		visitTypeLinks(s, converter, collect)
	}
	visitRelationLinks(s, converter, fetcher, flags, collect)
	if flags.Collection {
		visitCollectionStoreLinks(s, collect)
	}

	if flags.RoundDateIdsToDay {
		ids = roundDateIds(ids)
	}

	ids = lo.Uniq(ids)
	return
}

func DependentObjectIDsPerSpace(
	rootSpaceId string,
	s *state.State,
	converter KeyToIDConverter,
	resolver spaceIdResolver,
	formatFetcher relationutils.RelationFormatFetcher,
	flags Flags,
) map[string][]string {
	ids := DependentObjectIDs(s, converter, formatFetcher, flags)
	perSpace := map[string][]string{}
	for _, id := range ids {
		if dateObject, parseErr := dateutil.BuildDateObjectFromId(id); parseErr == nil {
			perSpace[rootSpaceId] = append(perSpace[rootSpaceId], dateObject.Id())
			continue
		}

		spaceId, err := resolver.ResolveSpaceID(id)
		if errors.Is(err, domain.ErrObjectNotFound) {
			perSpace[rootSpaceId] = append(perSpace[rootSpaceId], id)
			continue
		}

		if err != nil {
			perSpace[rootSpaceId] = append(perSpace[rootSpaceId], id)
			log.With("id", id).Warn("resolve space id", zap.Error(err))
			continue
		}
		perSpace[spaceId] = append(perSpace[spaceId], id)
	}
	return perSpace
}

// visitBlockLinks calls emit for every outgoing-link target found in blocks. Each emitted
// record carries SourceBlockID = block.Model().Id. emit returning false stops iteration.
func visitBlockLinks(s *state.State, flags Flags, emit func(OutgoingLink) bool) {
	err := s.Iterate(func(b simple.Block) (isContinue bool) {
		blockId := b.Model().Id

		if flags.DataviewBlockOnlyTarget {
			if dv := b.Model().GetDataview(); dv != nil {
				if dv.TargetObjectId != "" {
					if !emit(OutgoingLink{TargetID: dv.TargetObjectId, SourceBlockID: blockId}) {
						return false
					}
				}
				return true
			}
		}

		// if NoImages == false, then file block will be processed with FillSmartIds
		if flags.NoImages {
			if f := b.Model().GetFile(); f != nil {
				if f.TargetObjectId != "" && f.Type != model.BlockContentFile_Image {
					if !emit(OutgoingLink{TargetID: f.TargetObjectId, SourceBlockID: blockId}) {
						return false
					}
				}
				return true
			}
		}

		if ls, ok := b.(linkSource); ok {
			for _, id := range ls.FillSmartIds(nil) {
				if !emit(OutgoingLink{TargetID: id, SourceBlockID: blockId}) {
					return false
				}
			}
		}
		return true
	})
	if err != nil {
		log.With("objectID", s.RootId()).Errorf("failed to iterate over simple blocks: %s", err)
	}
}

// visitTypeLinks calls emit for the object's type IDs (no source attribution).
func visitTypeLinks(s *state.State, converter KeyToIDConverter, emit func(OutgoingLink) bool) {
	for _, objectTypeKey := range s.ObjectTypeKeys() {
		if objectTypeKey == "" { // TODO is it possible?
			log.Errorf("sb %s has empty ot", s.RootId())
			continue
		}
		id, err := converter.GetTypeIdByKey(context.Background(), objectTypeKey)
		if err != nil {
			log.With("objectID", s.RootId()).Errorf("failed to get object type id by key %s: %s", objectTypeKey, err)
			continue
		}
		if !emit(OutgoingLink{TargetID: id}) {
			return
		}
	}
}

// visitRelationLinks calls emit for every outgoing-link target found in object relations.
// Each record carries RelationKey = relation key string. Honors the Flags filters.
func visitRelationLinks(s *state.State, converter KeyToIDConverter, fetcher relationutils.RelationFormatFetcher, flags Flags, emit func(OutgoingLink) bool) {
	var det *domain.Details
	if flags.Details {
		det = s.CombinedDetails()
	}

	// Sort relation keys for deterministic emission order. The pre-GO-6052
	// collectOutgoingLinks used Details().IterateSorted(); without sorting here, two
	// consecutive runs over the same state would produce OutgoingLinks in random order
	// (Go map iteration), causing spurious isDetailedLinksChanged churn in the indexer.
	keys := s.AllRelationKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, key := range keys {
		if flags.Relations {
			id, err := converter.GetRelationIdByKey(context.Background(), key)
			if err != nil {
				// Match pre-refactor DependentObjectIDs: if we can't resolve the relation
				// schema id, skip the entire iteration for this key — don't fall through
				// to details extraction.
				log.With("objectID", s.RootId()).Errorf("failed to get relation id by key %s: %s", key, err)
				continue
			}
			if !emit(OutgoingLink{TargetID: id, RelationKey: key.String()}) {
				return
			}
		}

		if !flags.Details {
			continue
		}

		format, err := fetcher.GetRelationFormatByKey(converter.Id(), key)
		if err != nil {
			// let's suppose relation has an object format, so we don't miss dependencies
			format = model.RelationFormat_object
		}

		for _, id := range collectIdsFromDetail(&model.RelationLink{Key: key.String(), Format: format}, det, flags) {
			if !emit(OutgoingLink{TargetID: id, RelationKey: key.String()}) {
				return
			}
		}
	}
}

// visitCollectionStoreLinks calls emit for every member of the collection's StoreSlice
// (no SourceBlockID, no RelationKey).
func visitCollectionStoreLinks(s *state.State, emit func(OutgoingLink) bool) {
	for _, id := range s.GetStoreSlice(template.CollectionStoreKey) {
		if id == "" {
			continue
		}
		if !emit(OutgoingLink{TargetID: id}) {
			return
		}
	}
}

func collectIdsFromDetail(rel *model.RelationLink, det *domain.Details, flags Flags) (ids []string) {
	if flags.NoSystemRelations {
		if rel.Format != model.RelationFormat_object || bundle.IsSystemRelation(domain.RelationKey(rel.Key)) {
			return
		}
	}

	if flags.NoHiddenBundledRelations {
		// Only bundled relations can be hidden, so we don't need to request relations from object store.
		if r, err := bundle.GetRelation(domain.RelationKey(rel.Key)); err == nil && r.Hidden {
			return
		}
	}

	if rel.Key == bundle.RelationKeyBacklinks.String() && flags.NoBackLinks {
		return
	}

	// handle corner cases first for specific formats
	if rel.Format == model.RelationFormat_date &&
		!lo.Contains(bundle.LocalAndDerivedRelationKeys, domain.RelationKey(rel.Key)) {
		relInt := det.GetInt64(domain.RelationKey(rel.Key))
		if relInt > 0 {
			t := time.Unix(relInt, 0)
			t = t.In(time.Local)
			ids = append(ids, dateutil.NewDateObject(t, false).Id())
		}
		return
	}

	if rel.Key == bundle.RelationKeyCreator.String() ||
		rel.Key == bundle.RelationKeyLastModifiedBy.String() {
		if flags.CreatorModifierWorkspace {
			v := det.GetString(domain.RelationKey(rel.Key))
			ids = append(ids, v)
		}
		return
	}

	if rel.Key == bundle.RelationKeyId.String() ||
		rel.Key == bundle.RelationKeyLinks.String() ||
		rel.Key == bundle.RelationKeyType.String() || // always skip type because it was processed before
		rel.Key == bundle.RelationKeyFeaturedRelations.String() {
		return
	}

	if rel.Key == bundle.RelationKeyCoverId.String() {
		v := det.GetString(domain.RelationKey(rel.Key))
		_, err := cid.Decode(v)
		if err != nil {
			// this is an exception cause coverId can contain not a file hash but color
			return
		}
		ids = append(ids, v)
	}

	if rel.Format != model.RelationFormat_object &&
		rel.Format != model.RelationFormat_file &&
		rel.Format != model.RelationFormat_status &&
		rel.Format != model.RelationFormat_tag {
		return
	}

	// add all object relation values as dependents
	for _, targetID := range det.GetStringList(domain.RelationKey(rel.Key)) {
		if targetID != "" {
			ids = append(ids, targetID)
		}
	}

	return ids
}

// roundDateIds turns all date object ids into ids with no time included
func roundDateIds(ids []string) []string {
	for i, id := range ids {
		dateObject, err := dateutil.BuildDateObjectFromId(id)
		if err != nil {
			continue
		}

		ids[i] = dateutil.NewDateObject(dateObject.Time(), false).Id()
	}
	return ids
}
