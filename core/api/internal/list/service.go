package list

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrFailedGetObjectsInList      = errors.New("failed to get objects in list")
	ErrFailedAddObjectsToList      = errors.New("failed to add objects to list")
	ErrFailedRemoveObjectsFromList = errors.New("failed to remove objects from list")
	ErrFailedUpdateObjectsInList   = errors.New("failed to update objects in list")
)

type Service interface {
	GetObjectsInList(ctx context.Context, spaceId string, listId string, offset, limit int) ([]*model.Object, int, bool, error)
	AddObjectsToList(ctx context.Context, spaceId string, listId string, objectIDs []string) error
	RemoveObjectsFromList(ctx context.Context, spaceId string, listId string, objectIDs []string) error
	UpdateObjectsInList(ctx context.Context, spaceId string, listId string, objectIDs []string) error
}

type ListService struct {
	mw service.ClientCommandsServer
}

func NewService(mw service.ClientCommandsServer) *ListService {
	return &ListService{mw: mw}
}

// GetObjectsInList retrieves objects in a list
func (s *ListService) GetObjectsInList(ctx context.Context, spaceId string, listId string, offset, limit int) ([]*model.Object, int, bool, error) {
	return nil, 0, false, nil
}

// AddObjectsToList adds objects to a list
func (s *ListService) AddObjectsToList(ctx context.Context, spaceId string, listId string, objectIDs []string) error {
	return nil
}

// RemoveObjectsFromList removes objects from a list
func (s *ListService) RemoveObjectsFromList(ctx context.Context, spaceId string, listId string, objectIDs []string) error {
	return nil
}

// UpdateObjectsInList updates an object in a list
func (s *ListService) UpdateObjectsInList(ctx context.Context, spaceId string, listId string, objectIDs []string) error {
	return nil
}
