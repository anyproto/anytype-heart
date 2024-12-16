package accountobject

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/anystoredebug"
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
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/metricsid"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logger.NewNamedSugared("common.editor.accountobject")

const (
	collectionName    = "account"
	accountDocumentId = "accountObject"
	idKey             = "id"
	analyticsKey      = "analyticsId"
	iconMigrationKey  = "iconMigration"
)

type ProfileDetails struct {
	Name        string
	Description string
	IconImage   string
}

type AccountObject interface {
	smartblock.SmartBlock
	anystoredebug.AnystoreDebug

	basic.DetailsSettable
	SetSharedSpacesLimit(limit int) (err error)
	SetProfileDetails(details *types.Struct) (err error)
	MigrateIconImage(image string) (err error)
	IsIconMigrated() (bool, error)
	SetAnalyticsId(analyticsId string) (err error)
	GetAnalyticsId() (string, error)
}

type StoreDbProvider interface {
	GetStoreDb() anystore.DB
}

var _ AccountObject = (*accountObject)(nil)

type accountObject struct {
	anystoredebug.AnystoreDebug
	smartblock.SmartBlock
	keys        *accountdata.AccountKeys
	bs          basic.DetailsSettable
	state       *storestate.StoreState
	storeSource source.Store
	ctx         context.Context
	cancel      context.CancelFunc
	relMapper   *editor.RelationsMapper
	cfg         *config.Config
	crdtDb      anystore.DB
}

func (a *accountObject) SetDetails(ctx session.Context, details []*model.Detail, showEvent bool) (err error) {
	return a.bs.SetDetails(ctx, details, showEvent)
}

func (a *accountObject) SetDetailsAndUpdateLastUsed(ctx session.Context, details []*model.Detail, showEvent bool) (err error) {
	return a.bs.SetDetailsAndUpdateLastUsed(ctx, details, showEvent)
}

func New(
	sb smartblock.SmartBlock,
	keys *accountdata.AccountKeys,
	spaceObjects spaceindex.Store,
	layoutConverter converter.LayoutConverter,
	fileObjectService fileobject.Service,
	lastUsedUpdater lastused.ObjectUsageUpdater,
	crdtDb anystore.DB,
	cfg *config.Config) AccountObject {
	return &accountObject{
		crdtDb:     crdtDb,
		keys:       keys,
		bs:         basic.NewBasic(sb, spaceObjects, layoutConverter, fileObjectService, lastUsedUpdater),
		SmartBlock: sb,
		cfg:        cfg,
		relMapper: editor.NewRelationsMapper(map[string]editor.KeyType{
			bundle.RelationKeyName.String():        editor.KeyTypeString,
			bundle.RelationKeyDescription.String(): editor.KeyTypeString,
			bundle.RelationKeyIconImage.String():   editor.KeyTypeString,
			bundle.RelationKeyIconOption.String():  editor.KeyTypeInt64,
		}),
	}
}

func (a *accountObject) Init(ctx *smartblock.InitContext) error {
	err := a.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	stateStore, err := storestate.New(ctx.Ctx, a.Id(), a.crdtDb, accountHandler{})
	if err != nil {
		return fmt.Errorf("create state store: %w", err)
	}
	a.state = stateStore

	a.AnystoreDebug = anystoredebug.New(a.SmartBlock, stateStore)
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
	_, err = coll.FindId(ctx.Ctx, accountDocumentId)
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		return fmt.Errorf("find id: %w", err)
	}
	if errors.Is(err, anystore.ErrDocNotFound) {
		var builder *storestate.Builder
		if a.cfg.IsNewAccount() {
			builder, err = a.genInitialDoc()
			if err != nil {
				return fmt.Errorf("generate initial doc: %w", err)
			}
			_, err = a.storeSource.PushStoreChange(ctx.Ctx, source.PushStoreChangeParams{
				Changes: builder.ChangeSet,
				State:   a.state,
				Time:    time.Now(),
			})
		} else {
			docToInsert := fmt.Sprintf(`{"%s":"%s"}`, idKey, accountDocumentId)
			err = coll.Insert(ctx.Ctx, anyenc.MustParseJson(docToInsert))
		}
		if err != nil {
			return fmt.Errorf("insert account document: %w", err)
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

func (a *accountObject) genInitialDoc() (builder *storestate.Builder, err error) {
	builder = &storestate.Builder{}
	analyticsId, err := metricsid.DeriveMetricsId(a.keys.SignKey)
	if err != nil {
		return
	}
	newDocument := map[string]any{
		idKey:            accountDocumentId,
		analyticsKey:     analyticsId,
		iconMigrationKey: "true",
	}
	for key, val := range newDocument {
		if str, ok := val.(string); ok {
			val = fmt.Sprintf(`"%s"`, str)
		}
		err = builder.Modify(collectionName, accountDocumentId, []string{key}, pb.ModifyOp_Set, val)
		if err != nil {
			return
		}
	}
	return
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
			err := builder.Modify(collectionName, accountDocumentId, []string{set.Key}, pb.ModifyOp_Set, val)
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
	builder := &storestate.Builder{}
	err := builder.Modify(collectionName, accountDocumentId, []string{analyticsKey}, pb.ModifyOp_Set, fmt.Sprintf(`"%s"`, id))
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
	err := builder.Modify(collectionName, accountDocumentId, []string{key}, pb.ModifyOp_Set, val)
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

func (a *accountObject) getValue() (val *anyenc.Value, err error) {
	coll, err := a.state.Collection(a.ctx, collectionName)
	if err != nil {
		err = fmt.Errorf("get collection: %w", err)
		return
	}
	obj, err := coll.FindId(a.ctx, accountDocumentId)
	if err != nil {
		err = fmt.Errorf("find id: %w", err)
		return
	}
	return obj.Value(), nil
}

func (a *accountObject) GetAnalyticsId() (id string, err error) {
	val, err := a.getValue()
	if err != nil {
		return
	}
	return string(val.GetStringBytes(analyticsKey)), nil
}

func (a *accountObject) TryClose(objectTTL time.Duration) (res bool, err error) {
	return false, nil
}

func (a *accountObject) Close() error {
	a.cancel()
	return a.SmartBlock.Close()
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

func (a *accountObject) MigrateIconImage(image string) (err error) {
	if image != "" {
		st := a.NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, pbtypes.String(image))
		err = a.Apply(st)
		if err != nil {
			return fmt.Errorf("set icon image: %w", err)
		}
	}
	return a.setValue(iconMigrationKey, `"true"`)
}

func (a *accountObject) IsIconMigrated() (res bool, err error) {
	val, err := a.getValue()
	if err != nil {
		return
	}
	return string(val.GetStringBytes(iconMigrationKey)) != "", nil
}

func (a *accountObject) update(ctx context.Context, st *state.State) (err error) {
	coll, err := a.state.Collection(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	accId := accountDocumentId
	obj, err := coll.FindId(ctx, accId)
	if err != nil {
		return fmt.Errorf("find id: %w", err)
	}
	for key := range a.relMapper.Keys() {
		pbVal, ok := a.relMapper.GetRelationKey(key, obj.Value())
		if !ok {
			continue
		}
		st.SetDetailAndBundledRelation(domain.RelationKey(key), pbVal)
	}
	return
}

func generatePrivateAnalyticsId() (string, error) {
	raw := make([]byte, 64)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return crypto.EncodeBytesToString(raw), nil
}
