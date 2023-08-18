package state

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	"github.com/mb0/diff"
)

type snapshotOptions struct {
	changeId          string
	migrateUniqueKeys model.SmartBlockType // set >0 to migrate unique keys
}

type SnapshotOption func(*snapshotOptions)

func WithChangeId(changeId string) func(*snapshotOptions) {
	return func(o *snapshotOptions) {
		o.changeId = changeId
		return
	}
}

func WithUniqueKeyMigration(sbType model.SmartBlockType) func(*snapshotOptions) {
	return func(o *snapshotOptions) {
		o.migrateUniqueKeys = sbType
		return
	}
}

func NewDocFromSnapshot(rootId string, snapshot *pb.ChangeSnapshot, opts ...SnapshotOption) Doc {
	sOpts := snapshotOptions{}
	for _, opt := range opts {
		opt(&sOpts)
	}
	blocks := make(map[string]simple.Block)
	for _, b := range snapshot.Data.Blocks {
		blocks[b.Id] = simple.New(b)
	}
	fileKeys := make([]pb.ChangeFileKeys, 0, len(snapshot.FileKeys))
	for _, fk := range snapshot.FileKeys {
		fileKeys = append(fileKeys, *fk)
	}

	// clear nil values
	pbtypes.StructDeleteEmptyFields(snapshot.Data.Details)

	removedCollectionKeysMap := make(map[string]struct{}, len(snapshot.Data.RemovedCollectionKeys))
	for _, t := range snapshot.Data.RemovedCollectionKeys {
		removedCollectionKeysMap[t] = struct{}{}
	}

	detailsToSave := pbtypes.StructCutKeys(snapshot.Data.Details,
		append(bundle.DerivedRelationsKeys, bundle.LocalRelationsKeys...))

	if err := pbtypes.ValidateStruct(detailsToSave); err != nil {
		log.Errorf("NewDocFromSnapshot details validation error: %v; details normalized", err)
		pbtypes.NormalizeStruct(detailsToSave)
	}

	if sOpts.migrateUniqueKeys > 0 {
		migrateAddMissingUniqueKey(sOpts.migrateUniqueKeys, snapshot)
	}

	s := &State{
		changeId:          sOpts.changeId,
		rootId:            rootId,
		blocks:            blocks,
		details:           detailsToSave,
		relationLinks:     snapshot.Data.RelationLinks,
		objectTypeKeys:    migrateObjectTypeIDsToKeys(snapshot.Data.ObjectTypes),
		fileKeys:          fileKeys,
		store:             snapshot.Data.Collections,
		storeKeyRemoved:   removedCollectionKeysMap,
		uniqueKeyInternal: snapshot.Data.Key,
	}
	if s.store != nil {
		for collName, coll := range s.store.Fields {
			if c := coll.GetStructValue(); s != nil {
				for k := range c.GetFields() {
					s.setStoreChangeId(collName+addr.SubObjectCollectionIdSeparator+k, s.changeId)
				}
			}
		}
	}

	return s
}

func (s *State) SetLastModified(ts int64, profileId string) {
	if ts > 0 {
		s.SetDetailAndBundledRelation(bundle.RelationKeyLastModifiedDate, pbtypes.Int64(ts))
	}
	s.SetDetailAndBundledRelation(bundle.RelationKeyLastModifiedBy, pbtypes.String(profileId))
}

func (s *State) SetChangeId(id string) {
	s.changeId = id
}

func (s *State) ChangeId() string {
	return s.changeId
}

func (s *State) Merge(s2 *State) *State {
	// TODO:
	return s
}

func (s *State) ApplyChange(changes ...*pb.ChangeContent) (err error) {
	for _, ch := range changes {
		if err = s.applyChange(ch); err != nil {
			return
		}
	}
	return
}

func (s *State) AddFileKeys(keys ...*pb.ChangeFileKeys) {
	for _, k := range keys {
		if k != nil {
			s.fileKeys = append(s.fileKeys, *k)
		}
	}
}

// GetAndUnsetFileKeys returns file keys from the current set and unset them, so they will no longer pop up
func (s *State) GetAndUnsetFileKeys() (keys []pb.ChangeFileKeys) {
	if s.parent != nil {
		keys = s.parent.GetAndUnsetFileKeys()
	}
	if len(s.fileKeys) > 0 {
		keys = append(keys, s.fileKeys...)
		s.fileKeys = s.fileKeys[:0]
	}
	return
}

