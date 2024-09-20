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

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/lastused"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logger.NewNamedSugared("common.editor.accountobject")

const (
	collectionName  = "account"
	accountDocument = "accountObject"
	analyticsKey    = "analyticsId"
)

type ProfileDetails struct {
	Name        string
	Description string
	IconImage   string
}

type ProfileSubscription = func(ctx context.Context, profile ProfileDetails) error

type AccountObject interface {
	smartblock.SmartBlock
	basic.DetailsSettable
	GetAnalyticsId() (string, error)
	SetAnalyticsId(id string) error
	SetSharedSpacesLimit(limit int) (err error)
	SetProfileDetails(details *types.Struct) (err error)
}

type StoreDbProvider interface {
	GetStoreDb() anystore.DB
}

var _ AccountObject = (*accountObject)(nil)

type accountObject struct {
	smartblock.SmartBlock
	bs                  basic.DetailsSettable
	profileSubscription ProfileSubscription
	dbProvider          StoreDbProvider
	state               *storestate.StoreState
	storeSource         source.Store
	ctx                 context.Context
	cancel              context.CancelFunc
	mx                  sync.Mutex
	relMapper           *relationsMapper
	cfg                 *config.Config
}

func (a *accountObject) SetDetails(ctx session.Context, details []*model.Detail, showEvent bool) (err error) {
	return a.bs.SetDetails(ctx, details, showEvent)
}

func (a *accountObject) SetDetailsAndUpdateLastUsed(ctx session.Context, details []*model.Detail, showEvent bool) (err error) {
	return a.bs.SetDetailsAndUpdateLastUsed(ctx, details, showEvent)
}

func New(
	sb smartblock.SmartBlock,
	dbProvider StoreDbProvider,
	objectStore objectstore.ObjectStore,
	layoutConverter converter.LayoutConverter,
	fileObjectService fileobject.Service,
	lastUsedUpdater lastused.ObjectUsageUpdater,
	cfg *config.Config) AccountObject {
	return &accountObject{
		bs:         basic.NewBasic(sb, objectStore, layoutConverter, fileObjectService, lastUsedUpdater),
		SmartBlock: sb,
		dbProvider: dbProvider,
		cfg:        cfg,
		relMapper: newRelationsMapper(map[string]KeyType{
			bundle.RelationKeyName.String():        KeyTypeString,
			bundle.RelationKeyDescription.String(): KeyTypeString,
			bundle.RelationKeyIconImage.String():   KeyTypeString,
			bundle.RelationKeyIconOption.String():  KeyTypeInt64,
		}),
	}
}

func (a *accountObject) Init(ctx *smartblock.InitContext) error {
	err := a.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	stateStore, err := storestate.New(ctx.Ctx, a.Id(), a.dbProvider.GetStoreDb(), accountHandler{})
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
	a.ctx, a.cancel = context.WithCancel(context.Background())
	_, err = coll.FindId(ctx.Ctx, accountDocument)
	if err != nil {
		if errors.Is(err, anystore.ErrDocNotFound) {
			var docToInsert string
			if a.cfg.IsNewAccount() {
				docToInsert = fmt.Sprintf(`{"id":"%s","analyticsId":"%s"}`, accountDocument, a.cfg.AnalyticsId)
			} else {
				docToInsert = fmt.Sprintf(`{"id":"%s"}`, accountDocument)
			}
			err = coll.Insert(ctx.Ctx, docToInsert)
			if err != nil {
				return fmt.Errorf("insert account document: %w", err)
			}
		} else {
			return fmt.Errorf("find id: %w", err)
		}
	}
	st := a.NewState()
	err = a.update(a.ctx, st)
	if err != nil {
		return fmt.Errorf("update state: %w", err)
	}
	err = a.initState(st)
	if err != nil {
		return fmt.Errorf("init state: %w", err)
	}
	return a.SmartBlock.Apply(st, smartblock.NotPushChanges, smartblock.NoHistory, smartblock.SkipIfNoChanges)
}

func (a *accountObject) initState(st *state.State) error {
	template.InitTemplate(st,
		template.WithTitle,
		template.WithForcedObjectTypes([]domain.TypeKey{bundle.TypeKeyProfile}),
		template.WithForcedDetail(bundle.RelationKeyLayout, pbtypes.Float64(float64(model.ObjectType_profile))),
		template.WithDetail(bundle.RelationKeyLayoutAlign, pbtypes.Float64(float64(model.Block_AlignCenter))),
	)
	blockId := "identity"
	st.Set(simple.New(&model.Block{
		Id: blockId,
		Content: &model.BlockContentOfRelation{
			Relation: &model.BlockContentRelation{
				Key: bundle.RelationKeyProfileOwnerIdentity.String(),
			},
		},
		Restrictions: &model.BlockRestrictions{
			Edit:   true,
			Remove: true,
			Drag:   true,
			DropOn: true,
		},
	}))
	err := st.InsertTo(state.TitleBlockID, model.Block_Bottom, blockId)
	if err != nil {
		return fmt.Errorf("insert block: %w", err)
	}
	st.SetDetail(bundle.RelationKeyIsHidden.String(), pbtypes.Bool(true))
	return nil
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

func (a *accountObject) SetAnalyticsId(id string) error {
	return a.setValue(analyticsKey, id)
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

func (a *accountObject) setValue(key string, val any) error {
	builder := &storestate.Builder{}
	err := builder.Modify(collectionName, accountDocument, []string{key}, pb.ModifyOp_Set, val)
	if err != nil {
		return nil
	}
	_, err = a.storeSource.PushStoreChange(a.ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   a.state,
		Time:    time.Now(),
	})
	return err
}

func (a *accountObject) GetAnalyticsId() (id string, err error) {
	coll, err := a.state.Collection(a.ctx, collectionName)
	if err != nil {
		err = fmt.Errorf("get collection: %w", err)
		return
	}
	obj, err := coll.FindId(a.ctx, accountDocument)
	if err != nil {
		err = fmt.Errorf("find id: %w", err)
		return
	}
	return string(obj.Value().GetStringBytes(analyticsKey)), nil
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
