package list

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

var (
	ErrFailedGetObjectsInList      = errors.New("failed to get objects in list")
	ErrFailedAddObjectsToList      = errors.New("failed to add objects to list")
	ErrFailedRemoveObjectsFromList = errors.New("failed to remove objects from list")
)

type Service interface {
	GetObjectsInList(ctx context.Context, spaceId string, listId string, offset, limit int) ([]object.Object, int, bool, error)
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
func (s *ListService) GetObjectsInList(ctx context.Context, spaceId string, listId string, offset, limit int) ([]object.Object, int, bool, error) {
	resp := s.mw.ObjectSearchSubscribe(ctx, &pb.RpcObjectSearchSubscribeRequest{
		SpaceId:      spaceId,
		Limit:        int64(limit),  // nolint: gosec
		Offset:       int64(offset), // nolint: gosec
		Keys:         []string{bundle.RelationKeyId.String()},
		CollectionId: listId,
	})

	// TODO: returned error from ObjectSearchSubscribe is inconsistent with other RPCs: Error is nil instead of Code being NULL
	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchSubscribeResponseError_NULL {
		return nil, 0, false, ErrFailedGetObjectsInList
	}

	total := int(resp.Counters.Total)
	paginatedRecords, hasMore := pagination.Paginate(resp.Records, offset, limit)

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
		ContextId: spaceId,
		ObjectIds: objectIds,
	})

	if resp.Error.Code != pb.RpcObjectCollectionRemoveResponseError_NULL {
		return ErrFailedRemoveObjectsFromList
	}

	return nil
}