// ApplyChangeIgnoreErr should be called with changes from the single pb.Change
func (s *State) ApplyChangeIgnoreErr(changes ...*pb.ChangeContent) {
	for _, ch := range changes {
		if err := s.applyChange(ch); err != nil {
			log.With("objectID", s.RootId()).Warnf("error while applying change %T: %v; ignore", ch.Value, err)
		}
	}
	return
}

func (s *State) applyChange(ch *pb.ChangeContent) (err error) {
	switch {
	case ch.GetBlockCreate() != nil:
		if err = s.changeBlockCreate(ch.GetBlockCreate()); err != nil {
			return
		}
	case ch.GetBlockRemove() != nil:
		if err = s.changeBlockRemove(ch.GetBlockRemove()); err != nil {
			return
		}
	case ch.GetBlockUpdate() != nil:
		if err = s.changeBlockUpdate(ch.GetBlockUpdate()); err != nil {
			return
		}
	case ch.GetBlockMove() != nil:
		if err = s.changeBlockMove(ch.GetBlockMove()); err != nil {
			return
		}
	case ch.GetDetailsSet() != nil:
		if err = s.changeBlockDetailsSet(ch.GetDetailsSet()); err != nil {
			return
		}
	case ch.GetDetailsUnset() != nil:
		if err = s.changeBlockDetailsUnset(ch.GetDetailsUnset()); err != nil {
			return
		}
	case ch.GetRelationAdd() != nil:
		if err = s.changeRelationAdd(ch.GetRelationAdd()); err != nil {
			return
		}
	case ch.GetRelationRemove() != nil:
		if err = s.changeRelationRemove(ch.GetRelationRemove()); err != nil {
			return
		}
	case ch.GetObjectTypeAdd() != nil:
		if err = s.changeObjectTypeAdd(ch.GetObjectTypeAdd()); err != nil {
			return
		}
	case ch.GetObjectTypeRemove() != nil:
		if err = s.changeObjectTypeRemove(ch.GetObjectTypeRemove()); err != nil {
			return
		}
	case ch.GetStoreKeySet() != nil:
		if err = s.changeStoreKeySet(ch.GetStoreKeySet()); err != nil {
			return
		}
		s.changes = append(s.changes, ch)
	case ch.GetStoreKeyUnset() != nil:
		if err = s.changeStoreKeyUnset(ch.GetStoreKeyUnset()); err != nil {
			return
		}
		s.changes = append(s.changes, ch)
	case ch.GetStoreSliceUpdate() != nil:
		// TODO optimize: collect changes then apply them on one shot
		if err = s.changeStoreSliceUpdate(ch.GetStoreSliceUpdate()); err != nil {
			return
		}
	default:
		return fmt.Errorf("unexpected changes content type: %v", ch)
	}
	return
}

func (s *State) changeBlockDetailsSet(set *pb.ChangeDetailsSet) error {
	det := s.Details()
	if det == nil || det.Fields == nil {
		det = &types.Struct{
			Fields: make(map[string]*types.Value),
		}
	}
	s.details = pbtypes.CopyStruct(det)
	if set.Value != nil {
		s.details.Fields[set.Key] = set.Value
	} else {
		delete(s.details.Fields, set.Key)
	}
	return nil
}

func (s *State) changeBlockDetailsUnset(unset *pb.ChangeDetailsUnset) error {
	det := s.Details()
	if det == nil || det.Fields == nil {
		det = &types.Struct{
			Fields: make(map[string]*types.Value),
		}
	}
	s.details = pbtypes.CopyStruct(det)
	delete(s.details.Fields, unset.Key)
	return nil
}

func (s *State) changeRelationAdd(add *pb.ChangeRelationAdd) error {
	rl := s.GetRelationLinks()
	for _, r := range add.RelationLinks {
		if !rl.Has(r.Key) {
			rl = rl.Append(r)
		}
	}
	s.relationLinks = rl
	return nil
}

func (s *State) changeRelationRemove(rem *pb.ChangeRelationRemove) error {
	s.RemoveRelation(rem.RelationKey...)
	return nil
}

