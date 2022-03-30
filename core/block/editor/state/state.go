package state

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/ipfs/go-cid"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/anytypeio/go-anytype-middleware/util/text"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
)

var log = logging.Logger("anytype-mw-state")

const (
	snippetMinSize = 50
	snippetMaxSize = 300
)

var (
	ErrRestricted = errors.New("restricted")
)

var DetailsFileFields = [...]string{bundle.RelationKeyCoverId.String(), bundle.RelationKeyIconImage.String()}

type Doc interface {
	RootId() string
	NewState() *State
	NewStateCtx(ctx *Context) *State
	Blocks() []*model.Block
	Pick(id string) (b simple.Block)
	Details() *types.Struct
	CombinedDetails() *types.Struct
	LocalDetails() *types.Struct

	ExtraRelations() []*model.Relation

	ObjectTypes() []string
	ObjectType() string

	Iterate(f func(b simple.Block) (isContinue bool)) (err error)
	Snippet() (snippet string)
	GetAndUnsetFileKeys() []pb.ChangeFileKeys
	BlocksInit(ds simple.DetailsService)
	SearchText() string
}

func NewDoc(rootId string, blocks map[string]simple.Block) Doc {
	if blocks == nil {
		blocks = make(map[string]simple.Block)
	}
	s := &State{
		rootId: rootId,
		blocks: blocks,
	}
	s.InjectDerivedDetails()
	return s
}

type State struct {
	ctx                         *Context
	parent                      *State
	blocks                      map[string]simple.Block
	rootId                      string
	newIds                      []string
	changeId                    string
	changes                     []*pb.ChangeContent
	fileKeys                    []pb.ChangeFileKeys
	details                     *types.Struct
	localDetails                *types.Struct
	extraRelations              []*model.Relation
	aggregatedOptionsByRelation map[string][]*model.RelationOption

	store       *types.Struct
	objectTypes []string

	changesStructureIgnoreIds []string

	stringBuf []string

	groupId      string
	noObjectType bool
}

func (s *State) RootId() string {
	if s.rootId == "" {
		for id := range s.blocks {
			var found bool
			for _, b2 := range s.blocks {
				if slice.FindPos(b2.Model().ChildrenIds, id) != -1 {
					found = true
					break
				}
			}
			if !found {
				s.rootId = id
				break
			}
		}
	}
	return s.rootId
}

func (s *State) NewState() *State {
	return &State{parent: s, blocks: make(map[string]simple.Block), rootId: s.rootId, noObjectType: s.noObjectType}
}

func (s *State) NewStateCtx(ctx *Context) *State {
	return &State{parent: s, blocks: make(map[string]simple.Block), rootId: s.rootId, ctx: ctx, noObjectType: s.noObjectType}
}

func (s *State) Context() *Context {
	return s.ctx
}

func (s *State) SetGroupId(groupId string) *State {
	s.groupId = groupId
	return s
}

func (s *State) GroupId() string {
	return s.groupId
}

func (s *State) Add(b simple.Block) (ok bool) {
	id := b.Model().Id
	if s.Pick(id) == nil {
		s.blocks[id] = b
		s.blockInit(b)
		return true
	}
	return false
}

func (s *State) Set(b simple.Block) {
	if !s.Exists(b.Model().Id) {
		s.Add(b)
	} else {
		s.blocks[b.Model().Id] = b
		s.blockInit(b)
	}
}

func (s *State) Get(id string) (b simple.Block) {
	if b = s.blocks[id]; b != nil {
		return
	}
	if s.parent != nil {
		if b = s.Pick(id); b != nil {
			b = b.Copy()
			s.blocks[id] = b
			return
		}
	}
	return
}

func (s *State) Pick(id string) (b simple.Block) {
	var (
		t  = s
		ok bool
	)
	for t != nil {
		if b, ok = t.blocks[id]; ok {
			return
		}
		t = t.parent
	}
	return
}

func (s *State) CleanupBlock(id string) bool {
	var (
		t  = s
		ok bool
	)
	for t != nil {
		if _, ok = t.blocks[id]; ok {
			delete(t.blocks, id)
			return true
		}
		t = t.parent
	}
	return false
}

func (s *State) PickOrigin(id string) (b simple.Block) {
	if s.parent != nil {
		return s.parent.Pick(id)
	}
	return
}

func (s *State) Unlink(id string) (ok bool) {
	if parent := s.GetParentOf(id); parent != nil {
		parentM := parent.Model()
		parentM.ChildrenIds = slice.Remove(parentM.ChildrenIds, id)
		return true
	}
	return
}

func (s *State) GetParentOf(id string) (res simple.Block) {
	if parent := s.PickParentOf(id); parent != nil {
		return s.Get(parent.Model().Id)
	}
	return
}

func (s *State) IsParentOf(parentId string, childId string) bool {
	p := s.Pick(parentId)
	if p == nil {
		return false
	}

	if slice.FindPos(p.Model().ChildrenIds, childId) != -1 {
		return true
	}

	return false
}

func (s *State) HasParent(id, parentId string) bool {
	for {
		parent := s.PickParentOf(id)
		if parent == nil {
			return false
		}
		if parent.Model().Id == parentId {
			return true
		}
		id = parent.Model().Id
	}
}

func (s *State) PickParentOf(id string) (res simple.Block) {
	s.Iterate(func(b simple.Block) bool {
		if slice.FindPos(b.Model().ChildrenIds, id) != -1 {
			res = b
			return false
		}
		return true
	})
	return
}

func (s *State) IsChild(parentId, childId string) bool {
	for {
		parent := s.PickParentOf(childId)
		if parent == nil {
			return false
		}
		if parent.Model().Id == parentId {
			return true
		}
		childId = parent.Model().Id
	}
}

func (s *State) PickOriginParentOf(id string) (res simple.Block) {
	if s.parent != nil {
		return s.parent.PickParentOf(id)
	}
	return
}

func (s *State) getStringBuf() []string {
	if s.parent != nil {
		return s.parent.getStringBuf()
	}

	return s.stringBuf[:0]
}

