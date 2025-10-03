package sourceimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/keyvalueservice"
)

var _ updatelistener.UpdateListener = (*store)(nil)

var (
	_ updatelistener.UpdateListener = (*store)(nil)
	_ source.Store                  = (*store)(nil)
)

type store struct {
	*treeSource
	spaceService space.Service
	store        *storestate.StoreState
	onUpdateHook func()
	onPushChange source.PushChangeHook
	sbType       smartblock.SmartBlockType

	diffManagers map[string]*diffManager
}

type DiffManagerStats struct {
	DiffManagerName string   `json:"diffManagerName"`
	SeenHeads       []string `json:"seenHeads"`
	AllChanges      []string `json:"allChanges"`
	AllChangesCount int      `json:"allChangesCount"`
}

type StoreStat struct {
	DiffManagers []DiffManagerStats `json:"diffManagers"`
}

func (s *store) ProvideStat() any {
	stats := make([]DiffManagerStats, 0, len(s.diffManagers))
	for name, manager := range s.diffManagers {
		ids := manager.diffManager.GetIds()
		stats = append(stats, DiffManagerStats{
			DiffManagerName: name,
			SeenHeads:       manager.diffManager.SeenHeads(),
			AllChanges:      ids[0:min(len(ids), 1000)],
			AllChangesCount: len(ids),
		})
	}
	return StoreStat{
		DiffManagers: stats,
	}
}

func (s *store) StatId() string {
	return s.Id()
}

func (s *store) StatType() string {
	return "source.store"
}

type diffManager struct {
	diffManager *objecttree.DiffManager
	onRemove    func(removed []string)
}

func (s *store) getTechSpace() clientspace.Space {
	return s.spaceService.TechSpace()
}

func (s *store) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *store) SetPushChangeHook(onPushChange source.PushChangeHook) {
	s.onPushChange = onPushChange
}

func (s *store) RegisterDiffManager(name string, onRemoveHook func(removed []string)) {
	if _, ok := s.diffManagers[name]; !ok {
		s.diffManagers[name] = &diffManager{
			onRemove: onRemoveHook,
		}
	}
}

func (s *store) initDiffManagers(ctx context.Context) error {
	for name, manager := range s.diffManagers {
		err := s.InitDiffManager(ctx, name, nil)
		if err != nil {
			return fmt.Errorf("init diff manager: %w", err)
		}

		vals, err := s.getTechSpace().KeyValueService().Get(ctx, s.seenHeadsKey(name))
		if err != nil {
			log.With("error", err).Error("init diff manager: get value")
			continue
		}
		for _, val := range vals {
			seenHeads, err := unmarshalSeenHeads(val.Data)
			if err != nil {
				log.With("error", err).Error("init diff manager: unmarshal seen heads")
				continue
			}
			manager.diffManager.Remove(seenHeads)
		}
	}
	return nil
}

func unmarshalSeenHeads(raw []byte) ([]string, error) {
	var seenHeads []string
	err := json.Unmarshal(raw, &seenHeads)
	if err != nil {
		return nil, err
	}
	return seenHeads, nil
}

func (s *store) InitDiffManager(ctx context.Context, name string, seenHeads []string) (err error) {
	manager, ok := s.diffManagers[name]
	if !ok {
		return nil
	}

	curTreeHeads := s.treeSource.Tree().Heads()

	buildTree := func(heads []string) (objecttree.ReadableObjectTree, error) {
		return s.space.TreeBuilder().BuildHistoryTree(ctx, s.Id(), objecttreebuilder.HistoryTreeOpts{
			Heads:   heads,
			Include: true,
		})
	}
	onRemove := func(removed []string) {
		if manager.onRemove != nil {
			manager.onRemove(removed)
		}
	}

	manager.diffManager, err = objecttree.NewDiffManager(seenHeads, curTreeHeads, buildTree, onRemove)
	if err != nil {
		return fmt.Errorf("init diff manager: %w", err)
	}
	manager.diffManager.Init()

	err = s.getTechSpace().KeyValueService().SubscribeForKey(s.seenHeadsKey(name), name, func(key string, val keyvalueservice.Value) {
		s.ObjectTree.Lock()
		defer s.ObjectTree.Unlock()

		newSeenHeads, err := unmarshalSeenHeads(val.Data)
		if err != nil {
			log.Errorf("subscribe for seenHeads: %s: %v", name, err)
			return
		}
		manager.diffManager.Remove(newSeenHeads)
	})
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	return
}