func (s *State) changeObjectTypeAdd(add *pb.ChangeObjectTypeAdd) error {
	if add.Url != "" {
		// migration of the old type changes
		// before we were storing the change ID instead of Key
		// but it's pretty easy to convert it
		add.Key = strings.TrimPrefix(add.Url, addr.ObjectTypeKeyToIdPrefix)
	}

	for _, ot := range s.ObjectTypeKeys() {
		if ot == bundle.TypeKey(add.Key) {
			return nil
		}
	}
	objectTypes := append(s.ObjectTypeKeys(), bundle.TypeKey(add.Key))
	s.SetObjectTypeKeys(objectTypes)
	return nil
}

func (s *State) changeObjectTypeRemove(remove *pb.ChangeObjectTypeRemove) error {
	var found bool
	if remove.Url != "" {
		remove.Key = strings.TrimPrefix(remove.Url, addr.ObjectTypeKeyToIdPrefix)
	}
	s.objectTypeKeys = slice.Filter(s.ObjectTypeKeys(), func(key bundle.TypeKey) bool {
		if key == bundle.TypeKey(remove.Key) {
			found = true
			return false
		}
		return true
	})
	if !found {
		log.Warnf("changeObjectTypeRemove: type to remove not found: '%s'", remove.Url)
	} else {
		s.SetObjectTypeKeys(s.objectTypeKeys)
	}
	return nil
}

func (s *State) changeBlockCreate(bc *pb.ChangeBlockCreate) (err error) {
	var bIds = make([]string, 0, len(bc.Blocks))
	for _, m := range bc.Blocks {
		if m.Id == s.rootId {
			if b := s.Pick(m.Id); b != nil {
				continue
			}
		}
		b := simple.New(m)
		if m.Id != s.rootId {
			bIds = append(bIds, b.Model().Id)
			s.Unlink(m.Id)
		}
		s.Set(b)
		if dv := b.Model().GetDataview(); dv != nil {
			if len(dv.RelationLinks) == 0 {
				dv.RelationLinks = relationutils.MigrateRelationModels(dv.Relations)
			}
		}
	}
	return s.InsertTo(bc.TargetId, bc.Position, bIds...)
}

func (s *State) changeBlockRemove(remove *pb.ChangeBlockRemove) error {
	for _, id := range remove.Ids {
		s.Unlink(id)
		s.CleanupBlock(id)
	}
	return nil
}

func (s *State) changeBlockUpdate(update *pb.ChangeBlockUpdate) error {
	merr := multierror.Error{}
	for _, ev := range update.Events {
		if err := s.applyEvent(ev); err != nil {
			merr.Errors = append(merr.Errors, fmt.Errorf("failed to apply event %T: %s", ev.Value, err.Error()))
		}
	}
	return merr.ErrorOrNil()
}

func (s *State) changeBlockMove(move *pb.ChangeBlockMove) error {
	ns := s.NewState()
	for _, id := range move.Ids {
		ns.Unlink(id)
	}
	if err := ns.InsertTo(move.TargetId, move.Position, move.Ids...); err != nil {
		return err
	}
	_, _, err := ApplyStateFastOne(ns)
	return err
}

func (s *State) changeStoreKeySet(set *pb.ChangeStoreKeySet) error {
	s.setInStore(set.Path, set.Value)
	return nil
}

func (s *State) changeStoreKeyUnset(unset *pb.ChangeStoreKeyUnset) error {
	s.removeFromStore(unset.Path)
	return nil
}

func (s *State) changeStoreSliceUpdate(upd *pb.ChangeStoreSliceUpdate) error {
	var changes []slice.Change[string]
	if v := upd.GetAdd(); v != nil {
		changes = append(changes, slice.MakeChangeAdd(v.Ids, v.AfterId))
	} else if v := upd.GetRemove(); v != nil {
		changes = append(changes, slice.MakeChangeRemove[string](v.Ids))
	} else if v := upd.GetMove(); v != nil {
		changes = append(changes, slice.MakeChangeMove[string](v.Ids, v.AfterId))
	}

	store := s.Store()
	old := pbtypes.GetStringList(store, upd.Key)
	cur := slice.ApplyChanges(old, changes, slice.StringIdentity[string])
	s.setInStore([]string{upd.Key}, pbtypes.StringList(cur))
	return nil
}

