package editor

import (
	"errors"
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/doc"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	smartblock2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	database2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes/pbconvert"
	"github.com/gogo/protobuf/types"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const (
	collectionKeySignature = "signature"
	collectionKeyAccount   = "account"
	collectionKeyAddrs     = "addrs"
	collectionKeyId        = "id"
	collectionKeyKey       = "key"

	collectionNameRelations    = "relations"
	collectionNameRelationOpts = "relationOpts"
)

var (
	ErrRelationNotFound = errors.New("relation not found")
)

func NewWorkspace(dbCtrl database.Ctrl, dmservice DetailsModifier) *Workspaces {
	return &Workspaces{
		Set:             NewSet(dbCtrl),
		DetailsModifier: dmservice,
	}
}

type Workspaces struct {
	*Set
	DetailsModifier DetailsModifier
	threadService   threads.Service
	threadQueue     threads.ThreadQueue

	changedRelationIds, changedRelationIdsOptions []string
}

type WorkspaceParameters struct {
	IsHighlighted bool
	WorkspaceId   string
}

func (wp *WorkspaceParameters) Equal(other *WorkspaceParameters) bool {
	return wp.IsHighlighted == other.IsHighlighted
}

func (p *Workspaces) CreateObject(id thread.ID, sbType smartblock2.SmartBlockType) (core.SmartBlock, error) {
	st := p.NewState()
	if !id.Defined() {
		var err error
		id, err = threads.ThreadCreateID(thread.AccessControlled, sbType)
		if err != nil {
			return nil, err
		}
	}
	threadInfo, err := p.threadQueue.CreateThreadSync(id, p.Id())
	if err != nil {
		return nil, err
	}
	st.SetInStore([]string{source.WorkspaceCollection, threadInfo.ID.String()}, p.pbThreadInfoValueFromStruct(threadInfo))

	return core.NewSmartBlock(threadInfo, p.Anytype()), p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

func (p *Workspaces) DeleteObject(objectId string) error {
	st := p.NewState()
	err := p.threadQueue.DeleteThreadSync(objectId, p.Id())
	if err != nil {
		return err
	}
	st.RemoveFromStore([]string{source.WorkspaceCollection, objectId})
	return p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

func (p *Workspaces) GetAllObjects() []string {
	st := p.NewState()
	workspaceCollection := st.GetCollection(source.WorkspaceCollection)
	if workspaceCollection == nil || workspaceCollection.Fields == nil {
		return nil
	}
	objects := make([]string, 0, len(workspaceCollection.Fields))
	for objId, workspaceId := range workspaceCollection.Fields {
		if v, ok := workspaceId.Kind.(*types.Value_StringValue); ok && v.StringValue == p.Id() {
			objects = append(objects, objId)
		}
	}
	return objects
}

func (p *Workspaces) AddCreatorInfoIfNeeded() error {
	st := p.NewState()
	deviceId := p.Anytype().Device()

	creatorCollection := st.GetCollection(source.CreatorCollection)
	if creatorCollection != nil && creatorCollection.Fields != nil && creatorCollection.Fields[deviceId] != nil {
		return nil
	}
	info, err := p.threadService.GetCreatorInfo(p.Id())
	if err != nil {
		return err
	}
	st.SetInStore([]string{source.CreatorCollection, deviceId}, p.pbCreatorInfoValue(info))

	return p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

func (p *Workspaces) MigrateMany(infos []threads.ThreadInfo) (int, error) {
	st := p.NewState()
	migrated := 0
	for _, info := range infos {
		if st.ContainsInStore([]string{source.AccountMigration, info.ID}) {
			continue
		}
		st.SetInStore([]string{source.AccountMigration, info.ID}, pbtypes.Bool(true))
		st.SetInStore([]string{source.WorkspaceCollection, info.ID},
			p.pbThreadInfoValue(info.ID, info.Key, info.Addrs),
		)
		migrated++
	}

	err := p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
	if err != nil {
		return 0, err
	}

	return migrated, nil
}

func (p *Workspaces) AddObject(objectId string, key string, addrs []string) error {
	st := p.NewState()
	err := p.threadQueue.AddThreadSync(threads.ThreadInfo{
		ID:    objectId,
		Key:   key,
		Addrs: addrs,
	}, p.Id())
	if err != nil {
		return err
	}
	st.SetInStore([]string{source.WorkspaceCollection, objectId}, p.pbThreadInfoValue(objectId, key, addrs))

	return p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

func (p *Workspaces) GetObjectKeyAddrs(objectId string) (string, []string, error) {
	threadId, err := thread.Decode(objectId)
	if err != nil {
		return "", nil, fmt.Errorf("failed to decode object %s: %w", objectId, err)
	}

	// we could have gotten the data from state, but to be sure 100% let's take it from service :-)
	threadInfo, err := p.threadService.GetThreadInfo(threadId)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get info on the thread %s: %w", objectId, err)
	}
	var publicAddrs []ma.Multiaddr
	for _, adr := range threadInfo.Addrs {
		// ignore cafe addr if it is there because we will add this anyway
		if manet.IsPublicAddr(adr) && adr.String() != p.threadService.CafePeer().String() {
			publicAddrs = append(publicAddrs, adr)
		}
	}
	if len(publicAddrs) > 2 {
		publicAddrs = publicAddrs[len(publicAddrs)-2:]
	}
	publicAddrs = append(publicAddrs, p.threadService.CafePeer())

	return threadInfo.Key.String(), util.MultiAddressesToStrings(publicAddrs), nil
}

func (p *Workspaces) SetIsHighlighted(objectId string, value bool) error {
	// TODO: this should be removed probably in the future?
	if p.Anytype().PredefinedBlocks().IsAccount(p.Id()) {
		return fmt.Errorf("highlighting not supported for the account space")
	}

	st := p.NewState()
	st.SetInStore([]string{source.HighlightedCollection, objectId}, pbtypes.Bool(value))
	return p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

func (p *Workspaces) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.Source.Type() != model.SmartBlockType_Workspace && ctx.Source.Type() != model.SmartBlockType_AccountOld {
		return fmt.Errorf("source type should be a workspace or an old account")
	}

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	p.threadService = p.Anytype().ThreadsService()
	p.threadQueue = p.Anytype().ThreadsService().ThreadQueue()

	dataviewAllHighlightedObjects := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source:    []string{addr.BundledRelationURLPrefix + bundle.RelationKeyName.String()},
			Relations: []*model.Relation{bundle.MustGetRelation(bundle.RelationKeyName)},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   "_view1_1",
					Type: model.BlockContentDataviewView_Gallery,
					Name: "Highlighted",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: []*model.BlockContentDataviewRelation{
						{
							Key:       bundle.RelationKeyName.String(),
							IsVisible: true,
						},
						{
							Key:       bundle.RelationKeyCreator.String(),
							IsVisible: true,
						},
					},
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: bundle.RelationKeyWorkspaceId.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String(p.Id()),
					}, {
						RelationKey: bundle.RelationKeyId.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.String(p.Id()),
					}, {
						RelationKey: bundle.RelationKeyIsHighlighted.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Bool(true),
					}},
				},
			},
		},
	}

	dataviewAllWorkspaceObjects := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source:    []string{addr.BundledRelationURLPrefix + bundle.RelationKeyName.String()},
			Relations: []*model.Relation{bundle.MustGetRelation(bundle.RelationKeyName), bundle.MustGetRelation(bundle.RelationKeyCreator)},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   "_view2_1",
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: []*model.BlockContentDataviewRelation{
						{
							Key:       bundle.RelationKeyName.String(),
							IsVisible: true,
						},
						{
							Key:       bundle.RelationKeyCreator.String(),
							IsVisible: true,
						},
					},
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: bundle.RelationKeyWorkspaceId.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String(p.Id()),
					}, {
						RelationKey: bundle.RelationKeyId.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.String(p.Id()),
					}},
				},
			},
		},
	}

	p.AddHook(p.updateObjects, smartblock.HookAfterApply)
	p.AddHook(p.checkUpdatedRelations, smartblock.HookBeforeApply)
	defaultValue := &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyWorkspaceId.String(): pbtypes.String(p.Id())}}
	return smartblock.ApplyTemplate(p, ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithFeaturedRelations,
		template.WithCondition(p.Anytype().PredefinedBlocks().IsAccount(p.Id()),
			template.WithDetail(bundle.RelationKeyIsHidden, pbtypes.Bool(true))),
		template.WithCondition(p.Anytype().PredefinedBlocks().IsAccount(p.Id()),
			template.WithForcedDetail(bundle.RelationKeyName, pbtypes.String("Personal space"))),
		template.WithForcedDetail(bundle.RelationKeyFeaturedRelations, pbtypes.StringList([]string{bundle.RelationKeyType.String(), bundle.RelationKeyCreator.String()})),
		template.WithDataviewID("highlighted", dataviewAllHighlightedObjects, false),
		template.WithDataviewID(template.DataviewBlockId, dataviewAllWorkspaceObjects, false),
		template.WithBlockField(template.DataviewBlockId, dataview.DefaultDetailsFieldName, pbtypes.Struct(defaultValue)),
	)
}