func (s *store) ReadDoc(ctx context.Context, receiver source.ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s.receiver = receiver
	setter, ok := s.ObjectTree.(synctree.ListenerSetter)
	if !ok {
		err = fmt.Errorf("should be able to set listner inside object tree")
		return
	}
	setter.SetListener(s)

	// Fake state, this kind of objects not support state operations

	st := state.NewDoc(s.id, nil).(*state.State)
	// Set object type here in order to derive value of Type relation in smartblock.Init
	switch s.sbType {
	case smartblock.SmartBlockTypeChatDerivedObject:
		st.SetObjectTypeKey(bundle.TypeKeyChatDerived)
		st.SetDetailAndBundledRelation(bundle.RelationKeyLayout, domain.Int64(int64(model.ObjectType_chatDerived)))
		st.SetDetailAndBundledRelation(bundle.RelationKeyIsHidden, domain.Bool(false))
	case smartblock.SmartBlockTypeAccountObject:
		st.SetObjectTypeKey(bundle.TypeKeyProfile)
		st.SetDetailAndBundledRelation(bundle.RelationKeyLayout, domain.Int64(int64(model.ObjectType_profile)))
		st.SetDetailAndBundledRelation(bundle.RelationKeyIsHidden, domain.Bool(true))
	default:
		return nil, fmt.Errorf("unsupported smartblock type: %v", s.sbType)
	}

	return st, nil
}

func (s *store) PushChange(params source.PushChangeParams) (id string, err error) {
	if s.onPushChange != nil {
		return s.onPushChange(params)
	}
	return "", nil
}

func (s *store) ReadStoreDoc(ctx context.Context, storeState *storestate.StoreState, params source.ReadStoreDocParams) (err error) {
	s.onUpdateHook = params.OnUpdateHook
	s.store = storeState

	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()
	// checking if we have any data in the store regarding the tree (i.e. if tree is first arrived or created)
	allIsNew := false
	if _, err := tx.GetOrder(s.id); err != nil {
		allIsNew = true
	}
	applier := &storeApply{
		tx:       tx,
		allIsNew: allIsNew,
		ot:       s.ObjectTree,
		hook:     params.ReadStoreTreeHook,
	}
	if err = applier.Apply(); err != nil {
		return err
	}
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	err = s.initDiffManagers(ctx)
	if err != nil {
		return fmt.Errorf("init diff managers: %w", err)
	}

	if params.ReadStoreTreeHook != nil {
		err = params.ReadStoreTreeHook.AfterDiffManagersInit(ctx)
		if err != nil {
			return fmt.Errorf("after diff managers init hook: %w", err)
		}
	}
	return nil
}