func (s *State) GetChanges() []*pb.ChangeContent {
	return s.changes
}

func (s *State) fillChanges(msgs []simple.EventMessage) {
	var updMsgs = make([]*pb.EventMessage, 0, len(msgs))
	var delIds, delRelIds []string
	var newRelLinks pbtypes.RelationLinks
	var structMsgs = make([]*pb.EventBlockSetChildrenIds, 0, len(msgs))
	var b1, b2 []byte
	for i, msg := range msgs {
		if msg.Virtual {
			continue
		}
		if i > 0 {
			if msg.Msg.Size() == msgs[i-1].Msg.Size() {
				b1, _ = msg.Msg.Marshal()
				b2, _ = msgs[i-1].Msg.Marshal()
				if bytes.Equal(b1, b2) {
					log.With("objectID", s.rootId).Errorf("duplicate change: " + pbtypes.Sprint(msg.Msg))
				}
			}
		}
		switch o := msg.Msg.Value.(type) {
		case *pb.EventMessageValueOfBlockSetChildrenIds:
			structMsgs = append(structMsgs, o.BlockSetChildrenIds)
		case *pb.EventMessageValueOfBlockSetAlign:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetBackgroundColor:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetBookmark:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetVerticalAlign:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetDiv:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetText:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetFields:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetFile:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetLink:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetRelation:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetLatex:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetWidget:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockDelete:
			delIds = append(delIds, o.BlockDelete.BlockIds...)
		case *pb.EventMessageValueOfBlockAdd:
			for _, b := range o.BlockAdd.Blocks {
				if b.Id == s.rootId {
					// special case to add root block
					s.changes = append(s.changes, &pb.ChangeContent{
						Value: &pb.ChangeContentValueOfBlockCreate{
							BlockCreate: &pb.ChangeBlockCreate{
								Blocks: []*model.Block{b},
							},
						},
					})
				}
				s.newIds = append(s.newIds, b.Id)
				if len(b.ChildrenIds) > 0 {
					structMsgs = append(structMsgs, &pb.EventBlockSetChildrenIds{
						Id:          b.Id,
						ChildrenIds: b.ChildrenIds,
					})
				}
			}
		case *pb.EventMessageValueOfBlockDataviewSourceSet:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockDataviewViewSet:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockDataviewViewOrder:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockDataviewViewDelete:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockDataviewRelationSet:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockDataviewRelationDelete:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockDataViewGroupOrderUpdate:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfObjectRelationsAmend:
			newRelLinks = append(newRelLinks, msg.Msg.GetObjectRelationsAmend().RelationLinks...)
		case *pb.EventMessageValueOfObjectRelationsRemove:
			delRelIds = append(delRelIds, msg.Msg.GetObjectRelationsRemove().RelationKeys...)
		case *pb.EventMessageValueOfBlockDataViewObjectOrderUpdate:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockDataviewViewUpdate:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockDataviewTargetObjectIdSet:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockDataviewIsCollectionSet:
			updMsgs = append(updMsgs, msg.Msg)
		case *pb.EventMessageValueOfBlockSetRestrictions:
			updMsgs = append(updMsgs, msg.Msg)
		default:
			log.Errorf("unexpected event - can't convert to changes: %v", msg.Msg)
		}
	}
	var cb = &changeBuilder{changes: s.changes}
	if len(structMsgs) > 0 {
		s.fillStructureChanges(cb, structMsgs)
	}
	if len(delIds) > 0 {
		cb.AddChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockRemove{
				BlockRemove: &pb.ChangeBlockRemove{
					Ids: delIds,
				},
			},
		})
	}
	if len(newRelLinks) > 0 {
		cb.AddChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfRelationAdd{
				RelationAdd: &pb.ChangeRelationAdd{
					RelationLinks: newRelLinks,
				},
			},
		})
	}
	if len(delRelIds) > 0 {
		cb.AddChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfRelationRemove{
				RelationRemove: &pb.ChangeRelationRemove{
					RelationKey: delRelIds,
				},
			},
		})
	}
	if len(updMsgs) > 0 {
		cb.AddChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: updMsgs,
				},
			},
		})
	}
	s.collapseSameKeyStoreChanges()
	s.changes = cb.Build()
	s.changes = append(s.changes, s.makeDetailsChanges()...)
	s.changes = append(s.changes, s.makeObjectTypesChanges()...)

}

