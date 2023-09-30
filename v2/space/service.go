package space

import (
	"context"
)

type DeletionParams struct{}

type SpaceService interface {
	Get(ctx context.Context, id string) (Space, error)
	Create(ctx context.Context) (Space, error)
	Delete(ctx context.Context, params DeletionParams) error
	RevertDeletion(ctx context.Context) error
}

type spaceService struct {
}

func (s *spaceService) UpdateSpace(spaceId string, status SpaceStatus) {
}