func (s *store) PushStoreChange(ctx context.Context, params source.PushStoreChangeParams) (changeId string, err error) {
	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return "", fmt.Errorf("new tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	change := &pb.StoreChange{
		ChangeSet: params.Changes,
	}
	data, dataType, err := MarshalStoreChange(change)
	if err != nil {
		return "", fmt.Errorf("marshal change: %w", err)
	}

	addResult, err := s.ObjectTree.AddContentWithValidator(ctx, objecttree.SignableChangeContent{
		Data:              data,
		Key:               s.ObjectTree.AclList().AclState().Key(),
		ShouldBeEncrypted: true,
		DataType:          dataType,
		Timestamp:         params.Time.Unix(),
	}, func(change objecttree.StorageChange) error {
		err = tx.ApplyChangeSet(storestate.ChangeSet{
			Id:        change.Id,
			Order:     change.OrderId,
			Changes:   params.Changes,
			Creator:   s.accountService.AccountID(),
			Timestamp: params.Time.Unix(),
		})
		if err != nil {
			return fmt.Errorf("apply change set: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("add content: %w", err)
	}

	if len(addResult.Added) == 0 {
		return "", fmt.Errorf("add changes list is empty")
	}
	changeId = addResult.Added[0].Id
	err = tx.Commit()
	if err == nil {
		s.onUpdateHook()
	}
	ch, err := s.ObjectTree.GetChange(changeId)
	if err != nil {
		return "", err
	}

	s.addToDiffManagers(&objecttree.Change{
		Id:          changeId,
		PreviousIds: ch.PreviousIds,
	})

	return changeId, err
}

func (s *store) addToDiffManagers(change *objecttree.Change) {
	for _, m := range s.diffManagers {
		if m.diffManager != nil {
			m.diffManager.Add(change)
		}
	}
}

func (s *store) update(ctx context.Context, tree objecttree.ObjectTree) error {
	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return err
	}
	applier := &storeApply{
		tx:                   tx,
		ot:                   tree,
		needFetchPrevOrderId: true,
	}
	if err = applier.Apply(); err != nil {
		return errors.Join(tx.Rollback(), err)
	}
	err = tx.Commit()

	s.updateInDiffManagers(tree)
	if err == nil {
		s.onUpdateHook()
	}
	return err
}

func (s *store) updateInDiffManagers(tree objecttree.ObjectTree) {
	for _, m := range s.diffManagers {
		if m.diffManager != nil {
			m.diffManager.Update(tree)
		}
	}
}

func (s *store) MarkSeenHeads(ctx context.Context, name string, heads []string) error {
	manager, ok := s.diffManagers[name]
	if ok {
		manager.diffManager.Remove(heads)
		return s.StoreSeenHeads(ctx, name)
	}
	return nil
}

func (s *store) StoreSeenHeads(ctx context.Context, name string) error {
	manager, ok := s.diffManagers[name]
	if !ok {
		return nil
	}

	seenHeads := manager.diffManager.SeenHeads()
	raw, err := json.Marshal(seenHeads)
	if err != nil {
		return fmt.Errorf("marshal seen heads: %w", err)
	}

	return s.getTechSpace().KeyValueService().Set(ctx, s.seenHeadsKey(name), raw)
}

func (s *store) seenHeadsKey(diffManagerName string) string {
	return s.id + diffManagerName
}

func (s *store) Update(tree objecttree.ObjectTree) error {
	err := s.update(context.Background(), tree)
	if err != nil {
		log.With("objectId", s.id).Errorf("update: failed to read store doc: %v", err)
	}
	return err
}

func (s *store) Rebuild(tree objecttree.ObjectTree) error {
	err := s.update(context.Background(), tree)
	if err != nil {
		log.With("objectId", s.id).Errorf("rebuild: failed to read store doc: %v", err)
	}
	return err
}

func MarshalStoreChange(change *pb.StoreChange) (result []byte, dataType string, err error) {
	data := bytesPool.Get().([]byte)[:0]
	defer func() {
		bytesPool.Put(data)
	}()

	data = slices.Grow(data, change.Size())
	n, err := change.MarshalTo(data)
	if err != nil {
		return
	}
	data = data[:n]

	if n > snappyLowerLimit {
		result = snappy.Encode(nil, data)
		dataType = dataTypeSnappy
	} else {
		result = bytes.Clone(data)
	}

	return
}

func UnmarshalStoreChange(treeChange *objecttree.Change, data []byte) (result any, err error) {
	change := &pb.StoreChange{}
	if treeChange.DataType == dataTypeSnappy {
		buf := bytesPool.Get().([]byte)[:0]
		defer func() {
			bytesPool.Put(buf)
		}()

		var n int
		if n, err = snappy.DecodedLen(data); err == nil {
			buf = slices.Grow(buf, n)[:n]
			var decoded []byte
			decoded, err = snappy.Decode(buf, data)
			if err == nil {
				data = decoded
			}
		}
	}
	if err = proto.Unmarshal(data, change); err == nil {
		result = change
	}
	return
}
