package state

import (
	"bytes"
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	pb2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
	"github.com/mb0/diff"
)

func NewDocFromSnapshot(rootId string, snapshot *pb.ChangeSnapshot) Doc {
	blocks := make(map[string]simple.Block)
	for _, b := range snapshot.Data.Blocks {
		blocks[b.Id] = simple.New(b)
	}
	fileKeys := make([]pb.ChangeFileKeys, 0, len(snapshot.FileKeys))
	for _, fk := range snapshot.FileKeys {
		fileKeys = append(fileKeys, *fk)
	}

	// clear nil values
	pb2.StructDeleteEmptyFields(snapshot.Data.Details)

	s := &State{
		rootId:         rootId,
		blocks:         blocks,
		details:        snapshot.Data.Details,
		extraRelations: snapshot.Data.ExtraRelations,
		objectTypes:    snapshot.Data.ObjectTypes,
		fileKeys:       fileKeys,
	}
	s.InjectDerivedDetails()

	return s
}

func (s *State) SetLastModified(ts int64, accountId string) {
	if ts > 0 {
		s.SetDetailAndBundledRelation(bundle.RelationKeyLastModifiedDate, pbtypes.Int64(ts))
	}
	if profileId, err := threads.ProfileThreadIDFromAccountAddress(accountId); err == nil {
		s.SetDetailAndBundledRelation(bundle.RelationKeyLastModifiedBy, pbtypes.String(profileId.String()))
	}
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

func (s *State) GetFileKeys() (keys []pb.ChangeFileKeys) {
	if s.parent != nil {
		keys = s.parent.GetFileKeys()
	}
	if len(s.fileKeys) > 0 {
		keys = append(keys, s.fileKeys...)
		s.fileKeys = s.fileKeys[:0]
	}
	return
}

func (s *State) ApplyChangeIgnoreErr(changes ...*pb.ChangeContent) {
	for _, ch := range changes {
		if err := s.applyChange(ch); err != nil {
			log.Infof("error while applying changes: %v; ignore", err)
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
	case ch.GetRelationUpdate() != nil:
		if err = s.changeRelationUpdate(ch.GetRelationUpdate()); err != nil {
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
	for _, rel := range s.ExtraRelations() {
		if rel.Key == add.Relation.Key {
			// todo: update?
			log.Warnf("changeRelationAdd, relation already exists")
			return nil
		}
	}

	rel := add.Relation
	if rel.Format == model.RelationFormat_file && rel.ObjectTypes == nil {
		rel.ObjectTypes = bundle.FormatFilePossibleTargetObjectTypes
	}

	s.extraRelations = append(pbtypes.CopyRelations(s.ExtraRelations()), rel)
	return nil
}

func (s *State) changeRelationRemove(remove *pb.ChangeRelationRemove) error {
	rels := pbtypes.CopyRelations(s.ExtraRelations())
	for i, rel := range rels {
		if rel.Key == remove.Key {
			s.extraRelations = append(rels[:i], rels[i+1:]...)
			return nil
		}
	}

	log.Warnf("changeRelationRemove: relation to remove not found")
	return nil
}

func (s *State) changeRelationUpdate(update *pb.ChangeRelationUpdate) error {
	rels := pbtypes.CopyRelations(s.ExtraRelations())
	for _, rel := range rels {
		if rel.Key != update.Key {
			continue
		}

		switch val := update.Value.(type) {
		case *pb.ChangeRelationUpdateValueOfFormat:
			rel.Format = val.Format
		case *pb.ChangeRelationUpdateValueOfName:
			rel.Name = val.Name
		case *pb.ChangeRelationUpdateValueOfDefaultValue:
			rel.DefaultValue = val.DefaultValue
		case *pb.ChangeRelationUpdateValueOfSelectDict:
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
	objectTypes := append(s.ObjectTypes(), add.Url)
	s.SetObjectTypes(objectTypes)
	// Set only the first(0) object type to the detail
	s.SetLocalDetail(bundle.RelationKeyType.String(), pbtypes.String(s.ObjectType()))

	return nil
}

func (s *State) changeObjectTypeRemove(remove *pb.ChangeObjectTypeRemove) error {
	var found bool
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
	var bIds = make([]string, len(bc.Blocks))
	for i, m := range bc.Blocks {
		b := simple.New(m)
		bIds[i] = b.Model().Id
		s.Unlink(bIds[i])
		s.Add(b)
	}
	return s.InsertTo(bc.TargetId, bc.Position, bIds...)
}

func (s *State) changeBlockRemove(remove *pb.ChangeBlockRemove) error {
	for _, id := range remove.Ids {
		s.Unlink(id)
	}
	return nil
}

func (s *State) changeBlockUpdate(update *pb.ChangeBlockUpdate) error {
	for _, ev := range update.Events {
		if err := s.applyEvent(ev); err != nil {
			return err
		}
	}
	return nil
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

func (s *State) GetChanges() []*pb.ChangeContent {
	return s.changes
}

func (s *State) fillChanges(msgs []simple.EventMessage) {
	var updMsgs = make([]*pb.EventMessage, 0, len(msgs))
	var delIds []string
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
					log.With("thread", s.rootId).Errorf("duplicate change: " + pbtypes.Sprint(msg.Msg))
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
		case *pb.EventMessageValueOfBlockDelete:
			delIds = append(delIds, o.BlockDelete.BlockIds...)
		case *pb.EventMessageValueOfBlockAdd:
			for _, b := range o.BlockAdd.Blocks {
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
	if len(updMsgs) > 0 {
		cb.AddChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: updMsgs,
				},
			},
		})
	}
	s.changes = cb.Build()
	s.changes = append(s.changes, s.makeDetailsChanges()...)
	s.changes = append(s.changes, s.makeRelationsChanges()...)
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

func diffRelationsIntoUpdates(prev model.Relation, new model.Relation) ([]*pb.ChangeRelationUpdate, error) {
	var updates []*pb.ChangeRelationUpdate

	if prev.Key != new.Key {
		return nil, fmt.Errorf("key should be the same")
	}

	if prev.Name != new.Name {
		updates = append(updates, &pb.ChangeRelationUpdate{
			Key:   prev.Key,
			Value: &pb.ChangeRelationUpdateValueOfName{Name: new.Name},
		})
	}

	if prev.Format != new.Format {
		updates = append(updates, &pb.ChangeRelationUpdate{
			Key:   prev.Key,
			Value: &pb.ChangeRelationUpdateValueOfFormat{Format: new.Format},
		})
	}

	if !slice.UnsortedEquals(prev.ObjectTypes, new.ObjectTypes) {
		updates = append(updates, &pb.ChangeRelationUpdate{
			Key:   prev.Key,
			Value: &pb.ChangeRelationUpdateValueOfObjectTypes{ObjectTypes: &pb.ChangeRelationUpdateObjectTypes{ObjectTypes: new.ObjectTypes}},
		})
	}

	if !prev.DefaultValue.Equal(new.DefaultValue) {
		updates = append(updates, &pb.ChangeRelationUpdate{
			Key:   prev.Key,
			Value: &pb.ChangeRelationUpdateValueOfDefaultValue{DefaultValue: new.DefaultValue},
		})
	}

	if prev.Multi != new.Multi {
		updates = append(updates, &pb.ChangeRelationUpdate{
			Key:   prev.Key,
			Value: &pb.ChangeRelationUpdateValueOfMulti{Multi: new.Multi},
		})
	}

	if new.Format == model.RelationFormat_tag || new.Format == model.RelationFormat_status {
		newDict := pbtypes.RelationOptionsFilterScope(new.SelectDict, model.RelationOption_local)
		if !pbtypes.RelationSelectDictEqual(pbtypes.RelationOptionsFilterScope(prev.SelectDict, model.RelationOption_local), newDict) {
			// todo: CRDT SelectDict patches
			updates = append(updates, &pb.ChangeRelationUpdate{
				Key:   prev.Key,
				Value: &pb.ChangeRelationUpdateValueOfSelectDict{SelectDict: &pb.ChangeRelationUpdateDict{Dict: newDict}},
			})
		}
	}

	return updates, nil
}

func (s *State) makeRelationsChanges() (ch []*pb.ChangeContent) {
	if s.extraRelations == nil {
		return nil
	}
	var prev []*model.Relation
	if s.parent != nil {
		prev = s.parent.ExtraRelations()
	}

	var prevMap = pbtypes.CopyRelationsToMap(prev)
	var curMap = pbtypes.CopyRelationsToMap(s.extraRelations)

	for _, v := range s.extraRelations {
		var rel *model.Relation
		if v.Format == model.RelationFormat_tag || v.Format == model.RelationFormat_status {
			rel = pbtypes.CopyRelation(v)
			// filter-out non-local scope options which we can have in the state to receive events
			rel.SelectDict = pbtypes.RelationOptionsFilterScope(rel.SelectDict, model.RelationOption_local)
		} else {
			rel = v
		}

		pv, ok := prevMap[v.Key]
		if !ok {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfRelationAdd{
					RelationAdd: &pb.ChangeRelationAdd{Relation: v},
				},
			})
		} else {
			updates, err := diffRelationsIntoUpdates(*pv, *v)
			if err != nil {
				// bad input(not equal keys), return the fatal error
				log.Fatal("diffRelationsIntoUpdates fatal error: %s", err.Error())
			}

			for _, update := range updates {
				ch = append(ch, &pb.ChangeContent{
					Value: &pb.ChangeContentValueOfRelationUpdate{
						RelationUpdate: update,
					},
				})
			}
		}
	}
	for _, v := range prev {
		_, ok := curMap[v.Key]
		if !ok {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfRelationRemove{
					RelationRemove: &pb.ChangeRelationRemove{Key: v.Key},
				},
			})
		}
	}
	return
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
