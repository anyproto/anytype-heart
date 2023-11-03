package configfetcher

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/periodicsync"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/wallet"
	pbMiddle "github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/cafe/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

var log = logging.Logger("anytype-mw-configfetcher")

const (
	refreshIntervalSecs = 300
	timeout             = 10 * time.Second
	initialStatus       = -1
)

const CName = "configfetcher"

type WorkspaceGetter interface {
	GetAllWorkspaces() ([]string, error)
}

var defaultAccountState = &pb.AccountState{
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

type personalSpaceIDGetter interface {
	PersonalSpaceID() string
}

type configFetcher struct {
	store         objectstore.ObjectStore
	eventSender   event.Sender
	fetched       chan struct{}
	fetchedClosed sync.Once

	periodicSync periodicsync.PeriodicSync
	client       coordinatorclient.CoordinatorClient
	spaceService spacecore.SpaceCoreService
	account      personalSpaceIDGetter
	wallet       wallet.Wallet
	lastStatus   model.AccountStatusType
	mutex        sync.Mutex
}

func (c *configFetcher) GetAccountState() (state *pb.AccountState) {
	select {
	case <-c.fetched:
	case <-time.After(time.Second):
	}
	state = defaultAccountState
	status, err := c.store.GetAccountStatus()
	if err != nil {
		log.Debug("failed to account state config from the store: %s", err)
	} else {
		state.Status.Status = pb.AccountStateStatusType(status.Status)
		state.Status.DeletionDate = status.DeletionTimestamp
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
	c.wallet = a.MustComponent(wallet.CName).(wallet.Wallet)
	c.eventSender = a.MustComponent(event.CName).(event.Sender)
	c.periodicSync = periodicsync.NewPeriodicSync(refreshIntervalSecs, timeout, c.updateStatus, logger.CtxLogger{Logger: log.Desugar()})
	c.client = a.MustComponent(coordinatorclient.CName).(coordinatorclient.CoordinatorClient)
	c.spaceService = a.MustComponent(spacecore.CName).(spacecore.SpaceCoreService)
	c.account = app.MustComponent[personalSpaceIDGetter](a)
	c.fetched = make(chan struct{})
	c.lastStatus = initialStatus
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
	personalSpaceID := c.account.PersonalSpaceID()
	res, err := c.client.StatusCheck(ctx, personalSpaceID)
	if err == coordinatorproto.ErrSpaceNotExists {
		sp, cErr := c.spaceService.Get(ctx, personalSpaceID)
		if cErr != nil {
			return cErr
		}
		header, sErr := sp.Storage().SpaceHeader()
		if sErr != nil {
			return sErr
		}
		payload := coordinatorclient.SpaceSignPayload{
			SpaceId:     header.Id,
			SpaceHeader: header.RawHeader,
			OldAccount:  c.wallet.GetOldAccountKey(),
			Identity:    c.wallet.GetAccountPrivkey(),
		}
		// registering space inside coordinator
		_, err = c.client.SpaceSign(ctx, payload)
		if err != nil {
			return err
		}
		return c.updateStatus(ctx)
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := c.updateStatus(ctx)
	if err != nil {
		log.Errorf("failed to update status: %s", err)
	}
}

func (c *configFetcher) Close(ctx context.Context) (err error) {
	c.periodicSync.Close()
	return
}

func (c *configFetcher) notifyClientApp(status *coordinatorproto.SpaceStatusPayload) {
	s := convertToAccountStatusModel(status)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.lastStatus == s.StatusType {
		return
	}

	c.lastStatus = s.StatusType
	ev := &pbMiddle.Event{
		Messages: []*pbMiddle.EventMessage{
			{
				Value: &pbMiddle.EventMessageValueOfAccountUpdate{
					AccountUpdate: &pbMiddle.EventAccountUpdate{
						Status: s,
					},
				},
			},
		},
	}
	if c.eventSender != nil {
		c.eventSender.Broadcast(ev)
	}
}

func convertToAccountStatusModel(status *coordinatorproto.SpaceStatusPayload) *model.AccountStatus {
	return &model.AccountStatus{
		StatusType:   model.AccountStatusType(status.Status),
		DeletionDate: status.DeletionTimestamp,
	}
}
