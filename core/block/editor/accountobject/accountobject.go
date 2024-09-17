package accountobject

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logger.NewNamed("common.editor.accountobject")

const (
	collectionName  = "account"
	accountDocument = "accountObject"
)

type ProfileDetails struct {
	Name        string
	Description string
	IconImage   string
}

type ProfileSubscription = func(ctx context.Context, profile ProfileDetails) error

type AccountObject interface {
	smartblock.SmartBlock
	SetSharedSpacesLimit(limit int) (err error)
	SetProfileDetails(details *types.Struct) (err error)
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
	relMapper           *relationsMapper
}

func New(sb smartblock.SmartBlock, dbProvider StoreDbProvider) AccountObject {
	return &accountObject{
		SmartBlock: sb,
		dbProvider: dbProvider,
		relMapper: newRelationsMapper(map[string]KeyType{
			bundle.RelationKeyName.String():        KeyTypeString,
			bundle.RelationKeyDescription.String(): KeyTypeString,
			bundle.RelationKeyIconImage.String():   KeyTypeString,
		}),
	}
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
	storeSource.SetPushChangeHook(a.OnPushChange)
	a.storeSource = storeSource
	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, a.onUpdate)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}
	coll, err := a.state.Collection(ctx.Ctx, collectionName)
	if err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	_, err = coll.FindId(ctx.Ctx, accountDocument)
	if err != nil {
		if errors.Is(err, anystore.ErrDocNotFound) {
			err = coll.Insert(ctx.Ctx, fmt.Sprintf(`{"id":"%s"}`, accountDocument))
			if err != nil {
				return fmt.Errorf("insert account document: %w", err)
			}
			return nil
		}
		return fmt.Errorf("find id: %w", err)
	}
	st := a.NewState()
	err = a.update(a.ctx, st)
	if err != nil {
		return fmt.Errorf("update state: %w", err)
	}
	// TODO: [PS] not sure that this works :-)
	return a.SmartBlock.Apply(st, smartblock.NotPushChanges, smartblock.NoEvent, smartblock.NoHistory, smartblock.SkipIfNoChanges)
}

func (a *accountObject) OnPushChange(params source.PushChangeParams) (id string, err error) {
	var (
		chs     = params.Changes
		builder = &storestate.Builder{}
	)
	for _, ch := range chs {
		set := ch.GetDetailsSet()
		if set != nil && set.Key != "" {
			val, ok := a.relMapper.GetStoreKey(set.Key, set.Value)
			if !ok {
				continue
			}
			err := builder.Modify(collectionName, accountDocument, []string{set.Key}, pb.ModifyOp_Set, val)
			if err != nil {
				return "", fmt.Errorf("modify content: %w", err)
			}
		}
	}
	if builder.StoreChange == nil {
		return "", nil
	}
	return a.storeSource.PushStoreChange(a.ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   a.state,
		Time:    time.Now(),
	})
}

func (a *accountObject) onUpdate() {
	st := a.NewState()
	err := a.update(a.ctx, st)
	if err != nil {
		log.Warn("get profile details", zap.Error(err))
		return
	}
	err = a.SmartBlock.(source.ChangeReceiver).StateRebuild(st)
	if err != nil {
		log.Warn("state rebuild", zap.Error(err))
		return
	}
}

func (a *accountObject) SetSharedSpacesLimit(limit int) (err error) {
	st := a.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeySharedSpacesLimit, pbtypes.Int64(int64(limit)))
	return a.Apply(st)
}

func (a *accountObject) GetSharedSpacesLimit() (limit int) {
	return int(pbtypes.GetInt64(a.CombinedDetails(), bundle.RelationKeySharedSpacesLimit.String()))
}

func (a *accountObject) SetProfileDetails(details *types.Struct) (err error) {
	st := a.NewState()
	// we should set everything in local state, but not everything in the store (this should be filtered in OnPushChange)
	for key, val := range details.Fields {
		st.SetDetailAndBundledRelation(domain.RelationKey(key), val)
	}
	return a.Apply(st)
}

func (a *accountObject) update(ctx context.Context, st *state.State) (err error) {
	coll, err := a.state.Collection(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	obj, err := coll.FindId(ctx, accountDocument)
	if err != nil {
		return fmt.Errorf("find id: %w", err)
	}
	for key := range a.relMapper.keys {
		pbVal, ok := a.relMapper.GetRelationKey(key, obj.Value())
		if !ok {
			continue
		}
		st.SetDetailAndBundledRelation(domain.RelationKey(key), pbVal)
	}
	return
}