func (s *State) releaseStringBuf(buf []string) {
	if s.parent != nil {
		s.parent.releaseStringBuf(buf)
		return
	}

	s.stringBuf = buf[:0]
}

func (s *State) Iterate(f func(b simple.Block) (isContinue bool)) (err error) {
	var iter func(id string) (isContinue bool, err error)
	var parentIds = s.getStringBuf()
	defer func() {
		s.releaseStringBuf(parentIds[:0])
	}()

	iter = func(id string) (isContinue bool, err error) {
		if slice.FindPos(parentIds, id) != -1 {
			return false, fmt.Errorf("cycle reference: %v %s", parentIds, id)
		}
		parentIds = append(parentIds, id)
		parentSize := len(parentIds)
		if b := s.Pick(id); b != nil {
			if isContinue = f(b); !isContinue {
				return
			}
			for _, cid := range b.Model().ChildrenIds {
				if isContinue, err = iter(cid); !isContinue || err != nil {
					return
				}
				parentIds = parentIds[:parentSize]
			}
		}
		return true, nil
	}
	_, err = iter(s.RootId())
	return
}

func (s *State) Exists(id string) (ok bool) {
	return s.Pick(id) != nil
}

func (s *State) SearchText() (text string) {
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if tb := b.Model().GetText(); tb != nil {
			text += tb.Text + "\n"
		}
		return true
	})
	return
}

func ApplyState(s *State, withLayouts bool) (msgs []simple.EventMessage, action undo.Action, err error) {
	return s.apply(false, false, withLayouts)
}

func ApplyStateFast(s *State) (msgs []simple.EventMessage, action undo.Action, err error) {
	return s.apply(true, false, false)
}

func ApplyStateFastOne(s *State) (msgs []simple.EventMessage, action undo.Action, err error) {
	return s.apply(true, true, false)
}

