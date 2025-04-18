package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type PushChangeHook func(params PushChangeParams) (id string, err error)

var _ updatelistener.UpdateListener = (*store)(nil)

type Store interface {
	Source
	ReadStoreDoc(ctx context.Context, stateStore *storestate.StoreState, onUpdateHook func()) (err error)
	PushStoreChange(ctx context.Context, params PushStoreChangeParams) (changeId string, err error)
	SetPushChangeHook(onPushChange PushChangeHook)

	// RegisterDiffManager sets a hook that will be called when a change is removed (marked as read) from the diff manager
	// must be called before ReadStoreDoc.
	//
	// If a head is marked as read in the diff manager, all earlier heads for that branch marked as read as well
	RegisterDiffManager(name string, onRemoveHook func(removed []string))
	// MarkSeenHeads marks heads as seen in a diff manager. Then the diff manager will call a hook from SetDiffManagerOnRemoveHook
	MarkSeenHeads(ctx context.Context, name string, heads []string) error
	// StoreSeenHeads persists current seen heads in any-store
	StoreSeenHeads(ctx context.Context, name string) error
	// InitDiffManager initializes a diff manager with specified seen heads
	InitDiffManager(ctx context.Context, name string, seenHeads []string) error
}

type PushStoreChangeParams struct {
	State   *storestate.StoreState
	Changes []*pb.StoreChangeContent
	Time    time.Time // used to derive the lastModifiedDate; Default is time.Now()
}

var (
	_ updatelistener.UpdateListener = (*store)(nil)
	_ Store                         = (*store)(nil)
)

type store struct {
	*source
	store        *storestate.StoreState
	onUpdateHook func()
	onPushChange PushChangeHook
	sbType       smartblock.SmartBlockType

	diffManagers map[string]*diffManager
}

type diffManager struct {
	diffManager *objecttree.DiffManager
	onRemove    func(removed []string)
}

func (s *store) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *store) SetPushChangeHook(onPushChange PushChangeHook) {
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
	for name := range s.diffManagers {
		seenHeads, err := s.loadSeenHeads(ctx, name)
		if err != nil {
			return fmt.Errorf("load seen heads: %w", err)
		}
		err = s.InitDiffManager(ctx, name, seenHeads)
		if err != nil {
			return fmt.Errorf("init diff manager: %w", err)
		}
	}
	return nil
}

func (s *store) InitDiffManager(ctx context.Context, name string, seenHeads []string) (err error) {
	manager, ok := s.diffManagers[name]
	if !ok {
		return nil
	}

	curTreeHeads := s.source.Tree().Heads()

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

	return
}

func (s *store) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
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
	case smartblock.SmartBlockTypeAccountObject:
		st.SetObjectTypeKey(bundle.TypeKeyProfile)
		st.SetDetailAndBundledRelation(bundle.RelationKeyLayout, domain.Int64(int64(model.ObjectType_profile)))
	default:
		return nil, fmt.Errorf("unsupported smartblock type: %v", s.sbType)
	}

	st.SetDetailAndBundledRelation(bundle.RelationKeyIsHidden, domain.Bool(true))
	return st, nil
}

func (s *store) PushChange(params PushChangeParams) (id string, err error) {
	if s.onPushChange != nil {
		return s.onPushChange(params)
	}
	return "", nil
}

func (s *store) ReadStoreDoc(ctx context.Context, storeState *storestate.StoreState, onUpdateHook func()) (err error) {
	s.onUpdateHook = onUpdateHook
	s.store = storeState

	err = s.initDiffManagers(ctx)
	if err != nil {
		return err
	}
	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return
	}
	// checking if we have any data in the store regarding the tree (i.e. if tree is first arrived or created)
	allIsNew := false
	if _, err := tx.GetOrder(s.id); err != nil {
		allIsNew = true
	}
	applier := &storeApply{
		tx:       tx,
		allIsNew: allIsNew,
		ot:       s.ObjectTree,
	}
	if err = applier.Apply(); err != nil {
		return errors.Join(tx.Rollback(), err)
	}
	return tx.Commit()
}

func (s *store) PushStoreChange(ctx context.Context, params PushStoreChangeParams) (changeId string, err error) {
	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return "", fmt.Errorf("new tx: %w", err)
	}
	rollback := func(err error) error {
		return errors.Join(tx.Rollback(), err)
	}

	change := &pb.StoreChange{
		ChangeSet: params.Changes,
	}
	data, dataType, err := MarshalStoreChange(change)
	if err != nil {
		return "", fmt.Errorf("marshal change: %w", err)
	}

	addResult, err := s.ObjectTree.AddContentWithValidator(ctx, objecttree.SignableChangeContent{
		Data:        data,
		Key:         s.ObjectTree.AclList().AclState().Key(),
		IsEncrypted: true,
		DataType:    dataType,
		Timestamp:   params.Time.Unix(),
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
		return "", rollback(fmt.Errorf("add content: %w", err))
	}

	if len(addResult.Added) == 0 {
		return "", rollback(fmt.Errorf("add changes list is empty"))
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

	for _, m := range s.diffManagers {
		if m.diffManager != nil {
			m.diffManager.Add(&objecttree.Change{
				Id:          changeId,
				PreviousIds: ch.PreviousIds,
			})
		}
	}

	return changeId, err
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
	for _, m := range s.diffManagers {
		if m.diffManager != nil {
			m.diffManager.Update(tree)
		}
	}
	if err == nil {
		s.onUpdateHook()
	}
	return err
}

func (s *store) MarkSeenHeads(ctx context.Context, name string, heads []string) error {
	manager, ok := s.diffManagers[name]
	if ok {
		manager.diffManager.Remove(heads)
		return s.StoreSeenHeads(ctx, name)
	}
	return nil
}

func seenHeadsCollectionName(name string) string {
	return "seenHeads/" + name
}

func (s *store) StoreSeenHeads(ctx context.Context, name string) error {
	manager, ok := s.diffManagers[name]
	if !ok {
		return nil
	}

	coll, err := s.store.Collection(ctx, seenHeadsCollectionName(name))
	if err != nil {
		return fmt.Errorf("get collection: %w", err)
	}

	seenHeads := manager.diffManager.SeenHeads()
	raw, err := json.Marshal(seenHeads)
	if err != nil {
		return fmt.Errorf("marshal seen heads: %w", err)
	}

	arena := &anyenc.Arena{}
	doc := arena.NewObject()
	doc.Set("id", arena.NewString(s.id))
	doc.Set("h", arena.NewBinary(raw))
	return coll.UpsertOne(ctx, doc)
}

func (s *store) loadSeenHeads(ctx context.Context, name string) ([]string, error) {
	coll, err := s.store.Collection(ctx, seenHeadsCollectionName(name))
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}

	doc, err := coll.FindId(ctx, s.id)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil, nil
	}

	raw := doc.Value().GetBytes("h")
	var seenHeads []string
	err = json.Unmarshal(raw, &seenHeads)
	if err != nil {
		return nil, fmt.Errorf("unmarshal seen heads: %w", err)
	}
	return seenHeads, nil
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
