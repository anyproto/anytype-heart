package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAccount(t *testing.T) {
	_, mw, close := start(t, nil)
	defer close()

	t.Run("account_should_open", func(t *testing.T) {
		accId := mw.GetAnytype().PredefinedBlocks().Account
		mw.PageCreate(&pb.RpcPageCreateRequest{})
		resp := mw.BlockOpen(&pb.RpcBlockOpenRequest{BlockId: accId})
		require.Equal(t, 0, int(resp.Error.Code), resp.Error.Description)
		show := getEventObjectShow(resp.Event.Messages)
	})

}
