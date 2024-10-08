package joiner

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/acl/aclwaiter"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/internal/components/aclnotifications"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type joiner struct {
	app *app.App
}

type Joiner interface {
	mode.Process
}

type Params struct {
	SpaceId string
	Status  spacestatus.SpaceStatus
	Log     logger.CtxLogger
}

func New(app *app.App, params Params) Joiner {
	child := app.ChildApp()
	joinHeadId := params.Status.GetLatestAclHeadId()
	child.Register(newStatusChanger()).
		Register(aclnotifications.NewAclNotificationSender()).
		Register(aclwaiter.New(params.SpaceId,
			joinHeadId,
			// onFinish
			func(acl list.AclList) error {
				info := spaceinfo.NewSpacePersistentInfo(params.SpaceId)
				info.SetAccountStatus(spaceinfo.AccountStatusActive).
					SetAclHeadId(acl.Head().Id)
				err := params.Status.SetPersistentInfo(info)
				if err != nil {
					params.Log.Error("failed to set persistent status", zap.Error(err))
				}
				return err
			},
			// onReject
			func(acl list.AclList) error {
				info := spaceinfo.NewSpacePersistentInfo(params.SpaceId)
				info.SetAccountStatus(spaceinfo.AccountStatusDeleted).
					SetAclHeadId(acl.Head().Id)
				err := params.Status.SetPersistentInfo(info)
				if err != nil {
					params.Log.Error("failed to set persistent status", zap.Error(err))
				}
				aclNotificationSender := child.MustComponent(aclnotifications.CName).(aclnotifications.AclNotification)
				aclNotificationSender.AddSingleRecord(acl.Id(), acl.Head(), 0, params.SpaceId, spaceinfo.AccountStatusDeleted)
				return err
			}))
	return &joiner{
		app: child,
	}
}

func (i *joiner) Start(ctx context.Context) error {
	return i.app.Start(ctx)
}

func (i *joiner) Close(ctx context.Context) error {
	return i.app.Close(ctx)
}

func (i *joiner) CanTransition(next mode.Mode) bool {
	return true
}
