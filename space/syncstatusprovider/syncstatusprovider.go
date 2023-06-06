package syncstatusprovider

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
)

func New() syncstatus.StatusServiceProvider {
	return statusProvider{}
}

type statusProvider struct {
}

func (s statusProvider) Init(a *app.App) (err error) {
	return nil
}

func (s statusProvider) Name() (name string) {
	return syncstatus.CName
}

func (s statusProvider) NewStatusService() syncstatus.StatusService {
	return syncstatus.NewSyncStatusProvider()
}