func (s *State) apply(fast, one, withLayouts bool) (msgs []simple.EventMessage, action undo.Action, err error) {
	if s.parent != nil && (s.parent.parent != nil || fast) {
		s.intermediateApply()
		if one {
			return
		}
		return s.parent.apply(fast, one, withLayouts)
	}
	if fast {
		return
	}
	st := time.Now()
	if !fast {
		if err = s.normalize(withLayouts); err != nil {
			return
		}
	}
	var (
		inUse          = make(map[string]struct{})
		affectedIds    = make([]string, 0, len(s.blocks))
		newBlocks      []*model.Block
		chmsgs         []simple.EventMessage
		detailsChanged bool
	)

	// apply snippet
	if s.parent != nil {
		if s.Snippet() != s.parent.Snippet() {
			s.SetLocalDetail(bundle.RelationKeySnippet.String(), pbtypes.String(s.Snippet()))
		}
	}

	if s.parent != nil && s.details != nil {
		prev := s.parent.Details()
		detailsChanged = !prev.Equal(s.details)
	}

	if err = s.Iterate(func(b simple.Block) (isContinue bool) {
		id := b.Model().Id
		inUse[id] = struct{}{}
		if _, ok := s.blocks[id]; ok {
			affectedIds = append(affectedIds, id)
		}

		if db, ok := b.(simple.DetailsHandler); ok {
			db = s.Get(id).(simple.DetailsHandler)
			if ok, err := db.ApplyToDetails(s.PickOrigin(id), s); err == nil && ok {
				detailsChanged = true
			}
			if detailsChanged {
				if slice.FindPos(affectedIds, id) == -1 {
					affectedIds = append(affectedIds, id)
				}
			}
		}
		return true
	}); err != nil {
		return
	}
	flushNewBlocks := func() {
		if len(newBlocks) > 0 {
			msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{
				Value: &pb.EventMessageValueOfBlockAdd{
					BlockAdd: &pb.EventBlockAdd{
						Blocks: newBlocks,
					},
				},
			}})
		}
		newBlocks = nil
	}

	// new and changed blocks
	// we need to create events with affectedIds order for correct changes generation
	for _, id := range affectedIds {
		orig := s.PickOrigin(id)
		if orig == nil {
			bc := s.blocks[id].Copy()
			newBlocks = append(newBlocks, bc.Model())
			action.Add = append(action.Add, bc)
		} else {
			flushNewBlocks()
			b := s.Get(id)
			if detailsChanged {
				if db, ok := b.(simple.DetailsHandler); ok {
					db.DetailsInit(s)
				}
			}
			diff, err := orig.Diff(b)
			if err != nil {
				return nil, undo.Action{}, err
			}
			if len(diff) > 0 {
				msgs = append(msgs, diff...)
				if file := orig.Model().GetFile(); file != nil {
					if file.State == model.BlockContentFile_Uploading {
						file.State = model.BlockContentFile_Empty
					}
				}
				action.Change = append(action.Change, undo.Change{
					Before: orig.Copy(),
					After:  b.Copy(),
				})
			}
		}
	}
	flushNewBlocks()
	msgs = append(msgs, chmsgs...)

	// removed blocks
	var (
		toRemove []string
		bm       map[string]simple.Block
	)
	if s.parent != nil {
		bm = s.parent.blocks
	} else {
		bm = s.blocks
	}
	for id := range bm {
		if _, ok := inUse[id]; !ok {
			toRemove = append(toRemove, id)
		}
	}
	if len(toRemove) > 0 {
		msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{
			Value: &pb.EventMessageValueOfBlockDelete{
				BlockDelete: &pb.EventBlockDelete{BlockIds: toRemove},
			},
		}})
	}
	// generate changes
	s.fillChanges(msgs)

	// apply to parent
	for _, id := range toRemove {
		if s.parent != nil {
			action.Remove = append(action.Remove, s.PickOrigin(id).Copy())
			delete(s.parent.blocks, id)
		}
	}
	for _, b := range s.blocks {
		if s.parent != nil {
			id := b.Model().Id
			if _, ok := inUse[id]; ok {
				s.parent.blocks[id] = b
			}
		}
	}
	if s.parent != nil {
		s.parent.changes = s.changes
	}
	if s.parent != nil && s.changeId != "" {
		s.parent.changeId = s.changeId
	}
	if s.parent != nil && s.details != nil {
		prev := s.parent.Details()
		if diff := pbtypes.StructDiff(prev, s.details); diff != nil {
			action.Details = &undo.Details{Before: pbtypes.CopyStruct(prev), After: pbtypes.CopyStruct(s.details)}
			msgs = append(msgs, WrapEventMessages(false, StructDiffIntoEvents(s.RootId(), diff))...)
			s.parent.details = s.details
		} else if !s.details.Equal(s.parent.details) {
			s.parent.details = s.details
		}
	}
	if s.parent != nil && s.extraRelations != nil {
		prev := s.parent.ExtraRelations()

		if added, updated, removed := pbtypes.RelationsDiff(prev, s.extraRelations); (len(added) + len(updated) + len(removed)) > 0 {
			action.Relations = &undo.Relations{Before: pbtypes.CopyRelations(prev), After: pbtypes.CopyRelations(s.extraRelations)}
			s.parent.extraRelations = s.extraRelations
			if len(added)+len(updated) > 0 {
				aggregetedOptions := s.AggregatedOptionsByRelation()
				if aggregetedOptions != nil {
					for _, rel := range append(added, updated...) {
						if m, exists := aggregetedOptions[rel.Key]; exists {
							addedOpts, updatedOpts, _ := pbtypes.RelationSelectDictDiff(pbtypes.GetRelation(prev, rel.Key).GetSelectDict(), rel.GetSelectDict())
							// so here we need to filter out options that has been actually changed in this apply
							// otherwise option names can be overwritten from the localstore with the old ones
							m = pbtypes.RelationOptionsFilter(m, func(option *model.RelationOption) bool {
								return pbtypes.GetOption(append(updatedOpts), option.Id) == nil && pbtypes.GetOption(append(addedOpts), option.Id) == nil
							})
							rel.SelectDict = pbtypes.MergeOptionsPreserveScope(rel.SelectDict, m)
						}
					}
				}
				msgs = append(msgs, simple.EventMessage{
					Msg: &pb.EventMessage{
						Value: &pb.EventMessageValueOfObjectRelationsAmend{
							ObjectRelationsAmend: &pb.EventObjectRelationsAmend{
								Id:        s.RootId(),
								Relations: append(added, updated...),
							},
						},
					},
				})
			}

			if len(removed) > 0 {
				msgs = append(msgs, simple.EventMessage{
					Msg: &pb.EventMessage{
						Value: &pb.EventMessageValueOfObjectRelationsRemove{
							ObjectRelationsRemove: &pb.EventObjectRelationsRemove{
								Id:   s.RootId(),
								Keys: removed,
							},
						},
					},
				})
			}
		}
	}

	if s.parent != nil && s.objectTypes != nil {
		prev := s.parent.ObjectTypes()
		if !slice.UnsortedEquals(prev, s.objectTypes) {
			action.ObjectTypes = &undo.ObjectType{Before: prev, After: s.ObjectTypes()}
			s.parent.objectTypes = s.objectTypes
		}
	}
	if s.parent != nil && len(s.fileKeys) > 0 {
		s.parent.fileKeys = append(s.parent.fileKeys, s.fileKeys...)
	}

	if len(msgs) == 0 && action.IsEmpty() && s.parent != nil {
		// revert lastModified update if we don't have any actual changes being made
		prevModifiedDate := pbtypes.Get(s.parent.LocalDetails(), bundle.RelationKeyLastModifiedDate.String())
		if s.localDetails != nil {
			if _, isNull := prevModifiedDate.GetKind().(*types.Value_NullValue); prevModifiedDate == nil || isNull {
				log.With("thread", s.rootId).Debugf("failed to revert prev modifed date: prev date is nil")
			} else {
				s.localDetails.Fields[bundle.RelationKeyLastModifiedDate.String()] = prevModifiedDate
			}
		}
		// todo: revert lastModifiedBy?
	}

	if s.parent != nil && s.localDetails != nil {
		prev := s.parent.LocalDetails()
		if diff := pbtypes.StructDiff(prev, s.localDetails); diff != nil {
			msgs = append(msgs, WrapEventMessages(true, StructDiffIntoEvents(s.RootId(), diff))...)
			s.parent.localDetails = s.localDetails
		} else if !s.localDetails.Equal(s.parent.localDetails) {
			s.parent.localDetails = s.localDetails
		}
	}

	if s.parent != nil && s.aggregatedOptionsByRelation != nil {
		// todo: when we will have an external subscription for the aggregatedOptionsByRelation we should send events here for all relations
		s.parent.aggregatedOptionsByRelation = s.aggregatedOptionsByRelation
	}

	if s.parent != nil && s.store != nil {
		s.parent.store = s.store
	}

	log.Infof("middle: state apply: %d affected; %d for remove; %d copied; %d changes; for a %v", len(affectedIds), len(toRemove), len(s.blocks), len(s.changes), time.Since(st))
	return
}

func (s *State) intermediateApply() {
	if s.changeId != "" {
		s.parent.changeId = s.changeId
	}
	for _, b := range s.blocks {
		s.parent.Set(b)
	}
	if s.details != nil {
		s.parent.details = s.details
	}
	if s.localDetails != nil {
		s.parent.localDetails = s.localDetails
	}
	if s.aggregatedOptionsByRelation != nil {
		s.parent.aggregatedOptionsByRelation = s.aggregatedOptionsByRelation
	}
	if s.extraRelations != nil {
		s.parent.extraRelations = s.extraRelations
	}
	if s.objectTypes != nil {
		s.parent.objectTypes = s.objectTypes
	}
	if s.store != nil {
		s.parent.store = s.store
	}
	if len(s.fileKeys) > 0 {
		s.parent.fileKeys = append(s.parent.fileKeys, s.fileKeys...)
	}
	s.parent.changes = append(s.parent.changes, s.changes...)
	return
}

