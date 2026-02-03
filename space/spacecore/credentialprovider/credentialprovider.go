package credentialprovider

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/credentialprovider"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core/wallet"
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
	// For accounts created after identity migration, OldAccount doesn't exist.
	// Fall back to current identity to avoid nil pointer in SpaceSign.
	oldAccount := c.wallet.GetOldAccountKey()
	if oldAccount == nil {
		oldAccount = c.wallet.GetAccountPrivkey()
	}
	payload := coordinatorclient.SpaceSignPayload{
		SpaceId:     spaceHeader.Id,
		SpaceHeader: spaceHeader.RawHeader,
		OldAccount:  oldAccount,
		Identity:    c.wallet.GetAccountPrivkey(),
	}
	receipt, err := c.client.SpaceSign(ctx, payload)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(receipt)
}
