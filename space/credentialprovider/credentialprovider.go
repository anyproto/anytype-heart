package credentialprovider

import (
	"context"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/credentialprovider"
	"github.com/anytypeio/any-sync/commonspace/spacesyncproto"
	"github.com/anytypeio/any-sync/coordinator/coordinatorclient"
	"github.com/gogo/protobuf/proto"

	"github.com/anytypeio/go-anytype-middleware/core/wallet"
)

func New() app.Component {
	return &credentialProvider{}
}

type credentialProvider struct {
	client coordinatorclient.CoordinatorClient
	wallet wallet.Wallet
}

func (c *credentialProvider) Init(a *app.App) (err error) {
	c.client = a.MustComponent(coordinatorclient.CName).(coordinatorclient.CoordinatorClient)
	c.wallet = a.MustComponent(wallet.CName).(wallet.Wallet)
	return
}

func (c *credentialProvider) Name() (name string) {
	return credentialprovider.CName
}

func (c *credentialProvider) GetCredential(ctx context.Context, spaceHeader *spacesyncproto.RawSpaceHeaderWithId) ([]byte, error) {
	payload := coordinatorclient.SpaceSignPayload{
		SpaceId:     spaceHeader.Id,
		SpaceHeader: spaceHeader.RawHeader,
		OldAccount:  c.wallet.GetOldAccountKey(),
		Identity:    c.wallet.GetAccountPrivkey(),
	}
	receipt, err := c.client.SpaceSign(ctx, payload)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(receipt)
}