// TODO: try to save results from processing of previous state and get changes from apply for performance
func (p *Workspaces) updateObjects(st *state.State) error {
	objects, parameters := p.workspaceObjectsAndParametersFromState(st)
	startTime := time.Now()
	p.threadQueue.ProcessThreadsAsync(objects, p.Id())
	metrics.SharedClient.RecordEvent(metrics.ProcessThreadsEvent{WaitTimeMs: time.Now().Sub(startTime).Milliseconds()})
	if !p.Anytype().PredefinedBlocks().IsAccount(p.Id()) {
		storedParameters := p.workspaceParametersFromRecords(p.storedRecordsForWorkspace())
		// we ignore the workspace object itself
		delete(storedParameters, p.Id())
		p.updateDetailsIfParametersChanged(storedParameters, parameters)
	}
	return nil
}

func (p *Workspaces) storedRecordsForWorkspace() []database2.Record {
	records, _, err := p.ObjectStore().Query(nil, database2.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyWorkspaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(p.Id()),
			},
		},
	})
	if err != nil {
		log.Errorf("workspace: can't get store workspace ids: %v", err)
		return nil
	}
	return records
}

func (p *Workspaces) workspaceParametersFromRecords(records []database2.Record) map[string]*WorkspaceParameters {
	var storeObjectInWorkspace = make(map[string]*WorkspaceParameters, len(records))
	for _, rec := range records {
		id := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())
		storeObjectInWorkspace[id] = &WorkspaceParameters{
			IsHighlighted: pbtypes.GetBool(rec.Details, bundle.RelationKeyIsHighlighted.String()),
			WorkspaceId:   pbtypes.GetString(rec.Details, bundle.RelationKeyWorkspaceId.String()),
		}
	}
	return storeObjectInWorkspace
}

