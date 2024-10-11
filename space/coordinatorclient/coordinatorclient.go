package coordinatorclient

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"

	"github.com/anyproto/anytype-heart/space/deletioncontroller"
)

const CName = coordinatorclient.CName

type coordinatorClient struct {
	coordinatorclient.CoordinatorClient
	delController deletioncontroller.DeletionController
}

func New() coordinatorclient.CoordinatorClient {
	return &coordinatorClient{}
}

func (c *coordinatorClient) Init(a *app.App) (err error) {
	c.delController = a.MustComponent(deletioncontroller.CName).(deletioncontroller.DeletionController)
	c.CoordinatorClient = coordinatorclient.New()
	return c.CoordinatorClient.Init(a)
}

func (c *coordinatorClient) Name() string {
	return CName
}

func (c *coordinatorClient) SpaceSign(ctx context.Context, payload coordinatorclient.SpaceSignPayload) (receipt *coordinatorproto.SpaceReceiptWithSignature, err error) {
	res, err := c.CoordinatorClient.SpaceSign(ctx, payload)
	if err != nil {
		return nil, err
	}
	c.delController.UpdateCoordinatorStatus()
	return res, nil
}
