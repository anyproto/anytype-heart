package list

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrFailedGetList               = errors.New("failed to get list")
	ErrFailedGetListDataview       = errors.New("failed to get list dataview")
	ErrFailedGetListDataviewView   = errors.New("failed to get list dataview view")
	ErrFailedGetObjectsInList      = errors.New("failed to get objects in list")
	ErrFailedAddObjectsToList      = errors.New("failed to add objects to list")
	ErrFailedRemoveObjectsFromList = errors.New("failed to remove objects from list")
)

type Service interface {
	GetObjectsInList(ctx context.Context, spaceId string, listId string, viewId string, offset, limit int) ([]object.Object, int, bool, error)
	AddObjectsToList(ctx context.Context, spaceId string, listId string, objectIds []string) error
	RemoveObjectsFromList(ctx context.Context, spaceId string, listId string, objectIds []string) error
}

type ListService struct {
	mw            service.ClientCommandsServer
	objectService *object.ObjectService
}

func NewService(mw service.ClientCommandsServer, objectService *object.ObjectService) *ListService {
	return &ListService{mw: mw, objectService: objectService}
}

// GetObjectsInList retrieves objects in a list
func (s *ListService) GetObjectsInList(ctx context.Context, spaceId string, listId string, viewId string, offset, limit int) ([]object.Object, int, bool, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: listId,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return nil, 0, false, ErrFailedGetList
	}

	var dataviewBlock *model.Block
	for _, block := range resp.ObjectView.Blocks {
		if block.Id == "dataview" {
			dataviewBlock = block
			break
		}
	}

	if dataviewBlock == nil {
		return nil, 0, false, ErrFailedGetListDataview
	}

	var sorts []*model.BlockContentDataviewSort
	var filters []*model.BlockContentDataviewFilter

	switch content := dataviewBlock.Content.(type) {
	case *model.BlockContentOfDataview:
		var targetView *model.BlockContentDataviewView
		if viewId == "" && len(content.Dataview.Views) > 0 {
			// fallback to first view when view id is empty
			targetView = content.Dataview.Views[0]
		} else if viewId != "" {
			for _, view := range content.Dataview.Views {
				if view.Id == viewId {
					targetView = view
					break
				}
			}
		}

		if targetView == nil {
			return nil, 0, false, ErrFailedGetListDataviewView
		}

		// use the sorts and filters from the selected view
		sorts = targetView.Sorts
		filters = targetView.Filters
	default:
		return nil, 0, false, ErrFailedGetListDataview
	}

	searchResp := s.mw.ObjectSearchSubscribe(ctx, &pb.RpcObjectSearchSubscribeRequest{
		SpaceId:      spaceId,
		Limit:        int64(limit),  // nolint: gosec
		Offset:       int64(offset), // nolint: gosec
		Keys:         []string{bundle.RelationKeyId.String()},
		CollectionId: listId,
		Sorts:        sorts,
		Filters:      filters,
	})

	// TODO: returned error from ObjectSearchSubscribe is inconsistent with other RPCs: Error is nil instead of Code being NULL
	if searchResp.Error != nil && searchResp.Error.Code != pb.RpcObjectSearchSubscribeResponseError_NULL {
		return nil, 0, false, ErrFailedGetObjectsInList
	}

	total := int(searchResp.Counters.Total)
	paginatedRecords, hasMore := pagination.Paginate(searchResp.Records, offset, limit)

	objects := make([]object.Object, 0, len(paginatedRecords))
	for _, record := range paginatedRecords {
		object, err := s.objectService.GetObject(ctx, spaceId, record.Fields[bundle.RelationKeyId.String()].GetStringValue())
		if err != nil {
			return nil, 0, false, err
		}
		objects = append(objects, object)
	}

	return objects, total, hasMore, nil
}

// AddObjectsToList adds objects to a list
func (s *ListService) AddObjectsToList(ctx context.Context, spaceId string, listId string, objectIds []string) error {
	resp := s.mw.ObjectCollectionAdd(ctx, &pb.RpcObjectCollectionAddRequest{
		ContextId: listId,
		ObjectIds: objectIds,
	})

	if resp.Error.Code != pb.RpcObjectCollectionAddResponseError_NULL {
		return ErrFailedAddObjectsToList
	}

	return nil
}

// RemoveObjectsFromList removes objects from a list
func (s *ListService) RemoveObjectsFromList(ctx context.Context, spaceId string, listId string, objectIds []string) error {
	resp := s.mw.ObjectCollectionRemove(ctx, &pb.RpcObjectCollectionRemoveRequest{
		ContextId: listId,
		ObjectIds: objectIds,
	})

	if resp.Error.Code != pb.RpcObjectCollectionRemoveResponseError_NULL {
		return ErrFailedRemoveObjectsFromList
	}

	return nil
}
