package invitemigrator

import (
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
)

const CName = "client.components.invitemigrator"

var log = logger.NewNamed(CName)

func New() InviteMigrator {
	return &inviteMigrator{}
}

type InviteMigrator interface {
	app.Component
	MigrateExistingInvites(space clientspace.Space) error
}

type inviteMigrator struct {
	status spacestatus.SpaceStatus
}

func (i *inviteMigrator) Init(a *app.App) (err error) {
	i.status = app.MustComponent[spacestatus.SpaceStatus](a)
	return nil
}

func (i *inviteMigrator) Name() (name string) {
	return CName
}

func (i *inviteMigrator) MigrateExistingInvites(space clientspace.Space) error {
	spaceView := i.status.GetSpaceView()
	spaceView.Lock()
	fileCid, fileKey := spaceView.GetExistingInviteInfo()
	if fileCid == "" {
		spaceView.Unlock()
		return nil
	}
	_, err := spaceView.RemoveExistingInviteInfo()
	if err != nil {
		log.Warn("remove existing invite info", zap.Error(err))
	}
	spaceView.Unlock()
	return space.Do(space.DerivedIDs().Workspace, func(sb smartblock.SmartBlock) error {
		invObject, ok := sb.(domain.InviteObject)
		if !ok {
			return fmt.Errorf("space is not invite object")
		}
		return invObject.SetInviteFileInfo(fileCid, fileKey)
	})
}
