package accountobject

import (
	"context"
	"errors"
	"fmt"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/anystoredebug"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
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
)

var log = logger.NewNamedSugared("common.editor.accountobject")

const (
	collectionName    = "account"
	accountDocumentId = "accountObject"
	idKey             = "id"
	analyticsKey      = "analyticsId"
	inboxOffsetKey    = "inboxOffset"
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
	SetProfileDetails(details *domain.Details) (err error)
	MigrateIconImage(image string) (err error)
	IsIconMigrated() (bool, error)
	SetAnalyticsId(analyticsId string) (err error)
	GetAnalyticsId() (string, error)
	SetInboxOffset(offset string) (err error)
	GetInboxOffset() (string, error)
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
	relMapper   *relationsMapper
	cfg         *config.Config
	crdtDb      anystore.DB
}

// required relations for spaceview beside the bundle.RequiredInternalRelations
var accountRequiredRelations = []domain.RelationKey{
	bundle.RelationKeyProfileOwnerIdentity,
	bundle.RelationKeySharedSpacesLimit,
}

func (a *accountObject) SetDetails(ctx session.Context, details []domain.Detail, showEvent bool) (err error) {
	return a.bs.SetDetails(ctx, details, showEvent)
}

func New(
	sb smartblock.SmartBlock,
	keys *accountdata.AccountKeys,
	spaceObjects spaceindex.Store,
	layoutConverter converter.LayoutConverter,
	fileObjectService fileobject.Service,
	crdtDb anystore.DB,
	cfg *config.Config) AccountObject {
	return &accountObject{
		crdtDb:     crdtDb,
		keys:       keys,
		bs:         basic.NewBasic(sb, spaceObjects, layoutConverter, fileObjectService),
		SmartBlock: sb,
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
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, accountRequiredRelations...)

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
	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, source.ReadStoreDocParams{
		OnUpdateHook: a.onUpdate,
	})
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

	err = a.update(a.ctx, ctx.State)
	if err != nil {
		return fmt.Errorf("update state: %w", err)
	}
	err = a.initState(ctx.State)
	if err != nil {
		return fmt.Errorf("init state: %w", err)
	}
	return a.SmartBlock.Apply(ctx.State, smartblock.NotPushChanges, smartblock.NoHistory)
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
		template.WithLayout(model.ObjectType_profile),
		template.WithDetail(bundle.RelationKeyLayoutAlign, domain.Int64(model.Block_AlignCenter)),
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
	st.SetDetail(bundle.RelationKeyIsHidden, domain.Bool(true))
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
	return a.setValue(analyticsKey, id)
}

func (a *accountObject) SetInboxOffset(offset string) error {
	return a.setValue(inboxOffsetKey, offset)
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
	err := builder.Modify(collectionName, accountDocumentId, []string{key}, pb.ModifyOp_Set, fmt.Sprintf(`"%s"`, val))
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

func (a *accountObject) getStringValue(key string) (stringValue string, err error) {
	val, err := a.getValue()
	if err != nil {
		return
	}
	return string(val.GetStringBytes(key)), nil
}

func (a *accountObject) GetAnalyticsId() (id string, err error) {
	return a.getStringValue(analyticsKey)
}

func (a *accountObject) GetInboxOffset() (id string, err error) {
	return a.getStringValue(inboxOffsetKey)
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
	st.SetDetailAndBundledRelation(bundle.RelationKeySharedSpacesLimit, domain.Int64(limit))
	return a.Apply(st)
}

func (a *accountObject) GetSharedSpacesLimit() (limit int) {
	return int(a.CombinedDetails().GetInt64(bundle.RelationKeySharedSpacesLimit))
}

func (a *accountObject) SetProfileDetails(details *domain.Details) (err error) {
	st := a.NewState()
	// we should set everything in local state, but not everything in the store (this should be filtered in OnPushChange)
	for key, value := range details.Iterate() {
		st.SetDetailAndBundledRelation(key, value)
	}
	return a.Apply(st)
}

func (a *accountObject) MigrateIconImage(image string) (err error) {
	if image != "" {
		st := a.NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, domain.String(image))
		err = a.Apply(st)
		if err != nil {
			return fmt.Errorf("set icon image: %w", err)
		}
	}
	return a.setValue(iconMigrationKey, "true")
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
	for key := range a.relMapper.keys {
		pbVal, ok := a.relMapper.GetRelationKey(key, obj.Value())
		if !ok {
			continue
		}
		st.SetDetailAndBundledRelation(domain.RelationKey(key), pbVal)
	}
	return
}
