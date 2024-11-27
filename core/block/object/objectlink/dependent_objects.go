package objectlink

import (
	"context"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-cid"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("objectlink")

type KeyToIDConverter interface {
	GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error)
	GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error)
}

type linkSource interface {
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
}

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
	UnifyDateObjectIds bool
}

func DependentObjectIDs(s *state.State, converter KeyToIDConverter, flags Flags) (ids []string) {
	if flags.Blocks {
		ids = collectIdsFromBlocks(s, flags)
	}

	if flags.Types {
		ids = append(ids, collectIdsFromTypes(s, converter)...)
	}

	var det *types.Struct
	if flags.Details {
		det = s.CombinedDetails()
	}

	for _, rel := range s.GetRelationLinks() {
		if flags.Relations {
			id, err := converter.GetRelationIdByKey(context.Background(), domain.RelationKey(rel.Key))
			if err != nil {
				log.With("objectID", s.RootId()).Errorf("failed to get relation id by key %s: %s", rel.Key, err)
				continue
			}
			ids = append(ids, id)
		}

		if !flags.Details {
			continue
		}

		ids = append(ids, collectIdsFromDetail(rel, det, flags)...)
	}

	if flags.Collection {
		ids = append(ids, s.GetStoreSlice(template.CollectionStoreKey)...)
	}

	if flags.UnifyDateObjectIds {
		ids = unifyDateObjectIds(ids)
	}

	ids = lo.Uniq(ids)
	return
}

func collectIdsFromBlocks(s *state.State, flags Flags) (ids []string) {
	err := s.Iterate(func(b simple.Block) (isContinue bool) {
		if flags.DataviewBlockOnlyTarget {
			if dv := b.Model().GetDataview(); dv != nil {
				if dv.TargetObjectId != "" {
					ids = append(ids, dv.TargetObjectId)
				}
				return true
			}
		}

		// if NoImages == false, then file block will be processed with FillSmartIds
		if flags.NoImages {
			if f := b.Model().GetFile(); f != nil {
				if f.TargetObjectId != "" && f.Type != model.BlockContentFile_Image {
					ids = append(ids, f.TargetObjectId)
				}
				return true
			}
		}

		if ls, ok := b.(linkSource); ok {
			ids = ls.FillSmartIds(ids)
		}
		return true
	})
	if err != nil {
		log.With("objectID", s.RootId()).Errorf("failed to iterate over simple blocks: %s", err)
	}
	return ids
}

func collectIdsFromTypes(s *state.State, converter KeyToIDConverter) (ids []string) {
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
		ids = append(ids, id)
	}
	return ids
}

func collectIdsFromDetail(rel *model.RelationLink, det *types.Struct, flags Flags) (ids []string) {
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

	// handle corner cases first for specific formats
	if rel.Format == model.RelationFormat_date &&
		!lo.Contains(bundle.LocalAndDerivedRelationKeys, rel.Key) {
		relInt := pbtypes.GetInt64(det, rel.Key)
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
			v := pbtypes.GetString(det, rel.Key)
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
		v := pbtypes.GetString(det, rel.Key)
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
	for _, targetID := range pbtypes.GetStringList(det, rel.Key) {
		if targetID != "" {
			ids = append(ids, targetID)
		}
	}

	return ids
}

// unifyDateObjectIds turns all date object ids into ids with no time included
func unifyDateObjectIds(ids []string) []string {
	for i, id := range ids {
		dateObject, err := dateutil.BuildDateObjectFromId(id)
		if err != nil {
			continue
		}

		ids[i] = dateutil.NewDateObject(dateObject.Time(), false).Id()
	}
	return ids
}