func (s *State) Diff(new *State) (msgs []simple.EventMessage, err error) {
	var (
		newBlocks []*model.Block
		removeIds []string
	)
	new.Iterate(func(nb simple.Block) (isContinue bool) {
		b := s.Pick(nb.Model().Id)
		if b == nil {
			newBlocks = append(newBlocks, nb.Copy().Model())
		} else {
			bdiff, e := b.Diff(nb)
			if e != nil {
				err = e
				return false
			}
			msgs = append(msgs, bdiff...)
		}
		return true
	})
	if err != nil {
		return
	}
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if !new.Exists(b.Model().Id) {
			removeIds = append(removeIds, b.Model().Id)
		}
		return true
	})
	if len(newBlocks) > 0 {
		msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{
			Value: &pb.EventMessageValueOfBlockAdd{
				BlockAdd: &pb.EventBlockAdd{
					Blocks: newBlocks,
				},
			},
		}})
	}
	if len(removeIds) > 0 {
		msgs = append(msgs, simple.EventMessage{Msg: &pb.EventMessage{
			Value: &pb.EventMessageValueOfBlockDelete{
				BlockDelete: &pb.EventBlockDelete{
					BlockIds: removeIds,
				},
			},
		}})
	}
	return
}

func (s *State) Blocks() []*model.Block {
	var (
		ids    = []string{s.RootId()}
		blocks = make([]*model.Block, 0, len(s.blocks))
	)

	for len(ids) > 0 {
		next := ids[0]
		ids = ids[1:]

		if b := s.Pick(next); b != nil {
			blocks = append(blocks, b.Copy().Model())
			ids = append(ids, b.Model().ChildrenIds...)
		}
	}

	return blocks
}

func (s *State) BlocksToSave() []*model.Block {
	var (
		ids    = []string{s.RootId()}
		blocks = make([]*model.Block, 0, len(s.blocks))
	)

	for len(ids) > 0 {
		next := ids[0]
		ids = ids[1:]

		if b := s.Pick(next); b != nil {
			blocks = append(blocks, b.Copy().ModelToSave())
			ids = append(ids, b.Model().ChildrenIds...)
		}
	}
	return blocks
}

func (s *State) String() (res string) {
	buf := bytes.NewBuffer(nil)
	s.writeString(buf, 0, s.RootId())
	return buf.String()
}

func (s *State) writeString(buf *bytes.Buffer, l int, id string) {
	b := s.Pick(id)
	buf.WriteString(strings.Repeat("\t", l))
	if b == nil {
		buf.WriteString(id)
		buf.WriteString(" MISSING")
	} else {
		buf.WriteString(b.String())
	}
	buf.WriteString("\n")
	if b != nil {
		for _, cid := range b.Model().ChildrenIds {
			s.writeString(buf, l+1, cid)
		}
	}
}

func (s *State) StringDebug() string {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintf(buf, "RootId: %s\n", s.RootId())
	fmt.Fprintf(buf, "ObjectTypes: %v\n", s.ObjectTypes())
	fmt.Fprintf(buf, "Relations:\n")
	for _, rel := range s.ExtraRelations() {
		fmt.Fprintf(buf, "\t%v\n", rel.Key)
	}

	fmt.Fprintf(buf, "\nDetails:\n")
	pbtypes.SortedRange(s.Details(), func(k string, v *types.Value) {
		fmt.Fprintf(buf, "\t%s:\t%v\n", k, pbtypes.Sprint(v))
	})
	fmt.Fprintf(buf, "\nLocal details:\n")
	pbtypes.SortedRange(s.LocalDetails(), func(k string, v *types.Value) {
		fmt.Fprintf(buf, "\t%s:\t%v\n", k, pbtypes.Sprint(v))
	})
	fmt.Fprintf(buf, "\nBlocks:\n")
	s.writeString(buf, 0, s.RootId())
	fmt.Fprintf(buf, "\nCollection:\n")
	pbtypes.SortedRange(s.Store(), func(k string, v *types.Value) {
		fmt.Fprintf(buf, "\t%s\n", k)
		if st := v.GetStructValue(); st != nil {
			pbtypes.SortedRange(st, func(k string, v *types.Value) {
				fmt.Fprintf(buf, "\t\t%s:\t%v\n", k, pbtypes.Sprint(v))
			})
		}
	})
	return buf.String()
}

func (s *State) SetDetails(d *types.Struct) *State {
	local := pbtypes.StructFilterKeys(d, append(bundle.DerivedRelationsKeys, bundle.LocalRelationsKeys...))
	if local != nil && local.GetFields() != nil && len(local.GetFields()) > 0 {
		for k, v := range local.Fields {
			s.SetLocalDetail(k, v)
		}
		s.details = pbtypes.StructCutKeys(d, append(bundle.DerivedRelationsKeys, bundle.LocalRelationsKeys...))
		return s
	}
	s.details = d
	return s
}

// SetDetailAndBundledRelation sets the detail value and bundled relation in case it is missing
func (s *State) SetDetailAndBundledRelation(key bundle.RelationKey, value *types.Value) {
	s.SetDetail(key.String(), value)
	// AddRelation adds only in case of missing relation
	s.AddRelation(bundle.MustGetRelation(key))

	return
}

func (s *State) SetLocalDetail(key string, value *types.Value) {
	if s.localDetails == nil && s.parent != nil {
		s.localDetails = pbtypes.CopyStruct(s.parent.LocalDetails())
	}
	if s.localDetails == nil || s.localDetails.Fields == nil {
		s.localDetails = &types.Struct{Fields: map[string]*types.Value{}}
	}

	if value == nil {
		delete(s.localDetails.Fields, key)
		return
	}

	s.localDetails.Fields[key] = value
	return
}

func (s *State) SetLocalDetails(d *types.Struct) {
	for k, v := range d.GetFields() {
		if v == nil {
			delete(d.Fields, k)
		}
	}
	s.localDetails = d
}

