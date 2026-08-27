package objectlink

import (
	"context"
	"errors"
	"slices"
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
	NoBackLinks bool
}

func DependentObjectIDs(s *state.State, converter KeyToIDConverter, fetcher relationutils.RelationFormatFetcher, flags Flags) (ids []string) {
	// TODO Blocks is always true
	if flags.Blocks {
		ids = collectIdsFromBlocks(s, flags)
	}

	if flags.Types {
		ids = append(ids, collectIdsFromTypes(s, converter)...)
	}

	var det *domain.Details
	if flags.Details {
		det = s.CombinedDetails()
	}

	for _, key := range s.AllRelationKeys() {
		if flags.Relations {
			id, err := converter.GetRelationIdByKey(context.Background(), key)
			if err != nil {
				log.With("objectID", s.RootId()).Errorf("failed to get relation id by key %s: %s", key, err)
				continue
			}
			ids = append(ids, id)
		}

		if !flags.Details {
			continue
		}

		format, err := fetcher.GetRelationFormatByKey(converter.Id(), key)
		if err != nil {
			// let's suppose relation has an object format, so we don't miss dependencies
			format = model.RelationFormat_object
		}

		ids = append(ids, collectIdsFromDetail(&model.RelationLink{Key: key.String(), Format: format}, det, flags)...)
	}

	if flags.Collection {
		ids = append(ids, s.GetStoreSlice(template.CollectionStoreKey)...)
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

// relationsExcludedFromDeps lists object/file-format relations whose value must never be collected
// as a dependent object. Three kinds live here:
//   - references outside this space's graph (spaceId, sourceObject pointing at a bundled or
//     marketplace object, identityProfileLink);
//   - infrastructure pointers the client never renders as a chip (chatId, discussionId,
//     spaceDashboardId, targetObjectType, defaultTypeId, defaultTemplateId);
//   - file ids rendered straight from the id rather than from the file object's record
//     (iconImage, coverId, picture).
//
// The list only guards the generic relation-value walk at the bottom of collectIdsFromDetail. The
// explicit branches above it (creator/lastModifiedBy behind CreatorModifierWorkspace, and coverId's
// CID check) keep their own behaviour — coverId is listed here so the generic walk never picks it
// up, not to disable that branch.
var relationsExcludedFromDeps = []domain.RelationKey{
	bundle.RelationKeySpaceId,
	bundle.RelationKeySourceObject,
	bundle.RelationKeyIdentityProfileLink,
	bundle.RelationKeyChatId,
	bundle.RelationKeyDiscussionId,
	bundle.RelationKeySpaceDashboardId,
	bundle.RelationKeyTargetObjectType,
	bundle.RelationKeyDefaultTypeId,
	bundle.RelationKeyDefaultTemplateId,
	bundle.RelationKeyIconImage,
	bundle.RelationKeyCoverId,
	bundle.RelationKeyPicture,
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

	if slices.Contains(relationsExcludedFromDeps, domain.RelationKey(rel.Key)) {
		return
	}

	// add all object relation values as dependents. WrapToStringList, not GetStringList: a
	// maxCount=1 relation (createdInContext, and every single-value object/tag/status relation)
	// holds a plain string, which GetStringList silently reports as no value at all.
	for _, targetID := range det.WrapToStringList(domain.RelationKey(rel.Key)) {
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
