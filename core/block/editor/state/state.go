package state

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	relationCol "github.com/anytypeio/go-anytype-middleware/pkg/lib/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/anytypeio/go-anytype-middleware/util/text"
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

var DetailsFileFields = [...]string{"coverId", "iconImage"}

type Doc interface {
	RootId() string
	NewState() *State
	NewStateCtx(ctx *Context) *State
	Blocks() []*model.Block
	Pick(id string) (b simple.Block)
	Details() *types.Struct
	ExtraRelations() []*pbrelation.Relation

	ObjectTypes() []string

	Iterate(f func(b simple.Block) (isContinue bool)) (err error)
	Snippet() (snippet string)
	GetFileKeys() []pb.ChangeFileKeys
	BlocksInit()
}

func NewDoc(rootId string, blocks map[string]simple.Block) Doc {
	if blocks == nil {
		blocks = make(map[string]simple.Block)
	}
	s := &State{
		rootId: rootId,
		blocks: blocks,
	}
	return s
}

type State struct {
	ctx            *Context
	parent         *State
	blocks         map[string]simple.Block
	rootId         string
	newIds         []string
	changeId       string
	changes        []*pb.ChangeContent
	fileKeys       []pb.ChangeFileKeys
	details        *types.Struct
	extraRelations []*pbrelation.Relation
	objectTypes    []string

	changesStructureIgnoreIds []string

	bufIterateParentIds []string
	groupId             string
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
	return &State{parent: s, blocks: make(map[string]simple.Block), rootId: s.rootId}
}

func (s *State) NewStateCtx(ctx *Context) *State {
	return &State{parent: s, blocks: make(map[string]simple.Block), rootId: s.rootId, ctx: ctx}
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
		if s.parent != nil {
			s.newIds = append(s.newIds, id)
		}
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
			s.blockInit(b)
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

func (s *State) Iterate(f func(b simple.Block) (isContinue bool)) (err error) {
	var iter func(id string) (isContinue bool, err error)
	var parentIds = s.bufIterateParentIds[:0]
	iter = func(id string) (isContinue bool, err error) {
		if slice.FindPos(parentIds, id) != -1 {
			return false, fmt.Errorf("cycle reference: %v %s", parentIds, id)
		}
		parentIds = append(parentIds, id)
		parentSize := len(parentIds)
		b := s.Pick(id)
		if b != nil {
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
			if dmsgs, err := db.DetailsApply(s); err == nil && len(dmsgs) > 0 {
				chmsgs = append(chmsgs, dmsgs...)
			} else if detailsChanged {
				if dmsgs, err := db.OnDetailsChange(s); err == nil {
					chmsgs = append(chmsgs, dmsgs...)
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
			b := s.blocks[id]
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
		if !prev.Equal(s.details) {
			action.Details = &undo.Details{Before: pbtypes.CopyStruct(prev), After: pbtypes.CopyStruct(s.details)}
			s.parent.details = s.details
			msgs = append(msgs, simple.EventMessage{
				Msg: &pb.EventMessage{
					Value: &pb.EventMessageValueOfBlockSetDetails{
						BlockSetDetails: &pb.EventBlockSetDetails{
							Id:      s.RootId(),
							Details: pbtypes.CopyStruct(s.details),
						},
					},
				},
			})
		}
	}
	if s.parent != nil && s.extraRelations != nil {
		prev := s.parent.ExtraRelations()

		if !pbtypes.RelationsEqual(prev, s.extraRelations) {
			action.Relations = &undo.Relations{Before: pbtypes.CopyRelations(prev), After: pbtypes.CopyRelations(s.extraRelations)}
			s.parent.extraRelations = s.extraRelations
			msgs = append(msgs, simple.EventMessage{
				Msg: &pb.EventMessage{
					Value: &pb.EventMessageValueOfBlockSetRelations{
						BlockSetRelations: &pb.EventBlockSetRelations{
							Id:        s.RootId(),
							Relations: pbtypes.CopyRelations(s.extraRelations),
						},
					},
				},
			})
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
	if s.extraRelations != nil {
		s.parent.extraRelations = s.extraRelations
	}
	if s.objectTypes != nil {
		s.parent.objectTypes = s.objectTypes
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

func (s *State) SetDetails(d *types.Struct) *State {
	s.details = d
	return s
}

func (s *State) SetDetail(key string, value *types.Value) {
	if s.details == nil {
		s.details = pbtypes.CopyStruct(s.parent.Details())
	}
	if s.details == nil || s.details.Fields == nil {
		s.details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	s.details.Fields[key] = value
	return
}

func (s *State) AddRelation(relation *pbrelation.Relation) *State {
	for _, rel := range s.ExtraRelations() {
		if rel.Key == relation.Key {
			return s
		}
	}
	if relation.Format == pbrelation.RelationFormat_file && relation.ObjectTypes == nil {
		relation.ObjectTypes = relationCol.FormatFilePossibleTargetObjectTypes
	}

	s.extraRelations = append(s.ExtraRelations(), relation)
	return s
}

func (s *State) SetExtraRelations(relations []*pbrelation.Relation) *State {
	s.extraRelations = relations
	return s
}

func (s *State) SetObjectTypes(objectTypes []string) *State {
	s.objectTypes = objectTypes
	return s
}

func (s *State) Details() *types.Struct {
	if s.details == nil && s.parent != nil {
		return s.parent.Details()
	}

	return s.details
}

func (s *State) ExtraRelations() []*pbrelation.Relation {
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

func (s *State) Snippet() (snippet string) {
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if text := b.Model().GetText(); text != nil && text.Style != model.BlockContentText_Title {
			if snippet != "" {
				snippet += " "
			}
			snippet += text.Text
			if utf8.RuneCountInString(snippet) >= snippetMinSize {
				return false
			}
		}
		return true
	})
	return text.Truncate(snippet, snippetMaxSize)
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

	for _, field := range detailsKeys {
		if v := det.Fields[field]; v != nil && v.GetStringValue() != "" {
			hashes = append(hashes, v.GetStringValue())
		}
	}
	return
}

func (s *State) blockInit(b simple.Block) {
	if db, ok := b.(simple.DetailsHandler); ok {
		db.DetailsInit(s)
	}
}

func (s *State) BlocksInit() {
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if db, ok := b.(simple.DetailsHandler); ok {
			db.DetailsInit(s)
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

func (s *State) Validate() (err error) {
	var (
		err2        error
		childrenIds = make(map[string]string)
	)

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
	i := 0
	blocksToTraverse := []string{"header"}
	ignoredTemplateBlocksMap := map[string]struct{}{s.rootId: {}}
	for i < len(blocksToTraverse) {
		id := blocksToTraverse[i]
		i++
		b := s.Pick(id)
		if b == nil {
			continue
		}
		blocksToTraverse = append(blocksToTraverse, b.Model().ChildrenIds...)
		ignoredTemplateBlocksMap[id] = struct{}{}
	}

	if len(s.blocks) <= len(ignoredTemplateBlocksMap) {
		return true
	}

	return false
}

func (s *State) Copy() *State {
	blocks := make(map[string]simple.Block, len(s.blocks))
	for k, v := range s.blocks {
		blocks[k] = v.Copy()
	}
	objTypes := make([]string, len(s.objectTypes))
	copy(objTypes, s.objectTypes)

	copy := &State{
		ctx:            s.ctx,
		blocks:         blocks,
		rootId:         s.rootId,
		details:        pbtypes.CopyStruct(s.details),
		extraRelations: pbtypes.CopyRelations(s.extraRelations),
		objectTypes:    objTypes,
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

type linkSource interface {
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
}