func (s *State) SetDetail(key string, value *types.Value) {
	if slice.FindPos(bundle.LocalRelationsKeys, key) > -1 || slice.FindPos(bundle.DerivedRelationsKeys, key) > -1 {
		s.SetLocalDetail(key, value)
		return
	}

	if s.details == nil && s.parent != nil {
		s.details = pbtypes.CopyStruct(s.parent.Details())
	}
	if s.details == nil || s.details.Fields == nil {
		s.details = &types.Struct{Fields: map[string]*types.Value{}}
	}

	if value == nil {
		delete(s.details.Fields, key)
		return
	}
	s.details.Fields[key] = value
	return
}

func (s *State) SetExtraRelation(rel *model.Relation) {
	if s.extraRelations == nil && s.parent != nil {
		s.extraRelations = pbtypes.CopyRelations(s.parent.ExtraRelations())
	}
	relCopy := pbtypes.CopyRelation(rel)
	relCopy.Scope = model.Relation_object
	s.normalizeRelationOptionsValues(relCopy)
	var found bool
	for i, exRel := range s.extraRelations {
		if exRel.Key == rel.Key {
			found = true
			s.extraRelations[i] = relCopy
		}
	}
	if !found {
		s.extraRelations = append(s.extraRelations, relCopy)
	}
}

// AddRelation adds new extraRelation to the state.
// In case the one is already exists with the same key it does nothing
func (s *State) AddRelation(relation *model.Relation) *State {
	for _, rel := range s.ExtraRelations() {
		if rel.Key == relation.Key {
			return s
		}
	}

	relCopy := pbtypes.CopyRelation(relation)
	// reset the scope to object
	relCopy.Scope = model.Relation_object
	if !pbtypes.RelationFormatCanHaveListValue(relCopy.Format) && relCopy.MaxCount != 1 {
		relCopy.MaxCount = 1
	}

	if relCopy.Format == model.RelationFormat_file && relCopy.ObjectTypes == nil {
		relCopy.ObjectTypes = bundle.FormatFilePossibleTargetObjectTypes
	}

	s.normalizeRelationOptionsValues(relCopy)
	s.extraRelations = append(pbtypes.CopyRelations(s.ExtraRelations()), relCopy)
	return s
}

func (s *State) SetExtraRelations(relations []*model.Relation) *State {
	relationsCopy := pbtypes.CopyRelations(relations)
	for _, rel := range relationsCopy {
		// reset scopes for all relations
		rel.Scope = model.Relation_object
		if !pbtypes.RelationFormatCanHaveListValue(rel.Format) && rel.MaxCount != 1 {
			rel.MaxCount = 1
		}
		s.normalizeRelationOptionsValues(rel)
	}
	s.extraRelations = relationsCopy
	return s
}

func (s *State) SetAggregatedRelationsOptions(relationKey string, options []*model.RelationOption) *State {
	s.aggregatedOptionsByRelation = s.AggregatedOptionsByRelation()
	if s.aggregatedOptionsByRelation == nil {
		s.aggregatedOptionsByRelation = make(map[string][]*model.RelationOption)
	}

	s.aggregatedOptionsByRelation[relationKey] = pbtypes.CopyRelationOptions(options)
	return s
}

// normalizeRelationOptionsValues may modify relation provided by pointer and set the Detail on the state
// - removes details values that not exists in the selectDict for the relation
// - migrate non-local scope options in case we have this optionId in the corresponding detail
func (s *State) normalizeRelationOptionsValues(rel *model.Relation) (changed bool) {
	if rel.Format != model.RelationFormat_tag && rel.Format != model.RelationFormat_status {
		return
	}
	vals := pbtypes.GetStringList(s.Details(), rel.Key)
	if len(vals) == 0 {
		return
	}
	var filtered = make([]string, 0, len(vals))
	var found bool
	for _, val := range pbtypes.GetStringList(s.Details(), rel.Key) {
		found = false
		for _, v := range rel.SelectDict {
			if v.Id == val {
				found = true
				break
			}
		}
		if found {
			filtered = append(filtered, val)
		}
	}
	if len(filtered) < len(vals) {
		changed = true
		s.SetDetail(rel.Key, pbtypes.StringList(filtered))
	}

	if len(rel.SelectDict) == 0 {
		return
	}
	var optionsMigrated bool
	var dict = make([]*model.RelationOption, 0, len(rel.SelectDict))
	var optExists = make(map[string]struct{}, len(rel.SelectDict))
	for i, opt := range rel.SelectDict {
		if opt.Scope != model.RelationOption_local {
			optionsMigrated = true
			if slice.FindPos(pbtypes.GetStringList(s.Details(), rel.Key), opt.Id) != -1 {
				log.Warnf("obj %s rel %s opt %s migrate scope to local", s.RootId(), rel.Key, opt.Id)
				rel.SelectDict[i].Scope = model.RelationOption_local
				if _, exists := optExists[opt.Id]; !exists {
					rel.SelectDict[i].Scope = model.RelationOption_local
					dict = append(dict, rel.SelectDict[i])
					optExists[opt.Id] = struct{}{}
				}
			} else {
				log.Warnf("obj %s rel %s opt %s remove cause wrong scope and no detail", s.rootId, rel.Key, opt.Id)
			}
		} else {
			if _, exists := optExists[opt.Id]; exists {
				optionsMigrated = true
				continue
			}

			optExists[opt.Id] = struct{}{}
			dict = append(dict, opt)
		}
	}
	if optionsMigrated {
		changed = true
		rel.SelectDict = dict
	}

	return
}

