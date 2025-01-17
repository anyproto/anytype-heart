package relationutils

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ObjectIDDeriver interface {
	DeriveObjectID(ctx context.Context, uniqueKey domain.UniqueKey) (id string, err error)
}

var (
	defaultRecommendedFeaturedRelationKeys = []domain.RelationKey{
		bundle.RelationKeyType,
		bundle.RelationKeyTag,
		bundle.RelationKeyBacklinks,
	}

	defaultRecommendedRelationKeys = []domain.RelationKey{
		bundle.RelationKeyCreatedDate,
		bundle.RelationKeyCreator,
		bundle.RelationKeyLastModifiedDate,
		bundle.RelationKeyLastModifiedBy,
		bundle.RelationKeyLastOpenedDate,
		bundle.RelationKeyLinks,
	}

	fileSpecificRelationKeysMap = map[domain.RelationKey]struct{}{
		bundle.RelationKeyFileExt:               {},
		bundle.RelationKeySizeInBytes:           {},
		bundle.RelationKeyFileMimeType:          {},
		bundle.RelationKeyArtist:                {},
		bundle.RelationKeyAudioAlbum:            {},
		bundle.RelationKeyAudioGenre:            {},
		bundle.RelationKeyAudioAlbumTrackNumber: {},
		bundle.RelationKeyAudioLyrics:           {},
		bundle.RelationKeyReleasedYear:          {},
		bundle.RelationKeyHeightInPixels:        {},
		bundle.RelationKeyWidthInPixels:         {},
		bundle.RelationKeyCamera:                {},
		bundle.RelationKeyCameraIso:             {},
		bundle.RelationKeyAperture:              {},
		bundle.RelationKeyExposure:              {},
		bundle.RelationKeyFocalRatio:            {},
	}

	errRecommendedRelationsAlreadyFilled = fmt.Errorf("recommended featured relations are already filled")
)

// FillRecommendedRelations fills recommendedRelations and recommendedFeaturedRelations based on object's details
// If these relations are already filled with correct ids, isAlreadyFilled = true is returned
func FillRecommendedRelations(ctx context.Context, deriver ObjectIDDeriver, details *domain.Details) (keys []domain.RelationKey, isAlreadyFilled bool, err error) {
	keys, err = getRelationKeysFromDetails(details)
	if err != nil {
		if errors.Is(err, errRecommendedRelationsAlreadyFilled) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("get recommended relation keys: %w", err)
	}

	if isFileType(details) {
		// for file types we need to fill separate relation list with file-specific recommended relations
		var fileRecommendedRelationKeys, other []domain.RelationKey
		for _, key := range keys {
			if _, found := fileSpecificRelationKeysMap[key]; found {
				fileRecommendedRelationKeys = append(fileRecommendedRelationKeys, key)
				continue
			}
			other = append(other, key)
		}
		fileRelationIds, err := prepareRelationIds(ctx, deriver, fileRecommendedRelationKeys)
		if err != nil {
			return nil, false, fmt.Errorf("prepare file recommended relation ids: %w", err)
		}
		details.SetStringList(bundle.RelationKeyRecommendedFileRelations, fileRelationIds)
		keys = other
	}

	// we should include default system recommended relations and exclude default recommended featured relations
	keys = lo.Uniq(append(keys, defaultRecommendedRelationKeys...))
	keys = slices.DeleteFunc(keys, func(key domain.RelationKey) bool {
		return slices.Contains(defaultRecommendedFeaturedRelationKeys, key)
	})

	relationIds, err := prepareRelationIds(ctx, deriver, keys)
	if err != nil {
		return nil, false, fmt.Errorf("prepare recommended relation ids: %w", err)
	}
	details.SetStringList(bundle.RelationKeyRecommendedRelations, relationIds)

	featuredRelationIds, err := prepareRelationIds(ctx, deriver, defaultRecommendedFeaturedRelationKeys)
	if err != nil {
		return nil, false, fmt.Errorf("prepare recommended featured relation ids: %w", err)
	}
	details.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, featuredRelationIds)

	return append(keys, defaultRecommendedFeaturedRelationKeys...), false, nil
}

func getRelationKeysFromDetails(details *domain.Details) ([]domain.RelationKey, error) {
	bundledRelationIds := details.GetStringList(bundle.RelationKeyRecommendedRelations)
	if len(bundledRelationIds) == 0 {
		rawRecommendedLayout := details.GetInt64(bundle.RelationKeyRecommendedLayout)
		// nolint: gosec
		recommendedLayout, err := bundle.GetLayout(model.ObjectTypeLayout(rawRecommendedLayout))
		if err != nil {
			return nil, fmt.Errorf("invalid recommended layout %d: %w", rawRecommendedLayout, err)
		}
		keys := make([]domain.RelationKey, 0, len(recommendedLayout.RequiredRelations))
		for _, rel := range recommendedLayout.RequiredRelations {
			keys = append(keys, domain.RelationKey(rel.Key))
		}
		return keys, nil
	}

	keys := make([]domain.RelationKey, 0, len(bundledRelationIds))
	for i, id := range bundledRelationIds {
		key, err := bundle.RelationKeyFromID(id)
		if err == nil {
			if key != bundle.RelationKeyDescription {
				keys = append(keys, key)
			}
			continue
		}
		if i == 0 {
			// if we fail to parse 1st bundled relation id, details are already filled with correct ids
			return nil, errRecommendedRelationsAlreadyFilled
		}
		return nil, fmt.Errorf("relation key from id: %w", err)
	}
	return keys, nil
}

func prepareRelationIds(ctx context.Context, deriver ObjectIDDeriver, relationKeys []domain.RelationKey) ([]string, error) {
	relationIds := make([]string, 0, len(relationKeys))
	for _, key := range relationKeys {
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, key.String())
		if err != nil {
			return nil, fmt.Errorf("failed to create unique Key: %w", err)
		}
		id, err := deriver.DeriveObjectID(ctx, uk)
		if err != nil {
			return nil, fmt.Errorf("failed to derive object id: %w", err)
		}
		relationIds = append(relationIds, id)
	}
	return relationIds, nil
}

func isFileType(details *domain.Details) bool {
	uniqueKey, err := domain.UnmarshalUniqueKey(details.GetString(bundle.RelationKeyUniqueKey))
	if err != nil {
		return false
	}
	return slices.Contains([]domain.TypeKey{
		bundle.TypeKeyFile, bundle.TypeKeyImage, bundle.TypeKeyVideo, bundle.TypeKeyAudio,
	}, domain.TypeKey(uniqueKey.InternalKey()))
}