func (s *State) fillStructureChanges(cb *changeBuilder, msgs []*pb.EventBlockSetChildrenIds) {
	for _, msg := range msgs {
		s.makeStructureChanges(cb, msg)
	}
}

func (s *State) makeStructureChanges(cb *changeBuilder, msg *pb.EventBlockSetChildrenIds) (ch []*pb.ChangeContent) {
	if slice.FindPos(s.changesStructureIgnoreIds, msg.Id) != -1 {
		return
	}
	var before []string
	orig := s.PickOrigin(msg.Id)
	if orig != nil {
		before = orig.Model().ChildrenIds
	}

	ds := &dstrings{a: before, b: msg.ChildrenIds}
	d := diff.Diff(len(ds.a), len(ds.b), ds)
	var (
		targetId  string
		targetPos model.BlockPosition
	)
	var makeTarget = func(pos int) {
		if pos == 0 {
			if len(ds.a) == 0 {
				targetId = msg.Id
				targetPos = model.Block_Inner
			} else {
				targetId = ds.a[0]
				targetPos = model.Block_Top
			}
		} else {
			targetId = ds.b[pos-1]
			targetPos = model.Block_Bottom
		}
	}
	for _, c := range d {
		if c.Ins > 0 {
			prevOp := 0
			for ins := 0; ins < c.Ins; ins++ {
				pos := c.B + ins
				id := ds.b[pos]
				if slice.FindPos(s.newIds, id) != -1 {
					if prevOp != 1 {
						makeTarget(pos)
					}
					cb.Add(targetId, targetPos, s.Pick(id).Copy().Model())
					prevOp = 1
				} else {
					if prevOp != 2 {
						makeTarget(pos)
					}
					cb.Move(targetId, targetPos, id)
					prevOp = 2
				}
			}
		}
	}
	return
}

func (s *State) makeDetailsChanges() (ch []*pb.ChangeContent) {
	if s.details == nil || s.details.Fields == nil {
		return nil
	}
	var prev *types.Struct
	if s.parent != nil {
		prev = s.parent.Details()
	}
	if prev == nil || prev.Fields == nil {
		prev = &types.Struct{Fields: make(map[string]*types.Value)}
	}
	curDetails := s.Details()
	for k, v := range curDetails.Fields {
		pv, ok := prev.Fields[k]
		if !ok || !pv.Equal(v) {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfDetailsSet{
					DetailsSet: &pb.ChangeDetailsSet{Key: k, Value: v},
				},
			})
		}
	}
	for k := range prev.Fields {
		if _, ok := curDetails.Fields[k]; !ok {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfDetailsUnset{
					DetailsUnset: &pb.ChangeDetailsUnset{Key: k},
				},
			})
		}
	}
	return
}

func (s *State) collapseSameKeyStoreChanges() {
	seen := make(map[string]struct{}, len(s.changes))
	var filteredChanges []*pb.ChangeContent
	for i := len(s.changes) - 1; i >= 0; i-- {
		ch := s.changes[i]
		var key []string
		if ch.GetStoreKeySet() != nil {
			key = ch.GetStoreKeySet().Path
		} else if ch.GetStoreKeyUnset() != nil {
			key = ch.GetStoreKeyUnset().Path
		} else {
			filteredChanges = append(filteredChanges, ch)
			continue
		}
		joined := strings.Join(key, "/")
		if _, exists := seen[joined]; exists {
			continue
		}
		seen[joined] = struct{}{}
		filteredChanges = append(filteredChanges, ch)
	}
	l := len(filteredChanges)
	for i := 0; i < l/2; i++ {
		temp := filteredChanges[i]
		filteredChanges[i] = filteredChanges[l-i-1]
		filteredChanges[l-i-1] = temp
	}
	s.changes = filteredChanges
}