func (p *Workspaces) workspaceObjectsAndParametersFromState(st *state.State) ([]threads.ThreadInfo, map[string]*WorkspaceParameters) {
	workspaceCollection := st.GetCollection(source.WorkspaceCollection)
	if workspaceCollection == nil || workspaceCollection.Fields == nil {
		return nil, nil
	}
	parameters := make(map[string]*WorkspaceParameters, len(workspaceCollection.Fields))
	objects := make([]threads.ThreadInfo, 0, len(workspaceCollection.Fields))
	for objId, value := range workspaceCollection.Fields {
		if value == nil {
			continue
		}
		parameters[objId] = &WorkspaceParameters{
			IsHighlighted: false,
			WorkspaceId:   p.Id(),
		}
		objects = append(objects, p.threadInfoFromWorkspacePB(value))
	}

	creatorCollection := st.GetCollection(source.CreatorCollection)
	if creatorCollection != nil {
		for _, value := range creatorCollection.Fields {
			info, err := p.threadInfoFromCreatorPB(value)
			if err != nil {
				continue
			}
			objects = append(objects, info)
		}
	}
	highlightedCollection := st.GetCollection(source.HighlightedCollection)
	if highlightedCollection != nil {
		for objId, isHighlighted := range highlightedCollection.Fields {
			if pbtypes.IsExpectedBoolValue(isHighlighted, true) {
				if _, exists := parameters[objId]; exists {
					parameters[objId].IsHighlighted = true
				}
			}
		}
	}

	return objects, parameters
}

func (p *Workspaces) updateDetailsIfParametersChanged(
	oldParameters map[string]*WorkspaceParameters,
	newParameters map[string]*WorkspaceParameters) {
	for id, params := range newParameters {
		if oldParams, exists := oldParameters[id]; exists && oldParams.Equal(params) {
			continue
		}

		// TODO: we need to move it to another service, but now it is what it is
		go func(id string, params WorkspaceParameters) {
			if err := p.DetailsModifier.ModifyLocalDetails(id, func(current *types.Struct) (*types.Struct, error) {
				if current == nil || current.Fields == nil {
					current = &types.Struct{
						Fields: map[string]*types.Value{},
					}
				}
				current.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(params.WorkspaceId)
				current.Fields[bundle.RelationKeyIsHighlighted.String()] = pbtypes.Bool(params.IsHighlighted)

				return current, nil
			}); err != nil {
				log.Errorf("workspace: can't set detail to object: %v", err)
			}
		}(id, *params)
	}
}

