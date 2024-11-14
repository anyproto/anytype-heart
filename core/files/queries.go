package files

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) getFileVariantBySourceChecksum(mill string, sourceChecksum string, options string) (domain.FileId, []*storage.FileInfo, error) {
	recs, err := s.objectStore.QueryCrossSpace(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileVariantMills.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(mill),
			},
			{
				RelationKey: bundle.RelationKeyFileSourceChecksum.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(sourceChecksum),
			},
			{
				RelationKey: bundle.RelationKeyFileVariantOptions.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(options),
			},
		},
		Limit: 1,
	})
	if err != nil {
		return "", nil, err
	}
	if len(recs) == 0 {
		return "", nil, fmt.Errorf("variant not found")
	}

	infos := filemodels.GetFileInfosFromDetails(recs[0].Details)
	return domain.FileId(pbtypes.GetString(recs[0].Details, bundle.RelationKeyFileId.String())), infos, nil
}

func (s *service) getFileVariantByChecksum(mill string, variantChecksum string) (domain.FileId, *storage.FileInfo, []*storage.FileInfo, error) {
	recs, err := s.objectStore.QueryCrossSpace(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileVariantMills.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(mill),
			},
			{
				RelationKey: bundle.RelationKeyFileVariantChecksums.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(variantChecksum),
			},
		},
		Limit: 1,
	})
	if err != nil {
		return "", nil, nil, err
	}
	if len(recs) == 0 {
		return "", nil, nil, fmt.Errorf("variant not found")
	}

	infos := filemodels.GetFileInfosFromDetails(recs[0].Details)
	for _, info := range infos {
		if info.Mill == mill && info.Checksum == variantChecksum {
			return domain.FileId(pbtypes.GetString(recs[0].Details, bundle.RelationKeyFileId.String())), info, infos, nil
		}
	}
	// Should never happen
	return "", nil, nil, fmt.Errorf("variant with specified mill not found")
}
