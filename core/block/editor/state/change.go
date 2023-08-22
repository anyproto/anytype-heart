package state

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	"github.com/mb0/diff"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type snapshotOptions struct {
	doNotMigrateTypes bool
	changeId          string
}

type SnapshotOption func(*snapshotOptions)

func DoNotMigrateTypes(o *snapshotOptions) {
	o.doNotMigrateTypes = true
}

func WithChangeId(changeId string) func(*snapshotOptions) {
	return func(o *snapshotOptions) {
		o.changeId = changeId
		return
	}
}

func NewDocFromSnapshot(rootId string, snapshot *pb.ChangeSnapshot, opts ...SnapshotOption) Doc {
	var typesToMigrate []string
	sOpts := snapshotOptions{}
	for _, opt := range opts {
		opt(&sOpts)
	}
	blocks := make(map[string]simple.Block)
	for _, b := range snapshot.Data.Blocks {
		// migrate old dataview blocks with relations
		if dvBlock := b.GetDataview(); dvBlock != nil {
			if len(dvBlock.RelationLinks) == 0 {
				dvBlock.RelationLinks = relationutils.MigrateRelationModels(dvBlock.Relations)
			}
			if !sOpts.doNotMigrateTypes {
				dvBlock.Source, typesToMigrate = relationutils.MigrateObjectTypeIds(dvBlock.Source)
			}
			dvBlock.Source = relationutils.MigrateRelationIds(dvBlock.Source) // can also contain relation ids
		}
		blocks[b.Id] = simple.New(b)
	}
	fileKeys := make([]pb.ChangeFileKeys, 0, len(snapshot.FileKeys))
	for _, fk := range snapshot.FileKeys {
		fileKeys = append(fileKeys, *fk)
	}

	if len(snapshot.Data.RelationLinks) == 0 && len(snapshot.Data.ExtraRelations) > 0 {
		snapshot.Data.RelationLinks = relationutils.MigrateRelationModels(snapshot.Data.ExtraRelations)
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

	s := &State{
		changeId:        sOpts.changeId,
		rootId:          rootId,
		blocks:          blocks,
		details:         detailsToSave,
		relationLinks:   snapshot.Data.RelationLinks,
		objectTypes:     snapshot.Data.ObjectTypes,
		fileKeys:        fileKeys,
		store:           snapshot.Data.Collections,
		storeKeyRemoved: removedCollectionKeysMap,
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

	if !sOpts.doNotMigrateTypes {
		s.objectTypes, s.objectTypesToMigrate = relationutils.MigrateObjectTypeIds(s.objectTypes)
		s.objectTypesToMigrate = append(s.objectTypesToMigrate, typesToMigrate...)
	}
	s.InjectDerivedDetails()
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
	case ch.GetOldRelationAdd() != nil:
		if err = s.changeOldRelationAdd(ch.GetOldRelationAdd()); err != nil {
			return
		}
	case ch.GetOldRelationRemove() != nil:
		if err = s.changeOldRelationRemove(ch.GetOldRelationRemove()); err != nil {
			return
		}
	case ch.GetOldRelationUpdate() != nil:
		if err = s.changeOldRelationUpdate(ch.GetOldRelationUpdate()); err != nil {
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
	shortenDetailsToLimit(s.rootId, map[string]*types.Value{set.Key: set.Value})
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

func (s *State) changeOldRelationAdd(add *pb.Change_RelationAdd) error {
	// MIGRATION: add old relation as new relationLinks
	err := s.changeRelationAdd(&pb.ChangeRelationAdd{RelationLinks: []*model.RelationLink{{Key: add.Relation.Key, Format: add.Relation.Format}}})
	if err != nil {
		return err
	}

	for _, rel := range s.OldExtraRelations() {
		if rel.Key == add.Relation.Key {
			// todo: update?
			log.Warnf("changeOldRelationAdd, relation already exists")
			return nil
		}
	}

	rel := add.Relation
	if rel.Format == model.RelationFormat_file && rel.ObjectTypes == nil {
		rel.ObjectTypes = bundle.FormatFilePossibleTargetObjectTypes
	}

	s.extraRelations = pbtypes.CopyRelations(append(s.OldExtraRelations(), rel))
	return nil
}

func (s *State) changeOldRelationRemove(remove *pb.Change_RelationRemove) error {
	rels := pbtypes.CopyRelations(s.OldExtraRelations())
	for i, rel := range rels {
		if rel.Key == remove.Key {
			s.extraRelations = append(rels[:i], rels[i+1:]...)
			return nil
		}
	}

	err := s.changeRelationRemove(&pb.ChangeRelationRemove{RelationKey: []string{remove.Key}})
	if err != nil {
		return err
	}

	log.Warnf("changeOldRelationRemove: relation to remove not found")
	return nil
}

func (s *State) changeOldRelationUpdate(update *pb.Change_RelationUpdate) error {
	rels := pbtypes.CopyRelations(s.OldExtraRelations())
	for _, rel := range rels {
		if rel.Key != update.Key {
			continue
		}

		switch val := update.Value.(type) {
		case *pb.Change_RelationUpdateValueOfFormat:
			rel.Format = val.Format
		case *pb.Change_RelationUpdateValueOfName:
			rel.Name = val.Name
		case *pb.Change_RelationUpdateValueOfDefaultValue:
			rel.DefaultValue = val.DefaultValue
		case *pb.Change_RelationUpdateValueOfSelectDict:
			rel.SelectDict = val.SelectDict.Dict
		}
		s.extraRelations = rels

		return nil
	}

	return fmt.Errorf("relation not found")
}

func (s *State) changeObjectTypeAdd(add *pb.ChangeObjectTypeAdd) error {
	for _, ot := range s.ObjectTypes() {
		if ot == add.Url {
			return nil
		}
	}
	// in-place migration for bundled object types moved into workspace
	url, migrated := relationutils.MigrateObjectTypeId(add.Url)
	if migrated {
		s.SetObjectTypesToMigrate(append(s.ObjectTypesToMigrate(), url))
		add.Url = url
	}
	objectTypes := append(s.ObjectTypes(), add.Url)
	s.SetObjectTypes(objectTypes)
	// Set only the first(0) object type to the detail
	s.SetLocalDetail(bundle.RelationKeyType.String(), pbtypes.String(s.ObjectType()))

	return nil
}

func (s *State) changeObjectTypeRemove(remove *pb.ChangeObjectTypeRemove) error {
	var found bool
	// in-place migration for bundled object types moved into workspace
	url, migrated := relationutils.MigrateObjectTypeId(remove.Url)
	if migrated {
		// todo: should we also migrate all the object types from the history of object?
		s.objectTypesToMigrate = slice.Filter(s.ObjectTypesToMigrate(), func(s string) bool {
			if s == remove.Url {
				found = true
				return false
			}
			return true
		})
		remove.Url = url
	}

	s.objectTypes = slice.Filter(s.ObjectTypes(), func(s string) bool {
		if s == remove.Url {
			found = true
			return false
		}
		return true
	})
	if !found {
		log.Warnf("changeObjectTypeRemove: type to remove not found: '%s'", remove.Url)
	} else {
		s.SetObjectTypes(s.objectTypes)
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
			var typesToMigrate []string
			dv.Source, typesToMigrate = relationutils.MigrateObjectTypeIds(dv.Source)
			s.objectTypesToMigrate = append(s.objectTypesToMigrate, typesToMigrate...)
			dv.Source = relationutils.MigrateRelationIds(dv.Source) // can also contain relation ids
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
	if s.objectTypes == nil {
		return nil
	}
	var prev []string
	if s.parent != nil {
		prev = s.parent.ObjectTypes()
	}

	var prevMap = make(map[string]struct{}, len(prev))
	var curMap = make(map[string]struct{}, len(s.objectTypes))

	for _, v := range s.objectTypes {
		curMap[v] = struct{}{}
		_, ok := prevMap[v]
		if !ok {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfObjectTypeAdd{
					ObjectTypeAdd: &pb.ChangeObjectTypeAdd{Url: v},
				},
			})
		}
	}
	for _, v := range prev {
		_, ok := curMap[v]
		if !ok {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfObjectTypeRemove{
					ObjectTypeRemove: &pb.ChangeObjectTypeRemove{Url: v},
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