func (s *State) AddExtraRelationOption(rel model.Relation, option model.RelationOption) (*model.RelationOption, error) {
	exRel := pbtypes.GetRelation(s.ExtraRelations(), rel.Key)
	if exRel == nil {
		rel.SelectDict = nil
		s.AddRelation(&rel)
		exRel = &rel
	}
	exRel = pbtypes.CopyRelation(exRel)

	if exRel.Format != model.RelationFormat_status && exRel.Format != model.RelationFormat_tag {
		return nil, fmt.Errorf("relation has incorrect format")
	}

	for _, opt := range exRel.SelectDict {
		if strings.EqualFold(opt.Text, option.Text) && (option.Id == "" || opt.Id == option.Id) {
			if opt.Scope != option.Scope {
				opt.Scope = option.Scope
				s.SetExtraRelation(exRel)
			}
			// here we can have the option with another color, but we can ignore this
			return opt, nil
		}
	}
	if option.Id == "" {
		option.Id = bson.NewObjectId().Hex()
	}
	exRel.SelectDict = append(exRel.SelectDict, &option)
	s.SetExtraRelation(exRel)

	return &option, nil
}

func (s *State) SetObjectType(objectType string) *State {
	return s.SetObjectTypes([]string{objectType})
}

func (s *State) SetObjectTypes(objectTypes []string) *State {
	s.objectTypes = objectTypes
	s.SetDetailAndBundledRelation(bundle.RelationKeyType, pbtypes.String(s.ObjectType()))
	return s
}

func (s *State) InjectDerivedDetails() {
	if objTypes := s.ObjectTypes(); len(objTypes) > 0 && objTypes[0] == bundle.TypeKeySet.URL() {
		if b := s.Get("dataview"); b != nil {
			source := b.Model().GetDataview().GetSource()
			s.SetLocalDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList(source))
		} else {
			s.SetLocalDetail(bundle.RelationKeySetOf.String(), pbtypes.StringList([]string{}))
		}
	}
	s.SetDetailAndBundledRelation(bundle.RelationKeyId, pbtypes.String(s.RootId()))
	if ot := s.ObjectType(); ot != "" {
		s.SetDetailAndBundledRelation(bundle.RelationKeyType, pbtypes.String(ot))
	}
	s.SetDetailAndBundledRelation(bundle.RelationKeySnippet, pbtypes.String(s.Snippet()))
}

func (s *State) InjectLocalDetails(localDetails *types.Struct) {
	for key, v := range localDetails.GetFields() {
		if v == nil {
			continue
		}
		if _, isNull := v.Kind.(*types.Value_NullValue); isNull {
			continue
		}
		s.SetLocalDetail(key, v)
		if !pbtypes.HasRelation(s.ExtraRelations(), key) {
			s.SetExtraRelation(bundle.MustGetRelation(bundle.RelationKey(key)))
		}
	}
}

func (s *State) LocalDetails() *types.Struct {
	if s.localDetails == nil && s.parent != nil {
		return s.parent.LocalDetails()
	}

	return s.localDetails
}

func (s *State) AggregatedOptionsByRelation() map[string][]*model.RelationOption {
	if s.aggregatedOptionsByRelation == nil && s.parent != nil {
		return s.parent.AggregatedOptionsByRelation()
	}

	return s.aggregatedOptionsByRelation
}

func (s *State) CombinedDetails() *types.Struct {
	return pbtypes.StructMerge(s.Details(), s.LocalDetails(), false)
}

func (s *State) HasCombinedDetailsKey(key string) bool {
	if pbtypes.HasField(s.Details(), key) {
		return true
	}
	if pbtypes.HasField(s.LocalDetails(), key) {
		return true
	}
	return false
}

func (s *State) Details() *types.Struct {
	if s.details == nil && s.parent != nil {
		return s.parent.Details()
	}
	return s.details
}

func (s *State) ExtraRelations() []*model.Relation {
	if s.extraRelations == nil && s.parent != nil {
		return s.parent.ExtraRelations()
	}
	return s.extraRelations
}

func (s *State) ObjectTypes() []string {
	if s.objectTypes == nil && s.parent != nil {
		return s.parent.ObjectTypes()
	}
	return s.objectTypes
}

// ObjectType returns only the first objectType and produce warning in case the state has more than 1 object type
// this method is useful because we have decided that currently objects can have only one object type, while preserving the ability to unlock this later
func (s *State) ObjectType() string {
	objTypes := s.ObjectTypes()
	if len(objTypes) == 0 && !s.noObjectType {
		log.Debugf("obj %s(%s) has %d objectTypes instead of 1", s.RootId(), pbtypes.GetString(s.Details(), bundle.RelationKeyName.String()), len(objTypes))
	}

	if len(objTypes) > 0 {
		return objTypes[0]
	}

	return ""
}

func (s *State) Snippet() (snippet string) {
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if text := b.Model().GetText(); text != nil && text.Style != model.BlockContentText_Title && text.Style != model.BlockContentText_Description {
			nextText := strings.TrimSpace(text.Text)
			if snippet != "" && nextText != "" {
				snippet += "\n"
			}
			snippet += nextText
			if utf8.RuneCountInString(snippet) >= snippetMinSize {
				return false
			}
		}
		return true
	})
	return text.Truncate(snippet, snippetMaxSize)
}

func (s *State) FileRelationKeys() (fileKeys []string) {
	for _, rel := range s.ExtraRelations() {
		// coverId can contains both hash or predefined cover id
		if rel.Format == model.RelationFormat_file || rel.Key == bundle.RelationKeyCoverId.String() {
			if slice.FindPos(fileKeys, rel.Key) == -1 {
				fileKeys = append(fileKeys, rel.Key)
			}
		}
	}
	return
}

func (s *State) GetAllFileHashes(detailsKeys []string) (hashes []string) {
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if fh, ok := b.(simple.FileHashes); ok {
			hashes = fh.FillFileHashes(hashes)
		}
		return true
	})
	det := s.Details()
	if det == nil || det.Fields == nil {
		return
	}

	for _, key := range detailsKeys {
		if key == bundle.RelationKeyCoverId.String() {
			v := pbtypes.GetString(det, key)
			_, err := cid.Decode(v)
			if err != nil {
				// this is an exception cause coverId can contains not a file hash but color
				continue
			}
		}
		if v := pbtypes.GetStringList(det, key); v != nil {
			for _, hash := range v {
				if hash == "" {
					continue
				}
				if slice.FindPos(hashes, hash) == -1 {
					hashes = append(hashes, hash)
				}
			}
		}
	}
	return
}

