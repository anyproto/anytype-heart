package push

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-push-server/pushclient/pushapi"
)

type Service interface {
	app.ComponentRunnable
	SendPush(ctx context.Context) (err error)
}

type service struct {
	pushClient pushapi.DRPCPushClient
}
