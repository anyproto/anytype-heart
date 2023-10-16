package space

import (
	"context"
	"time"
)

type spaceDeleter interface {
	Delete(ctx context.Context, id string, deletionPeriod time.Duration) (err error)
}

type deletionController struct {
	deleter spaceDeleter
}

func newDeletionController(deleter spaceDeleter) *deletionController {
	return &deletionController{deleter}
}

func (d *deletionController) Run(ctx context.Context) (err error) {
	return nil
}

func (d *deletionController) NetworkDelete(ctx context.Context, id string) (err error) {
	return nil
}
