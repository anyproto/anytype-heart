package sourceimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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
	// SeenFrontierCount is the size of the pruned resolved seen frontier
	// (len(w.seen), typically 1-2 heads) — NOT a total change count.
	SeenFrontierCount int `json:"seenFrontierCount"`
}

type StoreStat struct {
	DiffManagers []DiffManagerStats `json:"diffManagers"`
}

func (s *store) ProvideStat() any {
	stats := make([]DiffManagerStats, 0, len(s.diffManagers))
	for name, manager := range s.diffManagers {
		stats = append(stats, DiffManagerStats{
			DiffManagerName:   name,
			SeenHeads:         manager.wm.seenHeadIds(),
			SeenFrontierCount: manager.wm.frontierLen(),
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
	wm         *watermark
	onRemove   func(removed []string)
	candidates source.CandidateProvider
}

func (s *store) getTechSpace() clientspace.Space {
	return s.spaceService.TechSpace()
}

func (s *store) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *store) AclList() list.AclList {
	return s.ObjectTree.AclList()
}

func (s *store) SetPushChangeHook(onPushChange source.PushChangeHook) {
	s.onPushChange = onPushChange
}

func (s *store) RegisterDiffManager(name string, onRemoveHook func(removed []string), candidateProvider source.CandidateProvider) {
	if _, ok := s.diffManagers[name]; !ok {
		s.diffManagers[name] = &diffManager{
			onRemove:   onRemoveHook,
			wm:         newWatermark(onRemoveHook),
			candidates: candidateProvider,
		}
	}
}

// resolvePair looks up a change's local read coordinates (OrderId, AddSeq).
// Uses Storage().Get (lock-free, always finds inserted changes) rather than
// Tree().GetChange (lock-sensitive; only the in-memory attached map).
func (s *store) resolvePair(id string) (readPair, bool) {
	ch, err := s.treeSource.Tree().Storage().Get(context.Background(), id)
	if err != nil {
		return readPair{}, false
	}
	return readPair{OrderId: ch.OrderId, AddSeq: ch.AddSeq}, true
}

// eachChange streams every change's (id, pair) from tree storage — id/OrderId/
// AddSeq only, no payload decode, no graph, no BuildHistoryTree.
func (s *store) eachChange(ctx context.Context) func(yield func(string, readPair)) {
	return func(yield func(string, readPair)) {
		err := s.treeSource.Tree().Storage().GetAfterOrder(ctx, "", func(_ context.Context, ch objecttree.StorageChange) (bool, error) {
			yield(ch.Id, readPair{OrderId: ch.OrderId, AddSeq: ch.AddSeq})
			return true, nil
		})
		if err != nil {
			// A truncated change stream silently under-emits reads (some
			// messages stay unread). Log so the failure is observable, matching
			// the sibling fallback path in eachChangeFor.
			log.With("error", err).Error("eachChange: tree storage scan")
		}
	}
}

// boundedCandidateFallbackThreshold caps the bounded path: above this many
// unread candidates, reverting to the single full tree stream is cheaper than
// N point lookups. Tunable; benchmarked in the cold-start measurement task.
const boundedCandidateFallbackThreshold = 5000

// buildBoundedEachChange yields (id, pair) for each candidate id that resolves
// in tree storage, skipping unresolved ids. Pure: no store/DB dependency.
func buildBoundedEachChange(ids []string, resolve func(string) (readPair, bool)) func(yield func(string, readPair)) {
	return func(yield func(string, readPair)) {
		for _, id := range ids {
			if p, ok := resolve(id); ok {
				yield(id, p)
			}
		}
	}
}

// eachChangeFor returns the (id, pair) stream for a diff manager: the bounded
// unread-candidate stream when a provider is set and the candidate set is not
// pathologically large, else the legacy full tree-change stream. Falling back
// is correctness-safe — advance's dominated/marked logic is identical for any
// superset of the dominated ids.
func (s *store) eachChangeFor(ctx context.Context, manager *diffManager) func(yield func(string, readPair)) {
	if manager.candidates == nil {
		return s.eachChange(ctx)
	}
	ids, err := manager.candidates(ctx)
	if err != nil {
		log.With("error", err).Error("bounded candidates: fallback to full scan")
		return s.eachChange(ctx)
	}
	if len(ids) > boundedCandidateFallbackThreshold {
		return s.eachChange(ctx)
	}
	return buildBoundedEachChange(ids, s.resolvePair)
}

func (s *store) initDiffManagers(ctx context.Context) error {
	for name := range s.diffManagers {
		// Merge ALL persisted per-device seenHeads values into one set and
		// apply in a SINGLE InitDiffManager/advance per counter. The previous
		// per-value advance loop ran a full change scan + onRemove for every
		// device value (× counters) — the cold-start regression.
		var merged []string
		vals, err := s.getTechSpace().KeyValueService().Get(ctx, s.seenHeadsKey(name))
		if err != nil {
			log.With("error", err).Error("init diff manager: get value")
		} else {
			for _, val := range vals {
				seenHeads, uerr := unmarshalSeenHeads(val.Data)
				if uerr != nil {
					log.With("error", uerr).Error("init diff manager: unmarshal seen heads")
					continue
				}
				merged = append(merged, seenHeads...)
			}
		}
		if err := s.InitDiffManager(ctx, name, merged); err != nil {
			return fmt.Errorf("init diff manager: %w", err)
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

	// Fresh engine per init: a reduced seen set (e.g. after MarkMessagesAsUnread
	// re-derives seenHeads) must yield a smaller frontier, not accumulate onto a
	// stale one. Mirrors the old NewDiffManager rebuild.
	manager.wm = newWatermark(manager.onRemove)
	manager.wm.advance(seenHeads, s.resolvePair, s.eachChangeFor(ctx, manager))

	err = s.getTechSpace().KeyValueService().SubscribeForKey(s.seenHeadsKey(name), name, func(key string, val keyvalueservice.Value) {
		s.ObjectTree.Lock()
		defer s.ObjectTree.Unlock()

		newSeenHeads, err := unmarshalSeenHeads(val.Data)
		if err != nil {
			log.Errorf("subscribe for seenHeads: %s: %v", name, err)
			return
		}
		manager.wm.advance(newSeenHeads, s.resolvePair, s.eachChangeFor(context.Background(), manager))
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
	setter.SetDeferredUpdater(true)

	// Fake state, this kind of objects not support state operations.
	// Object type and layout details are set in the corresponding smartblock Init methods.
	return state.NewDoc(s.id, nil), nil
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
	applier := &storeApply{
		tx:   tx,
		ot:   s.ObjectTree,
		hook: params.ReadStoreTreeHook,
	}
	if err = applier.Apply(ctx); err != nil {
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
		err = tx.ApplyChangeSetReturnAllErrors(storestate.ChangeSet{
			Id:        change.Id,
			Order:     change.OrderId,
			AclHeadId: s.ObjectTree.AclList().Head().Id,
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
	tx.UpdateMaxAddSeq(addResult.Added[0].AddSeq)
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

// addToDiffManagers: no-op. A freshly pushed/synced change has the newest
// AddSeq, so it can never be dominated by an existing seen frontier ⇒ it is
// unread by construction. No graph to maintain (watermark model).
func (s *store) addToDiffManagers(change *objecttree.Change) {}

func (s *store) update(ctx context.Context, tree objecttree.ObjectTree) error {
	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return err
	}
	applier := &storeApply{
		tx: tx,
		ot: tree,
	}
	if err = applier.Apply(ctx); err != nil {
		return errors.Join(tx.Rollback(), err)
	}
	err = tx.Commit()

	s.updateInDiffManagers(ctx)
	if err == nil {
		s.onUpdateHook()
	}
	return err
}

// updateInDiffManagers re-runs advance(nil, ...) for every counter after a tree
// sync. A freshly synced change has the newest AddSeq and can never be dominated
// by an existing seen frontier (so the change itself stays unread by
// construction — see addToDiffManagers), but a previously synced *peer* seenHead
// may have been deferred to w.pending because its change was not yet local. When
// that change finally lands here, re-advancing re-resolves the pending head and
// marks its now-dominated ancestors read. This restores the old
// objecttree.DiffManager.Update re-resolution semantics that the watermark model
// otherwise drops, fixing cross-device unread counts that would otherwise stay
// stale until the next reload / local read / peer update.
func (s *store) updateInDiffManagers(ctx context.Context) {
	for _, manager := range s.diffManagers {
		manager.wm.advance(nil, s.resolvePair, s.eachChangeFor(ctx, manager))
	}
}

func (s *store) MarkSeenHeads(ctx context.Context, name string, heads []string) error {
	manager, ok := s.diffManagers[name]
	if ok {
		manager.wm.advance(heads, s.resolvePair, s.eachChangeFor(ctx, manager))
		return s.StoreSeenHeads(ctx, name)
	}
	return nil
}

func (s *store) StoreSeenHeads(ctx context.Context, name string) error {
	manager, ok := s.diffManagers[name]
	if !ok {
		return nil
	}

	seenHeads := manager.wm.seenHeadIds()
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