func (p *Workspaces) pbCreatorInfoValue(info threads.CreatorInfo) *types.Value {
	return &types.Value{
		Kind: &types.Value_StructValue{
			StructValue: &types.Struct{
				Fields: map[string]*types.Value{
					collectionKeyAccount:   pbtypes.String(info.AccountPubKey),
					collectionKeySignature: pbtypes.String(string(info.WorkspaceSig)),
					collectionKeyAddrs:     pbtypes.StringList(info.Addrs),
				},
			},
		},
	}
}

func (p *Workspaces) pbThreadInfoValue(id string, key string, addrs []string) *types.Value {
	return &types.Value{
		Kind: &types.Value_StructValue{
			StructValue: &types.Struct{
				Fields: map[string]*types.Value{
					collectionKeyId:    pbtypes.String(id),
					collectionKeyKey:   pbtypes.String(key),
					collectionKeyAddrs: pbtypes.StringList(addrs),
				},
			},
		},
	}
}

func (p *Workspaces) pbThreadInfoValueFromStruct(ti thread.Info) *types.Value {
	return p.pbThreadInfoValue(ti.ID.String(), ti.Key.String(), util.MultiAddressesToStrings(ti.Addrs))
}

func (p *Workspaces) threadInfoFromWorkspacePB(val *types.Value) threads.ThreadInfo {
	fields := val.Kind.(*types.Value_StructValue).StructValue
	return threads.ThreadInfo{
		ID:    pbtypes.GetString(fields, collectionKeyId),
		Key:   pbtypes.GetString(fields, collectionKeyKey),
		Addrs: pbtypes.GetStringListValue(fields.Fields[collectionKeyAddrs]),
	}
}

func (p *Workspaces) threadInfoFromCreatorPB(val *types.Value) (threads.ThreadInfo, error) {
	fields := val.Kind.(*types.Value_StructValue).StructValue
	account := pbtypes.GetString(fields, collectionKeyAccount)
	profileId, err := threads.ProfileThreadIDFromAccountAddress(account)
	if err != nil {
		return threads.ThreadInfo{}, err
	}
	sk, pk, err := threads.ProfileThreadKeysFromAccountAddress(account)
	if err != nil {
		return threads.ThreadInfo{}, err
	}
	return threads.ThreadInfo{
		ID:    profileId.String(),
		Key:   thread.NewKey(sk, pk).String(),
		Addrs: pbtypes.GetStringListValue(fields.Fields[collectionKeyAddrs]),
	}, nil
}

func (p *Workspaces) SaveRelations(rels ...*model.Relation) (err error) {
	s := p.NewState()
	for _, rel := range rels {
		s.SetInStore([]string{collectionNameRelations, rel.Key}, pbconvert.RelationToValue(rel))
	}
	return p.Apply(s, smartblock.NoHistory, smartblock.NoEvent)
}

func (p *Workspaces) GetRelation(relKey string) (r *model.Relation, err error) {
	s := p.NewState()
	v := pbtypes.Get(s.Store(), collectionNameRelations, relKey)
	if v == nil {
		return nil, ErrRelationNotFound
	}
	r = pbconvert.StructToRelation(v.GetStructValue())
	if r == nil {
		err = ErrRelationNotFound
	}
	return
}

func (p *Workspaces) SetRelationOptions(relKey string, opts ...*model.RelationOption) (err error) {
	s := p.NewState()
	for _, opt := range opts {
		s.SetInStore([]string{collectionNameRelationOpts, relKey, opt.Id}, pbconvert.RelationOptionToValue(opt))
	}
	return p.Apply(s, smartblock.NoHistory, smartblock.NoEvent)
}

func (p *Workspaces) GetDocInfo() (info doc.DocInfo, err error) {
	if info, err = p.SmartBlock.GetDocInfo(); err != nil {
		return
	}
	s := p.NewState()
	rels := pbtypes.Get(s.Store(), collectionNameRelations).GetStructValue()
	if rels == nil && rels.Fields == nil {
		return
	}

	for _, v := range rels.Fields {
		info.AllRelations = append(info.AllRelations, v.GetStructValue())
	}

	return
}

func (p *Workspaces) checkUpdatedRelations(s *state.State) (err error) {
	paths := s.GetChangedStoreKeys(collectionNameRelations)
	for _, path := range paths {
		if len(path) >= 2 {
			p.changedRelationIds = append(p.changedRelationIds, path[1])
		}
	}
	paths = s.GetChangedStoreKeys(collectionNameRelationOpts)
	for _, path := range paths {
		if len(path) >= 2 {
			p.changedRelationIdsOptions = append(p.changedRelationIdsOptions, path[1])
		}
	}
	return
}