func (s *State) makeObjectTypesChanges() (ch []*pb.ChangeContent) {
	if s.objectTypeKeys == nil {
		return nil
	}
	var prev []bundle.TypeKey
	if s.parent != nil {
		prev = s.parent.ObjectTypeKeys()
	}

	var prevMap = make(map[bundle.TypeKey]struct{}, len(prev))
	var curMap = make(map[bundle.TypeKey]struct{}, len(s.objectTypeKeys))

	for _, v := range s.objectTypeKeys {
		curMap[v] = struct{}{}
		_, ok := prevMap[v]
		if !ok {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfObjectTypeAdd{
					ObjectTypeAdd: &pb.ChangeObjectTypeAdd{Url: string(v)},
				},
			})
		}
	}
	for _, v := range prev {
		_, ok := curMap[v]
		if !ok {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfObjectTypeRemove{
					ObjectTypeRemove: &pb.ChangeObjectTypeRemove{Url: string(v)},
				},
			})
		}
	}
	return
}

type dstrings struct{ a, b []string }

func (d *dstrings) Equal(i, j int) bool { return d.a[i] == d.b[j] }

type changeBuilder struct {
	changes []*pb.ChangeContent

	isLastAdd    bool
	lastTargetId string
	lastPosition model.BlockPosition
	lastIds      []string
	lastBlocks   []*model.Block
}

func (cb *changeBuilder) Move(targetId string, pos model.BlockPosition, id string) {
	if cb.isLastAdd || cb.lastTargetId != targetId || cb.lastPosition != pos {
		cb.Flush()
	}
	cb.isLastAdd = false
	cb.lastTargetId = targetId
	cb.lastPosition = pos
	cb.lastIds = append(cb.lastIds, id)
}

func (cb *changeBuilder) Add(targetId string, pos model.BlockPosition, m *model.Block) {
	if !cb.isLastAdd || cb.lastTargetId != targetId || cb.lastPosition != pos {
		cb.Flush()
	}
	m.ChildrenIds = nil
	cb.isLastAdd = true
	cb.lastTargetId = targetId
	cb.lastPosition = pos
	cb.lastBlocks = append(cb.lastBlocks, m)
}

func (cb *changeBuilder) AddChange(ch ...*pb.ChangeContent) {
	cb.Flush()
	cb.changes = append(cb.changes, ch...)
}

func (cb *changeBuilder) Flush() {
	if cb.lastTargetId == "" {
		return
	}
	if cb.isLastAdd && len(cb.lastBlocks) > 0 {
		var create = &pb.ChangeBlockCreate{
			TargetId: cb.lastTargetId,
			Position: cb.lastPosition,
			Blocks:   cb.lastBlocks,
		}
		cb.changes = append(cb.changes, &pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockCreate{
				BlockCreate: create,
			},
		})
	} else if !cb.isLastAdd && len(cb.lastIds) > 0 {
		var move = &pb.ChangeBlockMove{
			TargetId: cb.lastTargetId,
			Position: cb.lastPosition,
			Ids:      cb.lastIds,
		}
		cb.changes = append(cb.changes, &pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockMove{
				BlockMove: move,
			},
		})
	}
	cb.lastTargetId = ""
	cb.lastBlocks = nil
	cb.lastIds = nil
}

func (cb *changeBuilder) Build() []*pb.ChangeContent {
	cb.Flush()
	return cb.changes
}

func migrateObjectTypeIDsToKeys(objectTypeIDs []string) []bundle.TypeKey {
	objectTypeKeys := make([]bundle.TypeKey, 0, len(objectTypeIDs))
	for _, id := range objectTypeIDs {
		var key bundle.TypeKey
		if strings.HasPrefix(id, addr.ObjectTypeKeyToIdPrefix) {
			key = bundle.TypeKey(strings.TrimPrefix(id, addr.ObjectTypeKeyToIdPrefix))
		} else {
			key = bundle.TypeKey(id)
		}
		objectTypeKeys = append(objectTypeKeys, key)
	}
	return objectTypeKeys
}

func migrateAddMissingUniqueKey(sbType model.SmartBlockType, snapshot *pb.ChangeSnapshot) {
	id := pbtypes.GetString(snapshot.Data.Details, bundle.RelationKeyId.String())
	uk, err := uniquekey.UnmarshalFromString(id)
	if err != nil {
		// means that sbtype is not supported
		return
	}
	if uk.SmartblockType() != sbType {
		log.Errorf("missingKeyMigration: wrong sbtype %s != %s", uk.SmartblockType(), sbType)
		return
	}

	snapshot.Data.Key = uk.InternalKey()
}
