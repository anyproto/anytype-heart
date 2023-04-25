package configfetcher

import (
	"context"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/coordinator/coordinatorclient"
	"github.com/anytypeio/any-sync/coordinator/coordinatorproto"
	"github.com/anytypeio/any-sync/util/periodicsync"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
	"github.com/anytypeio/go-anytype-middleware/space"
	"sync"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	pbMiddle "github.com/anytypeio/go-anytype-middleware/pb"
	cafeClient "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw-configfetcher")

const CName = "configfetcher"

type WorkspaceGetter interface {
	GetAllWorkspaces() ([]string, error)
}

var defaultAccountState = &pb.AccountState{
	Config: &pb.Config{
		EnableDataview:             false,
		EnableDebug:                false,
		EnableReleaseChannelSwitch: false,
		EnablePrereleaseChannel:    false,
		SimultaneousRequests:       20,
		EnableSpaces:               false,
		Extra:                      nil,
	},
	Status: &pb.AccountStateStatus{
		Status:       pb.AccountState_Active,
		DeletionDate: 0,
	},
}

type ConfigFetcher interface {
	app.ComponentRunnable
	GetAccountState() *pb.AccountState
	Refetch()
}

type configFetcher struct {
	store         objectstore.ObjectStore
	cafe          cafeClient.Client
	eventSender   func(event *pbMiddle.Event)
	fetched       chan struct{}
	fetchedClosed sync.Once

	observers    []util.CafeAccountStateUpdateObserver
	periodicSync periodicsync.PeriodicSync
	client       coordinatorclient.CoordinatorClient
	accountId    string
}

func (c *configFetcher) GetAccountState() (state *pb.AccountState) {
	select {
	case <-c.fetched:
	case <-time.After(time.Second):
	}
	state = defaultAccountState
	status, err := c.store.GetAccountStatus()
	if err != nil {
		log.Debug("failed to account state config from the store: %s", err.Error())
	} else {
		state.Status.Status = pb.AccountStateStatusType(status.Status)
		state.Status.DeletionDate = status.DeletionTimestamp / int64(time.Second)
	}
	return state
}

func New() ConfigFetcher {
	return &configFetcher{}
}

func (c *configFetcher) Run(context.Context) error {
	c.periodicSync.Run()
	return nil
}

func (c *configFetcher) Init(a *app.App) (err error) {
	c.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	c.eventSender = a.MustComponent(event.CName).(event.Sender).Send
	c.periodicSync = periodicsync.NewPeriodicSync(300, time.Second*10, c.updateStatus, logger.CtxLogger{Logger: log.Desugar()})
	c.client = a.MustComponent(coordinatorclient.CName).(coordinatorclient.CoordinatorClient)
	c.accountId = a.MustComponent(space.CName).(space.Service).AccountId()
	c.fetched = make(chan struct{})
	return nil
}

func (c *configFetcher) Name() (name string) {
	return CName
}

func (c *configFetcher) updateStatus(ctx context.Context) (err error) {
	defer func() {
		c.fetchedClosed.Do(func() {
			close(c.fetched)
		})
	}()
	res, err := c.client.StatusCheck(ctx, c.accountId)
	if err != nil {
		return
	}
	err = c.store.SaveAccountStatus(res)
	if err != nil {
		return
	}
	c.notifyClientApp(res)
	return
}

func (c *configFetcher) Refetch() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c.updateStatus(ctx)
}

func (c *configFetcher) Close(ctx context.Context) (err error) {
	c.periodicSync.Close()
	return
}

func (c *configFetcher) notifyClientApp(status *coordinatorproto.SpaceStatusPayload) {
	ev := &pbMiddle.Event{
		Messages: []*pbMiddle.EventMessage{
			{
				Value: &pbMiddle.EventMessageValueOfAccountUpdate{
					AccountUpdate: &pbMiddle.EventAccountUpdate{
						Config: convertToAccountConfigModel(defaultAccountState.Config),
						Status: convertToAccountStatusModel(status),
					},
				},
			},
		},
	}
	if c.eventSender != nil {
		c.eventSender(ev)
	}
}

func convertToAccountConfigModel(cfg *pb.Config) *model.AccountConfig {
	return &model.AccountConfig{
		EnableDataview:          cfg.EnableDataview,
		EnableDebug:             cfg.EnableDebug,
		EnablePrereleaseChannel: cfg.EnablePrereleaseChannel,
		EnableSpaces:            cfg.EnableSpaces,
		Extra:                   cfg.Extra,
	}
}

func convertToAccountStatusModel(status *coordinatorproto.SpaceStatusPayload) *model.AccountStatus {
	return &model.AccountStatus{
		StatusType:   model.AccountStatusType(status.Status),
		DeletionDate: status.DeletionTimestamp / int64(time.Second),
	}
}