func (s *State) blockInit(b simple.Block) {
	if db, ok := b.(simple.DetailsHandler); ok {
		db.DetailsInit(s)
	}
}

func (s *State) BlocksInit(st simple.DetailsService) {
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if db, ok := b.(simple.DetailsHandler); ok {
			db.DetailsInit(st)
		}
		return true
	})
}

func (s *State) CheckRestrictions() (err error) {
	if s.parent == nil {
		return
	}
	for id, b := range s.blocks {
		rest := b.Model().Restrictions
		if rest == nil {
			continue
		}
		if rest.Edit {
			if ob := s.parent.Pick(id); ob != nil {
				if msgs, _ := ob.Diff(b); len(msgs) > 0 {
					return ErrRestricted
				}
			}
		}
	}
	return
}

func (s *State) SetParent(parent *State) {
	s.rootId = parent.rootId
	s.parent = parent
}

func (s *State) DepSmartIds() (ids []string) {
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if ls, ok := b.(linkSource); ok {
			ids = ls.FillSmartIds(ids)
		}
		return true
	})
	return
}

func (s *State) ValidateNewDetail(key string, v *types.Value) (err error) {
	rel := pbtypes.GetRelation(s.ExtraRelations(), key)
	if rel == nil {
		return fmt.Errorf("relation for detail not found")
	}

	if err := validateRelationFormat(rel, v); err != nil {
		log.Errorf("relation %s(%s) failed to validate: %s", rel.Key, rel.Format, err.Error())
		return fmt.Errorf("format validation failed: %s", err.Error())
	}

	return nil
}

func (s *State) ValidateRelations() (err error) {
	var details = s.Details()
	if details == nil {
		return nil
	}

	for k, v := range details.Fields {
		rel := pbtypes.GetRelation(s.ExtraRelations(), k)
		if rel == nil {
			return fmt.Errorf("relation for detail %s not found", k)
		}
		if err := validateRelationFormat(rel, v); err != nil {
			return fmt.Errorf("relation %s(%s) failed to validate: %s", rel.Key, rel.Format.String(), err.Error())
		}
	}
	return nil
}

func (s *State) Validate() (err error) {
	var (
		err2        error
		childrenIds = make(map[string]string)
	)

	if err = s.ValidateRelations(); err != nil {
		return fmt.Errorf("failed to validate relations: %s", err.Error())
	}

	if err = s.Iterate(func(b simple.Block) (isContinue bool) {
		for _, cid := range b.Model().ChildrenIds {
			if parentId, ok := childrenIds[cid]; ok {
				err2 = fmt.Errorf("two children with same id: %v; parent1: %s; parent2: %s", cid, parentId, b.Model().Id)
				return false
			}
			childrenIds[cid] = b.Model().Id
			if !s.Exists(cid) {
				err2 = fmt.Errorf("missed block: %s; parent: %s", cid, b.Model().Id)
				return false
			}
		}
		return true
	}); err != nil {
		return
	}

	return err2
}

// IsEmpty returns whether state has any blocks beside template blocks(root, header, title, etc)
func (s *State) IsEmpty() bool {
	if pbtypes.GetString(s.Details(), bundle.RelationKeyName.String()) != "" {
		return false
	}
	var emptyTextFound bool
	if title := s.Pick("title"); title != nil {
		if title.Model().GetText().Text != "" {
			return false
		}
		emptyTextFound = true
	}

	if pbtypes.GetString(s.Details(), bundle.RelationKeyDescription.String()) != "" {
		return false
	}

	if root := s.Pick(s.RootId()); root != nil {
		for _, chId := range root.Model().ChildrenIds {
			if chId == "header" {
				continue
			}
			if child := s.Pick(chId); child != nil && child.Model().GetText() != nil && !emptyTextFound {
				txt := child.Model().GetText()
				if txt.Text == "" && txt.Style == 0 {
					emptyTextFound = true
					continue
				}
			}
			return false
		}
	}

	return true
}

func (s *State) Copy() *State {
	blocks := make(map[string]simple.Block, len(s.blocks))
	s.Iterate(func(b simple.Block) (isContinue bool) {
		blocks[b.Model().Id] = b.Copy()
		return true
	})
	objTypes := make([]string, len(s.ObjectTypes()))
	copy(objTypes, s.ObjectTypes())

	agOptsCopy := make(map[string][]*model.RelationOption, len(s.AggregatedOptionsByRelation()))
	for k, v := range s.AggregatedOptionsByRelation() {
		agOptsCopy[k] = pbtypes.CopyRelationOptions(v)
	}

	copy := &State{
		ctx:                         s.ctx,
		blocks:                      blocks,
		rootId:                      s.rootId,
		details:                     pbtypes.CopyStruct(s.Details()),
		localDetails:                pbtypes.CopyStruct(s.LocalDetails()),
		extraRelations:              pbtypes.CopyRelations(s.ExtraRelations()),
		aggregatedOptionsByRelation: agOptsCopy,
		objectTypes:                 objTypes,
		noObjectType:                s.noObjectType,
		store:                       pbtypes.CopyStruct(s.Store()),
	}
	return copy
}

func (s *State) HasRelation(key string) bool {
	for _, rel := range s.ExtraRelations() {
		if rel.Key == key {
			return true
		}
	}
	return false
}

func (s *State) Len() (l int) {
	s.Iterate(func(b simple.Block) (isContinue bool) {
		l++
		return true
	})
	return
}

func (s *State) SetNoObjectType(noObjectType bool) *State {
	s.noObjectType = noObjectType
	return s
}

func (s *State) SetRootId(newRootId string) {
	if s.rootId == "" {
		s.RootId()
	}
	if s.rootId != newRootId {
		if b := s.Get(s.rootId); b != nil {
			b.Model().Id = newRootId
			s.Add(b)
		}
		s.rootId = newRootId
	}
}

