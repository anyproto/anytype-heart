package space

import "context"

type SpaceService interface {
	Get(ctx context.Context, id string) (SpaceController, error)
	Create(ctx context.Context) (SpaceController, error)
}
