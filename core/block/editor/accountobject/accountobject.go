package accountobject

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
)

var log = logger.NewNamed("common.editor.accountobject")

const (
	collectionName  = "accountData"
	ProfileDocument = "profile"
)

type ProfileDetails struct {
	Name        string
	Description string
	IconImage   string
}

type ProfileSubscription = func(ctx context.Context, profile ProfileDetails) error

type AccountObject interface {
	smartblock.SmartBlock

	SubscribeProfile(subscription ProfileSubscription)
	UnsubscribeProfile()

	ProfileDetails() (ProfileDetails, error)
	UpdateProfileData(ctx context.Context, key string, data any) error
}

type StoreDbProvider interface {
	GetStoreDb() anystore.DB
}

var _ AccountObject = (*accountObject)(nil)

type accountObject struct {
	smartblock.SmartBlock
	profileSubscription ProfileSubscription
	dbProvider          StoreDbProvider
	state               *storestate.StoreState
	storeSource         source.Store
	ctx                 context.Context
	cancel              context.CancelFunc
	mx                  sync.Mutex
}

func New(sb smartblock.SmartBlock, dbProvider StoreDbProvider) AccountObject {
	return &accountObject{
		SmartBlock: sb,
		dbProvider: dbProvider,
	}
}

func (a *accountObject) SubscribeProfile(subscription ProfileSubscription) {
	a.mx.Lock()
	defer a.mx.Unlock()
	a.profileSubscription = subscription
}

func (a *accountObject) UnsubscribeProfile() {
	a.mx.Lock()
	defer a.mx.Unlock()
	a.profileSubscription = nil
}

func (a *accountObject) Init(ctx *smartblock.InitContext) error {
	err := a.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	a.ctx, a.cancel = context.WithCancel(ctx.Ctx)
	stateStore, err := storestate.New(a.ctx, a.Id(), a.dbProvider.GetStoreDb(), accountHandler{})
	if err != nil {
		return fmt.Errorf("create state store: %w", err)
	}
	a.state = stateStore
	storeSource, ok := ctx.Source.(source.Store)
	if !ok {
		return fmt.Errorf("source is not a store")
	}
	a.storeSource = storeSource
	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, a.onUpdate)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}

	return nil
}

func (a *accountObject) UpdateProfileData(ctx context.Context, key string, data any) error {
	builder := storestate.Builder{}
	err := builder.Modify(collectionName, ProfileDocument, []string{key}, pb.ModifyOp_Set, data)
	if err != nil {
		return fmt.Errorf("modify content: %w", err)
	}
	_, err = a.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   a.state,
		Time:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}

func (a *accountObject) ProfileDetails() (ProfileDetails, error) {
	return a.getProfile(a.ctx)
}

func (a *accountObject) onUpdate() {
	details, err := a.getProfile(a.ctx)
	if err != nil {
		log.Warn("get profile details", zap.Error(err))
		return
	}
	a.mx.Lock()
	sub := a.profileSubscription
	a.mx.Unlock()
	if sub != nil {
		err = sub(a.ctx, details)
		if err != nil {
			log.Warn("profile subscription", zap.Error(err))
		}
	}
}

func (a *accountObject) getProfile(ctx context.Context) (details ProfileDetails, err error) {
	coll, err := a.state.Collection(ctx, collectionName)
	if err != nil {
		return ProfileDetails{}, fmt.Errorf("get collection: %w", err)
	}
	txn, err := coll.ReadTx(ctx)
	if err != nil {
		return ProfileDetails{}, fmt.Errorf("start read tx: %w", err)
	}
	obj, err := coll.FindId(txn.Context(), ProfileDocument)
	if err != nil {
		return ProfileDetails{}, errors.Join(txn.Commit(), fmt.Errorf("find id: %w", err))
	}
	wrapper := newProfileWrapper(obj.Value())
	return wrapper.Profile(), txn.Commit()
}