func (s *State) ParentState() *State {
	return s.parent
}

func (s *State) RemoveDetail(keys ...string) (ok bool) {
	det := pbtypes.CopyStruct(s.Details())
	if det != nil && det.Fields != nil {
		for _, key := range keys {
			if _, ex := det.Fields[key]; ex {
				delete(det.Fields, key)
				ok = true
			}
		}
	}
	if ok {
		s.SetDetails(det)
	}
	return s.RemoveLocalDetail(keys...) || ok
}

func (s *State) RemoveLocalDetail(keys ...string) (ok bool) {
	det := pbtypes.CopyStruct(s.LocalDetails())
	if det != nil && det.Fields != nil {
		for _, key := range keys {
			if _, ex := det.Fields[key]; ex {
				delete(det.Fields, key)
				ok = true
			}
		}
	}
	if ok {
		s.SetLocalDetails(det)
	}
	return
}

func (s *State) createOrCopyStoreFromParent() {
	// for simplicity each time we are copying store in their entirety
	// the benefit of this is that you are sure that you will not have store on different levels
	// this may not be very good performance/memory wise, but it is simple, so it can stay for now
	if s.store != nil {
		return
	}
	s.store = pbtypes.CopyStruct(s.Store())
	if s.store == nil {
		s.store = &types.Struct{Fields: map[string]*types.Value{}}
	}
}

func (s *State) SetInStore(path []string, value *types.Value) {
	s.setInStore(path, value)
	if value != nil {
		s.changes = append(s.changes, &pb.ChangeContent{
			Value: &pb.ChangeContentValueOfStoreKeySet{
				StoreKeySet: &pb.ChangeStoreKeySet{Path: path, Value: value},
			},
		})
	} else {
		s.changes = append(s.changes, &pb.ChangeContent{
			Value: &pb.ChangeContentValueOfStoreKeyUnset{
				StoreKeyUnset: &pb.ChangeStoreKeyUnset{Path: path},
			},
		})
	}
}

func (s *State) setInStore(path []string, value *types.Value) {
	if len(path) == 0 {
		return
	}
	// todo: optimize to not copy all collection values, but only the map reusing existing values pointers
	s.createOrCopyStoreFromParent()
	store := s.store
	nested := path[:len(path)-1]
	storeStack := []*types.Struct{store}
	for _, key := range nested {
		if store.Fields == nil {
			store.Fields = map[string]*types.Value{}
		}
		_, ok := store.Fields[key]
		// TODO: refactor this with pbtypes
		if !ok {
			store.Fields[key] = &types.Value{
				Kind: &types.Value_StructValue{
					StructValue: &types.Struct{
						Fields: map[string]*types.Value{},
					},
				},
			}
		}
		_, ok = store.Fields[key].Kind.(*types.Value_StructValue)
		if !ok {
			store.Fields[key] = &types.Value{
				Kind: &types.Value_StructValue{
					StructValue: &types.Struct{
						Fields: map[string]*types.Value{},
					},
				},
			}
		}
		store = store.Fields[key].Kind.(*types.Value_StructValue).StructValue
		storeStack = append(storeStack, store)
	}
	if store.Fields == nil {
		store.Fields = map[string]*types.Value{}
	}
	if value != nil {
		store.Fields[path[len(path)-1]] = value
		return
	}
	delete(store.Fields, path[len(path)-1])
	// cleaning empty structs from collection to avoid empty pb values
	idx := len(path) - 2
	for len(store.Fields) == 0 && idx >= 0 {
		delete(storeStack[idx].Fields, path[idx])
		store = storeStack[idx]
		idx--
	}
}

func (s *State) ContainsInStore(path []string) bool {
	if len(path) == 0 {
		return false
	}
	store := s.Store()
	if store == nil {
		return false
	}
	nested := path[:len(path)-1]
	for _, key := range nested {
		if store.Fields == nil {
			return false
		}
		// TODO: refactor this with pbtypes
		_, ok := store.Fields[key]
		if !ok {
			return false
		}
		_, ok = store.Fields[key].Kind.(*types.Value_StructValue)
		if !ok {
			return false
		}
		store = store.Fields[key].Kind.(*types.Value_StructValue).StructValue
	}
	if store.Fields == nil {
		return false
	}
	return store.Fields[path[len(path)-1]] != nil
}

func (s *State) RemoveFromStore(path []string) bool {
	res := s.removeFromStore(path)
	if res {
		s.changes = append(s.changes, &pb.ChangeContent{
			Value: &pb.ChangeContentValueOfStoreKeyUnset{
				StoreKeyUnset: &pb.ChangeStoreKeyUnset{Path: path},
			},
		})
	}
	return res
}

func (s *State) removeFromStore(path []string) bool {
	if len(path) == 0 {
		return false
	}
	if !s.ContainsInStore(path) {
		return false
	}
	s.setInStore(path, nil)
	return true
}

func (s *State) GetCollection(collectionName string) *types.Struct {
	coll := s.Store()
	if coll == nil {
		return nil
	}
	_, ok := coll.Fields[collectionName]
	if !ok {
		return nil
	}
	_, ok = coll.Fields[collectionName].Kind.(*types.Value_StructValue)
	if !ok {
		return nil
	}
	return coll.Fields[collectionName].Kind.(*types.Value_StructValue).StructValue
}

func (s *State) Store() *types.Struct {
	iterState := s
	for iterState != nil && iterState.store == nil {
		iterState = iterState.parent
	}
	if iterState == nil {
		return nil
	}
	return iterState.store
}

func (s *State) Layout() (model.ObjectTypeLayout, bool) {
	if det := s.Details(); det != nil && det.Fields != nil {
		if _, ok := det.Fields[bundle.RelationKeyLayout.String()]; ok {
			return model.ObjectTypeLayout(pbtypes.GetInt64(det, bundle.RelationKeyLayout.String())), true
		}
	}
	return 0, false
}

func (s *State) SetContext(context *Context) {
	s.ctx = context
}

type linkSource interface {
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
}
