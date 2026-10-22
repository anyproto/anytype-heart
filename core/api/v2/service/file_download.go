package v2service

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api/filecontent"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// GetFileContent authorizes the file before opening any content reader.
func (s *Service) GetFileContent(ctx context.Context, spaceId, fileId string, width int) (*filecontent.Content, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, err
	}
	if domain.IsFileId(fileId) {
		allowed, err := s.isSpaceIcon(spaceId, fileId)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, v2model.NotFound("file or icon not found in this space")
		}
	} else {
		// Use the same object-to-space binding the file service resolves.
		// A caller-supplied space in the URL alone proves nothing about the file.
		actualSpaceId, err := s.store.GetSpaceId(fileId)
		if err != nil || actualSpaceId != spaceId {
			return nil, v2model.NotFound("file or icon not found in this space")
		}
	}
	content, err := filecontent.Get(ctx, s.fileService, fileId, width)
	if errors.Is(err, filecontent.ErrNotFound) {
		return nil, v2model.NotFound("file content not found")
	}
	return content, err
}

// isSpaceIcon is a compatibility exception for participant and space-view
// icons, which still store raw CIDs instead of file object IDs. A reference
// on any other object, or in another space, must not authorize a download.
// Recheck the current index on every request, including conditional reads.
func (s *Service) isSpaceIcon(spaceId, fileId string) (bool, error) {
	if spaceId != s.techSpaceId {
		details, err := s.store.GetSpaceViewDetails(spaceId)
		if err != nil {
			return false, fmt.Errorf("get space icon: %w", err)
		}
		if details.GetString(bundle.RelationKeyIconImage) == fileId {
			return true, nil
		}
	}
	ids, _, err := s.store.SpaceIndex(spaceId).QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_participant)),
			},
			{
				RelationKey: bundle.RelationKeyIconImage,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(fileId),
			},
		},
		Limit: 1,
	})
	if err != nil {
		return false, fmt.Errorf("query participant icons: %w", err)
	}
	return len(ids) > 0, nil
}
