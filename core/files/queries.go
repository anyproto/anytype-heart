package files

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

type existingFile struct {
	fileId   domain.FileId
	variants []*storage.FileInfo
}

func collectKeysFromVariants(variants []*storage.FileInfo) map[string]string {
	keys := map[string]string{}
	for _, variant := range variants {
		keys[variant.Path] = variant.Key
	}
	return keys
}

func (s *service) getFileVariantBySourceChecksum(mill string, sourceChecksum string, options string) (*existingFile, error) {
	recs, err := s.objectStore.QueryCrossSpace(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyFileVariantMills,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(mill),
			},
			{
				RelationKey: bundle.RelationKeyFileSourceChecksum,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(sourceChecksum),
			},
			{
				RelationKey: bundle.RelationKeyFileVariantOptions,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(options),
			},
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("variant not found")
	}

	variants, err := filemodels.GetFileInfosFromDetails(recs[0].Details)
	if err != nil {
		return nil, fmt.Errorf("get file info from details: %w", err)
	}
	return &existingFile{
		fileId:   domain.FileId(recs[0].Details.GetString(bundle.RelationKeyFileId)),
		variants: variants,
	}, nil
}

func (s *service) getFileVariantByChecksum(mill string, variantChecksum string) (*existingFile, *storage.FileInfo, error) {
	recs, err := s.objectStore.QueryCrossSpace(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyFileVariantMills,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(mill),
			},
			{
				RelationKey: bundle.RelationKeyFileVariantChecksums,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(variantChecksum),
			},
		},
		Limit: 1,
	})
	if err != nil {
		return nil, nil, err
	}
	if len(recs) == 0 {
		return nil, nil, fmt.Errorf("variant not found")
	}

	variants, err := filemodels.GetFileInfosFromDetails(recs[0].Details)
	if err != nil {
		return nil, nil, fmt.Errorf("get file info from details: %w", err)
	}
	for _, info := range variants {
		if info.Mill == mill && info.Checksum == variantChecksum {
			return &existingFile{
				fileId:   domain.FileId(recs[0].Details.GetString(bundle.RelationKeyFileId)),
				variants: variants,
			}, info, nil
		}
	}
	// Should never happen
	return nil, nil, fmt.Errorf("variant with specified mill not found")
}
