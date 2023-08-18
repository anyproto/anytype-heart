package objectlink

import (
	"context"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-cid"
	"github.com/samber/lo"
	"time"
)

var log = logging.Logger("objectlink")

type KeyToIDConverter interface {
	GetRelationIdByKey(ctx context.Context, spaceId string, key bundle.RelationKey) (id string, err error)
	GetTypeIdByKey(ctx context.Context, spaceId string, key bundle.TypeKey) (id string, err error)
}

type linkSource interface {
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
}

func DependentObjectIDs(s *state.State, converter KeyToIDConverter, blocks, details, relations, objTypes, creatorModifierWorkspace bool) (ids []string) {
	if blocks {
		err := s.Iterate(func(b simple.Block) (isContinue bool) {
			if ls, ok := b.(linkSource); ok {
				ids = ls.FillSmartIds(ids)
			}
			return true
		})
		if err != nil {
			log.With("objectID", s.RootId()).Errorf("failed to iterate over simple blocks: %s", err)
		}
	}

	if objTypes {
		for _, objectTypeKey := range s.ObjectTypeKeys() {
			if objectTypeKey == "" { // TODO is it possible?
				log.Errorf("sb %s has empty ot", s.RootId())
				continue
			}
			id, err := converter.GetTypeIdByKey(context.Background(), s.SpaceID(), bundle.TypeKey(objectTypeKey))
			if err != nil {
				log.With("objectID", s.RootId()).Errorf("failed to get object type id by key %s: %s", objectTypeKey, err)
				continue
			}
			ids = append(ids, id)
		}
	}

	var det *types.Struct
	if details {
		det = s.CombinedDetails()
	}

	for _, rel := range s.GetRelationLinks() {
		// do not index local dates such as lastOpened/lastModified
		if relations {
			id, err := converter.GetRelationIdByKey(context.Background(), s.SpaceID(), bundle.RelationKey(rel.Key))
			if err != nil {
				log.With("objectID", s.RootId()).Errorf("failed to get relation id by key %s: %s", rel.Key, err)
				continue
			}
			ids = append(ids, id)
		}

		if !details {
			continue
		}

		// handle corner cases first for specific formats
		if rel.Format == model.RelationFormat_date &&
			!lo.Contains(bundle.LocalRelationsKeys, rel.Key) &&
			!lo.Contains(bundle.DerivedRelationsKeys, rel.Key) {
			relInt := pbtypes.GetInt64(det, rel.Key)
			if relInt > 0 {
				t := time.Unix(relInt, 0)
				t = t.In(time.UTC)
				ids = append(ids, addr.TimeToID(t))
			}
			continue
		}

		if rel.Key == bundle.RelationKeyCreator.String() ||
			rel.Key == bundle.RelationKeyLastModifiedBy.String() ||
			rel.Key == bundle.RelationKeyWorkspaceId.String() {
			if creatorModifierWorkspace {
				v := pbtypes.GetString(det, rel.Key)
				ids = append(ids, v)
			}
			continue
		}

		if rel.Key == bundle.RelationKeyId.String() ||
			rel.Key == bundle.RelationKeyLinks.String() ||
			rel.Key == bundle.RelationKeyType.String() || // always skip type because it was proceed above
			rel.Key == bundle.RelationKeyFeaturedRelations.String() {
			continue
		}

		if rel.Key == bundle.RelationKeyCoverId.String() {
			v := pbtypes.GetString(det, rel.Key)
			_, err := cid.Decode(v)
			if err != nil {
				// this is an exception cause coverId can contains not a file hash but color
				continue
			}
			ids = append(ids, v)
		}

		if rel.Format != model.RelationFormat_object &&
			rel.Format != model.RelationFormat_file &&
			rel.Format != model.RelationFormat_status &&
			rel.Format != model.RelationFormat_tag {
			continue
		}

		// add all object relation values as dependents
		for _, targetID := range pbtypes.GetStringList(det, rel.Key) {
			if targetID == "" {
				continue
			}

			ids = append(ids, targetID)
		}
	}

	ids = lo.Uniq(ids)
	return
}
