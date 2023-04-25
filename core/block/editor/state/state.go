package state

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-cid"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	textutil "github.com/anytypeio/go-anytype-middleware/util/text"
)

var log = logging.Logger("anytype-mw-state")

const (
	snippetMinSize                 = 50
	snippetMaxSize                 = 300
	collectionKeysRemovedSeparator = "-"

	HeaderLayoutID           = "header"
	TitleBlockID             = "title"
	DescriptionBlockID       = "description"
	DataviewBlockID          = "dataview"
	DataviewTemplatesBlockID = "templates"
	FeaturedRelationsID      = "featuredRelations"
	SettingsStoreKey         = "settings"
	SettingsAnalyticsId      = "analyticsID"
)

var (
	ErrRestricted = errors.New("restricted")
)

var DetailsFileFields = [...]string{bundle.RelationKeyCoverId.String(), bundle.RelationKeyIconImage.String()}

type Doc interface {
	RootId() string
	NewState() *State
	NewStateCtx(ctx *session.Context) *State
	Blocks() []*model.Block
	Pick(id string) (b simple.Block)
	Details() *types.Struct
	CombinedDetails() *types.Struct
	LocalDetails() *types.Struct

	OldExtraRelations() []*model.Relation
	GetRelationLinks() pbtypes.RelationLinks

	ObjectTypes() []string
	ObjectType() string
	Layout() (model.ObjectTypeLayout, bool)

	Iterate(f func(b simple.Block) (isContinue bool)) (err error)
	Snippet() (snippet string)
	GetAndUnsetFileKeys() []pb.ChangeFileKeys
	BlocksInit(ds simple.DetailsService)
	SearchText() string
	ChangeId() string // last pushed change id
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
	ctx           *session.Context
	parent        *State
	blocks        map[string]simple.Block
	rootId        string
	newIds        []string
	changeId      string
	changes       []*pb.ChangeContent
	fileKeys      []pb.ChangeFileKeys
	details       *types.Struct
	localDetails  *types.Struct
	relationLinks pbtypes.RelationLinks

	migrationVersion uint32

	// deprecated, used for migration
	extraRelations              []*model.Relation
	aggregatedOptionsByRelation map[string][]*model.RelationOption // deprecated, used for migration

	store           *types.Struct
	storeKeyRemoved map[string]struct{}

	objectTypes          []string
	objectTypesToMigrate []string

	changesStructureIgnoreIds []string

	stringBuf []string

	groupId      string
	noObjectType bool
}

func (s *State) MigrationVersion() uint32 {
	return s.migrationVersion
}

func (s *State) SetMigrationVersion(v uint32) {
	s.migrationVersion = v
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
	return &State{parent: s, blocks: make(map[string]simple.Block), rootId: s.rootId, noObjectType: s.noObjectType, migrationVersion: s.migrationVersion}
}

func (s *State) NewStateCtx(ctx *session.Context) *State {
	return &State{parent: s, blocks: make(map[string]simple.Block), rootId: s.rootId, ctx: ctx, noObjectType: s.noObjectType, migrationVersion: s.migrationVersion}
}

func (s *State) Context() *session.Context {
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

func (s *State) IterateActive(f func(b simple.Block) (isContinue bool)) {
	for _, b := range s.blocks {
		if !f(b) {
			return
		}
	}
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

// Exists indicate that block exists in state, including parents
func (s *State) Exists(id string) (ok bool) {
	return s.Pick(id) != nil
}

// InState indicate that block was copied into this state, parents not checking
func (s *State) InState(id string) (ok bool) {
	_, ok = s.blocks[id]
	return
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

	if s.parent != nil && s.relationLinks != nil {
		added, removed := s.relationLinks.Diff(s.parent.relationLinks)

		if len(added)+len(removed) > 0 {
			action.RelationLinks = &undo.RelationLinks{
				Before: s.parent.relationLinks,
				After:  s.relationLinks,
			}
		}

		if len(removed) > 0 {
			msgs = append(msgs, WrapEventMessages(false, []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfObjectRelationsRemove{
						ObjectRelationsRemove: &pb.EventObjectRelationsRemove{
							Id:           s.RootId(),
							RelationKeys: removed,
						},
					},
				},
			})...)
		}
		if len(added) > 0 {
			msgs = append(msgs, WrapEventMessages(false, []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfObjectRelationsAmend{
						ObjectRelationsAmend: &pb.EventObjectRelationsAmend{
							Id:            s.RootId(),
							RelationLinks: added,
						},
					},
				},
			})...)
		}
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
		s.parent.migrationVersion = s.migrationVersion
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

	if s.parent != nil && s.objectTypes != nil {
		prev := s.parent.ObjectTypes()
		if !slice.UnsortedEquals(prev, s.objectTypes) {
			action.ObjectTypes = &undo.ObjectType{Before: prev, After: s.ObjectTypes()}
			s.parent.objectTypes = s.objectTypes
		}
	}

	if s.parent != nil && s.objectTypesToMigrate != nil {
		prev := s.parent.ObjectTypesToMigrate()
		if !slice.UnsortedEquals(prev, s.objectTypesToMigrate) {
			s.parent.objectTypesToMigrate = s.objectTypesToMigrate
		}
	}
	if s.parent != nil && len(s.fileKeys) > 0 {
		s.parent.fileKeys = append(s.parent.fileKeys, s.fileKeys...)
	}

	if s.parent != nil && s.relationLinks != nil {
		s.parent.relationLinks = s.relationLinks
	}

	if s.parent != nil && s.extraRelations != nil {
		s.parent.extraRelations = s.extraRelations
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

	if s.parent != nil && s.storeKeyRemoved != nil {
		s.parent.storeKeyRemoved = s.storeKeyRemoved
	}

	msgs = s.processTrailingDuplicatedEvents(msgs)

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
	if s.relationLinks != nil {
		s.parent.relationLinks = s.relationLinks
	}
	if s.extraRelations != nil {
		s.parent.extraRelations = s.extraRelations
	}
	if s.objectTypes != nil {
		s.parent.objectTypes = s.objectTypes
	}
	if s.objectTypesToMigrate != nil {
		s.parent.objectTypesToMigrate = s.objectTypesToMigrate
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

func (s *State) processTrailingDuplicatedEvents(msgs []simple.EventMessage) (filtered []simple.EventMessage) {
	var prev []byte
	filtered = msgs[:0]
	for _, e := range msgs {
		curr, err := e.Msg.Marshal()
		if err != nil {
			continue
		}
		if bytes.Equal(prev, curr) {
			log.With("thread", s.RootId()).Debugf("found trailing duplicated event %s", e.Msg.String())
			continue
		}
		prev = curr
		filtered = append(filtered, e)
	}
	return filtered
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
	for _, rel := range s.relationLinks {
		fmt.Fprintf(buf, "\t%v\n", rel)
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
	s.AddBundledRelations(key)
	s.SetDetail(key.String(), value)
	return
}

func (s *State) SetLocalDetail(key string, value *types.Value) {
	if s.localDetails == nil && s.parent != nil {
		d := s.parent.Details()
		if d.GetFields() != nil {
			// optimisation so we don't need to copy the struct if nothing has changed
			if prev, exists := d.Fields[key]; exists && prev.Equal(value) {
				return
			}
		}
		s.localDetails = pbtypes.CopyStruct(s.parent.LocalDetails())
	}
	if s.localDetails == nil || s.localDetails.Fields == nil {
		s.localDetails = &types.Struct{Fields: map[string]*types.Value{}}
	}

	if value == nil {
		delete(s.localDetails.Fields, key)
		return
	}

	if err := pbtypes.ValidateValue(value); err != nil {
		log.Errorf("invalid value for pb %s: %v", key, err)
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
		d := s.parent.Details()
		if d.GetFields() != nil {
			// optimisation so we don't need to copy the struct if nothing has changed
			if prev, exists := d.Fields[key]; exists && prev.Equal(value) {
				return
			}
		}
		s.details = pbtypes.CopyStruct(d)
	}
	if s.details == nil || s.details.Fields == nil {
		s.details = &types.Struct{Fields: map[string]*types.Value{}}
	}

	if value == nil {
		delete(s.details.Fields, key)
		return
	}

	if err := pbtypes.ValidateValue(value); err != nil {
		log.Errorf("invalid value for pb %s: %v", key, err)
	}

	s.details.Fields[key] = value
	return
}

func (s *State) SetAlign(align model.BlockAlign, ids ...string) (err error) {
	if len(ids) == 0 {
		s.SetDetail(bundle.RelationKeyLayoutAlign.String(), pbtypes.Int64(int64(align)))
		ids = []string{TitleBlockID, DescriptionBlockID, FeaturedRelationsID}
	}
	for _, id := range ids {
		if b := s.Get(id); b != nil {
			b.Model().Align = align
		}
	}
	return
}

func (s *State) SetObjectType(objectType string) *State {
	return s.SetObjectTypes([]string{objectType})
}

func (s *State) SetObjectTypes(objectTypes []string) *State {
	s.objectTypes = objectTypes
	// todo: we lost the second type here, so it becomes inconsistent with the objectTypes in the state
	s.SetDetailAndBundledRelation(bundle.RelationKeyType, pbtypes.String(s.ObjectType()))
	return s
}

func (s *State) SetObjectTypesToMigrate(objectTypes []string) *State {
	s.objectTypesToMigrate = objectTypes
	return s
}

func (s *State) InjectDerivedDetails() {
	id := s.RootId()
	if id != "" {
		s.SetDetailAndBundledRelation(bundle.RelationKeyId, pbtypes.String(id))
	}
	if ot := s.ObjectType(); ot != "" {
		s.SetDetailAndBundledRelation(bundle.RelationKeyType, pbtypes.String(ot))
	}

	snippet := s.Snippet()
	if snippet != "" || s.LocalDetails() != nil {
		s.SetDetailAndBundledRelation(bundle.RelationKeySnippet, pbtypes.String(snippet))
	}
}

func ListSmartblockTypes(objectId string) ([]int, error) {
	if strings.HasPrefix(objectId, addr.BundledObjectTypeURLPrefix) {
		var err error
		objectType, err := bundle.GetTypeByUrl(objectId)
		if err != nil {
			if err == bundle.ErrNotFound {
				return nil, fmt.Errorf("unknown object type")
			}
			return nil, err
		}
		res := make([]int, 0, len(objectType.Types))
		for _, t := range objectType.Types {
			res = append(res, int(t))
		}
		return res, nil
	} else if strings.HasPrefix(objectId, addr.ObjectTypeKeyToIdPrefix) && !strings.HasPrefix(objectId, "b") {
		return nil, fmt.Errorf("incorrect object type URL format")
	}

	// Default smartblock type for all custom object types
	return []int{int(model.SmartBlockType_Page)}, nil
}

func (s *State) InjectLocalDetails(localDetails *types.Struct) {
	for key, v := range localDetails.GetFields() {
		if v == nil {
			continue
		}
		if _, isNull := v.Kind.(*types.Value_NullValue); isNull {
			continue
		}
		s.SetDetailAndBundledRelation(bundle.RelationKey(key), v)
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

func (s *State) OldExtraRelations() []*model.Relation {
	if s.extraRelations == nil && s.parent != nil {
		return s.parent.OldExtraRelations()
	}
	return s.extraRelations
}

func (s *State) ObjectTypes() []string {
	if s.objectTypes == nil && s.parent != nil {
		return s.parent.ObjectTypes()
	}
	return s.objectTypes
}

func (s *State) ObjectTypesToMigrate() []string {
	if s.objectTypes == nil && s.parent != nil {
		return s.parent.ObjectTypesToMigrate()
	}
	return s.objectTypesToMigrate
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
			if textutil.UTF16RuneCountString(snippet) >= snippetMinSize {
				return false
			}
		}
		return true
	})
	return textutil.Truncate(snippet, snippetMaxSize)
}

func (s *State) FileRelationKeys() []string {
	var keys []string
	for _, rel := range s.GetRelationLinks() {
		// coverId can contain both hash or predefined cover id
		if rel.Format == model.RelationFormat_file || rel.Key == bundle.RelationKeyCoverId.String() {
			if slice.FindPos(keys, rel.Key) == -1 {
				keys = append(keys, rel.Key)
			}
		}
	}
	return keys
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
		// get the restrictions from the parent state
		bParent := s.parent.Get(id)
		if bParent == nil {
			// if we don't have this block in the parent state, it means we have no block-scope restrictions for it
			continue
		}
		rest := bParent.Model().Restrictions
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

func (s *State) DepSmartIds(blocks, details, relations, objTypes, creatorModifierWorkspace bool) (ids []string) {
	if blocks {
		err := s.Iterate(func(b simple.Block) (isContinue bool) {
			if ls, ok := b.(linkSource); ok {
				ids = ls.FillSmartIds(ids)
			}
			return true
		})
		if err != nil {
			log.With("thread", s.RootId()).Errorf("failed to iterate over simple blocks: %s", err)
		}
	}

	if objTypes {
		for _, ot := range s.ObjectTypes() {
			if ot == "" {
				log.Errorf("sb %s has empty ot", s.RootId())
				continue
			}
			ids = append(ids, ot)
		}
	}

	var det *types.Struct
	if details {
		det = s.CombinedDetails()
	}

	for _, rel := range s.GetRelationLinks() {
		// do not index local dates such as lastOpened/lastModified
		if relations {
			ids = append(ids, addr.RelationKeyToIdPrefix+rel.Key)
		}

		if !details {
			continue
		}

		// handle corner cases first for specific formats
		if rel.Format == model.RelationFormat_date &&
			!slices.Contains(bundle.LocalRelationsKeys, rel.Key) &&
			!slices.Contains(bundle.DerivedRelationsKeys, rel.Key) {
			relInt := pbtypes.GetInt64(det, rel.Key)
			if relInt > 0 {
				t := time.Unix(relInt, 0)
				t = t.In(time.UTC)
				ids = append(ids, addr.TimeToID(t))
			}
			continue
		}

		if rel.Key == bundle.RelationKeyCreator.String() ||
			rel.Key == bundle.RelationKeyLastModifiedBy.String() ||
			rel.Key == bundle.RelationKeyWorkspaceId.String() {
			if creatorModifierWorkspace {
				v := pbtypes.GetString(det, rel.Key)
				ids = append(ids, v)
			}
			continue
		}

		if rel.Key == bundle.RelationKeyId.String() ||
			rel.Key == bundle.RelationKeyType.String() || // always skip type because it was proceed above
			rel.Key == bundle.RelationKeyFeaturedRelations.String() {
			continue
		}

		if rel.Key == bundle.RelationKeyCoverId.String() {
			v := pbtypes.GetString(det, rel.Key)
			_, err := cid.Decode(v)
			if err != nil {
				// this is an exception cause coverId can contains not a file hash but color
				continue
			}
			ids = append(ids, v)
		}

		if rel.Format != model.RelationFormat_object &&
			rel.Format != model.RelationFormat_file &&
			rel.Format != model.RelationFormat_status &&
			rel.Format != model.RelationFormat_tag {
			continue
		}

		// add all object relation values as dependents
		for _, targetID := range pbtypes.GetStringList(det, rel.Key) {
			if targetID == "" {
				continue
			}

			ids = append(ids, targetID)
		}
	}

	ids = lo.Uniq(ids)
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
func (s *State) IsEmpty(checkTitle bool) bool {
	if checkTitle && pbtypes.GetString(s.Details(), bundle.RelationKeyName.String()) != "" {
		return false
	}
	var emptyTextFound bool

	if title := s.Pick(TitleBlockID); title != nil {
		if checkTitle {
			if title.Model().GetText().Text != "" {
				return false
			}
		}
		emptyTextFound = true
	}

	if pbtypes.GetString(s.Details(), bundle.RelationKeyDescription.String()) != "" {
		return false
	}

	if root := s.Pick(s.RootId()); root != nil {
		for _, chId := range root.Model().ChildrenIds {
			if chId == HeaderLayoutID ||
				chId == FeaturedRelationsID ||
				chId == DataviewBlockID ||
				chId == DataviewTemplatesBlockID {
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

	objTypesToMigrate := make([]string, len(s.ObjectTypesToMigrate()))
	copy(objTypesToMigrate, s.ObjectTypesToMigrate())

	storeKeyRemoved := s.StoreKeysRemoved()
	storeKeyRemovedCopy := make(map[string]struct{}, len(storeKeyRemoved))
	for i := range storeKeyRemoved {
		storeKeyRemovedCopy[i] = struct{}{}
	}
	copy := &State{
		ctx:                  s.ctx,
		blocks:               blocks,
		rootId:               s.rootId,
		details:              pbtypes.CopyStruct(s.Details()),
		localDetails:         pbtypes.CopyStruct(s.LocalDetails()),
		relationLinks:        s.GetRelationLinks(), // Get methods copy inside
		extraRelations:       pbtypes.CopyRelations(s.OldExtraRelations()),
		objectTypes:          objTypes,
		objectTypesToMigrate: objTypesToMigrate,
		noObjectType:         s.noObjectType,
		migrationVersion:     s.migrationVersion,
		store:                pbtypes.CopyStruct(s.Store()),
		storeKeyRemoved:      storeKeyRemovedCopy,
	}
	return copy
}

func (s *State) HasRelation(key string) bool {
	for _, rel := range s.relationLinks {
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

// IsTheHeaderChange return true if the state is the initial header change
// header change is the empty change without any blocks or details except protocol data
func (s *State) IsTheHeaderChange() bool {
	return s.changeId == s.rootId || s.changeId == "" && s.parent == nil
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
	// copy map[string]struct{} to map[string]struct{}
	m := s.StoreKeysRemoved()
	s.storeKeyRemoved = make(map[string]struct{}, len(m))
	for k := range m {
		s.storeKeyRemoved[k] = struct{}{}
	}

	if s.store == nil {
		s.store = &types.Struct{Fields: map[string]*types.Value{}}
	}
	s.storeKeyRemoved = make(map[string]struct{})
}

func (s *State) SetInStore(path []string, value *types.Value) (changed bool) {
	changed = s.setInStore(path, value)
	if !changed {
		return
	}
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
	return
}

func (s *State) UpdateStoreSlice(key string, val []string) {
	old := s.GetStoreSlice(key)
	s.setInStore([]string{key}, pbtypes.StringList(val))

	diff := slice.Diff(old, val, slice.StringIdentity[string], slice.Equal[string])
	changes := slice.UnwrapChanges(diff,
		func(afterID string, items []string) *pb.ChangeStoreSliceUpdate {
			return &pb.ChangeStoreSliceUpdate{
				Operation: &pb.ChangeStoreSliceUpdateOperationOfAdd{
					Add: &pb.ChangeStoreSliceUpdateAdd{
						AfterId: afterID,
						Ids:     items,
					},
				},
			}
		}, func(items []string) *pb.ChangeStoreSliceUpdate {
			return &pb.ChangeStoreSliceUpdate{
				Operation: &pb.ChangeStoreSliceUpdateOperationOfRemove{
					Remove: &pb.ChangeStoreSliceUpdateRemove{
						Ids: items,
					},
				},
			}
		}, func(afterID string, items []string) *pb.ChangeStoreSliceUpdate {
			return &pb.ChangeStoreSliceUpdate{
				Operation: &pb.ChangeStoreSliceUpdateOperationOfMove{
					Move: &pb.ChangeStoreSliceUpdateMove{
						AfterId: afterID,
						Ids:     items,
					},
				},
			}
		}, nil)

	for _, ch := range changes {
		ch.Key = key
		s.changes = append(s.changes, &pb.ChangeContent{
			Value: &pb.ChangeContentValueOfStoreSliceUpdate{StoreSliceUpdate: ch},
		})
	}
}

func (s *State) HasInStore(path []string) bool {
	store := s.Store()
	if store.GetFields() == nil {
		return false
	}

	for _, key := range path {
		_, ok := store.Fields[key]
		if !ok {
			return false
		}
		store = store.Fields[key].Kind.(*types.Value_StructValue).StructValue
	}
	return true
}

func (s *State) setInStore(path []string, value *types.Value) (changed bool) {
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
		oldval := store.Fields[path[len(path)-1]]
		changed = oldval.Compare(value) != 0
		store.Fields[path[len(path)-1]] = value
		// in case we have previously removed this key
		delete(s.storeKeyRemoved, strings.Join(path, collectionKeysRemovedSeparator))
		return
	}
	changed = true
	delete(store.Fields, path[len(path)-1])
	// store all keys that were removed, so we explicitly know this and can make an additional handling
	s.storeKeyRemoved[strings.Join(path, collectionKeysRemovedSeparator)] = struct{}{}
	// cleaning empty structs from collection to avoid empty pb values
	idx := len(path) - 2
	for len(store.Fields) == 0 && idx >= 0 {
		delete(storeStack[idx].Fields, path[idx])
		store = storeStack[idx]
		idx--
	}
	return
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

// GetSubObjectCollection returns the sub object collection, right now only used for account object
func (s *State) GetSubObjectCollection(collectionName string) *types.Struct {
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

// GetStoreSlice returns the list of items in the collection, used for objects with type collection
func (s *State) GetStoreSlice(collectionName string) []string {
	coll := s.Store()
	if coll == nil {
		return nil
	}
	v, ok := coll.Fields[collectionName]
	if !ok {
		return nil
	}
	_, ok = coll.Fields[collectionName].Kind.(*types.Value_ListValue)
	if !ok {
		return nil
	}
	return pbtypes.GetStringListValue(v)
}

func (s *State) GetSetting(name string) *types.Value {
	// get setting from the store
	coll := s.Store()
	if coll == nil {
		return nil
	}
	v, ok := coll.Fields[SettingsStoreKey]
	if !ok {
		return nil
	}
	vs, ok := v.Kind.(*types.Value_StructValue)
	if !ok {
		return nil
	}
	vv := vs.StructValue.GetFields()
	if vv == nil {
		return nil
	}
	return vv[name]
}

func (s *State) SetSetting(name string, val *types.Value) {
	// get setting from the store
	s.SetInStore([]string{SettingsStoreKey, name}, val)
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

func (s *State) StoreKeysRemoved() map[string]struct{} {
	iterState := s
	for iterState != nil && iterState.storeKeyRemoved == nil {
		iterState = iterState.parent
	}
	if iterState == nil {
		return nil
	}
	return iterState.storeKeyRemoved
}

func (s *State) GetChangedStoreKeys(prefixPath ...string) (paths [][]string) {
	if s.store == nil {
		return nil
	}
	pbtypes.StructIterate(s.store, func(path []string, v *types.Value) {
		if slice.HasPrefix(path, prefixPath) || prefixPath == nil {
			if s.parent == nil {
				paths = append(paths, path)
				return
			}
			parentVal := pbtypes.Get(s.parent.store, path...)
			if st := v.GetStructValue(); st != nil && parentVal.GetStructValue() != nil {
				if !pbtypes.StructEqualKeys(st, parentVal.GetStructValue()) {
					paths = append(paths, path)
				}
			} else if !v.Equal(pbtypes.Get(s.parent.store, path...)) {
				paths = append(paths, path)
			}
		}
	})
	return
}

func (s *State) Layout() (model.ObjectTypeLayout, bool) {
	if det := s.Details(); det != nil && det.Fields != nil {
		if _, ok := det.Fields[bundle.RelationKeyLayout.String()]; ok {
			return model.ObjectTypeLayout(pbtypes.GetInt64(det, bundle.RelationKeyLayout.String())), true
		}
	}
	return 0, false
}

func (s *State) SetContext(context *session.Context) {
	s.ctx = context
}

func (s *State) AddRelationLinks(links ...*model.RelationLink) {
	relLinks := s.GetRelationLinks()
	for _, l := range links {
		if !relLinks.Has(l.Key) {
			relLinks = append(relLinks, l)
		}
	}
	s.relationLinks = relLinks
}

func (s *State) PickRelationLinks() pbtypes.RelationLinks {
	if s.relationLinks != nil {
		return s.relationLinks
	}
	if s.parent != nil {
		return s.parent.PickRelationLinks()
	}
	return nil
}

func (s *State) GetRelationLinks() pbtypes.RelationLinks {
	if s.relationLinks != nil {
		return s.relationLinks
	}
	if s.parent != nil {
		parentLinks := s.parent.PickRelationLinks()
		s.relationLinks = parentLinks.Copy()
		return s.relationLinks
	}
	return nil
}

func (s *State) RemoveRelation(keys ...string) {
	relLinks := s.GetRelationLinks()
	relLinksFiltered := make(pbtypes.RelationLinks, 0, len(relLinks))
	for _, link := range relLinks {
		if slice.FindPos(keys, link.Key) >= 0 {
			continue
		}
		relLinksFiltered = append(relLinksFiltered, &model.RelationLink{
			Key:    link.Key,
			Format: link.Format,
		})
	}
	// remove detail value
	s.RemoveDetail(keys...)
	// remove from the list of featured relations
	var foundInFeatured bool
	featuredList := pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String())
	featuredList = slice.Filter(featuredList, func(s string) bool {
		if slice.FindPos(keys, s) == -1 {
			return true
		}
		foundInFeatured = true
		return false
	})
	if foundInFeatured {
		s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(featuredList))
	}
	s.relationLinks = relLinksFiltered
	return
}

func (s *State) Descendants(rootId string) []simple.Block {
	var (
		queue    = []string{rootId}
		children []simple.Block
	)

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		cur := s.Pick(id)
		if cur == nil {
			continue
		}
		for _, id := range cur.Model().ChildrenIds {
			b := s.Pick(id)
			if b == nil {
				continue
			}
			children = append(children, b)
			queue = append(queue, id)
		}
	}

	return children
}

// SelectRoots returns unique root blocks that are listed in ids AND present in the state
// "root" here means the block that hasn't any parents listed in input ids
func (s *State) SelectRoots(ids []string) []string {
	resCount := len(ids)
	discarded := make([]bool, len(ids))
	for i := 0; i < len(ids); i++ {

		if discarded[i] {
			continue
		}
		ai := ids[i]
		if !s.Exists(ai) {
			discarded[i] = true
			resCount--
		}
		for j := 0; j < len(ids); j++ {
			if i == j {
				continue
			}
			if discarded[j] {
				continue
			}

			aj := ids[j]
			if s.IsChild(ai, aj) {
				discarded[j] = true
				resCount--
			}
		}
	}

	res := make([]string, 0, resCount)
	for i, id := range ids {
		if !discarded[i] {
			res = append(res, id)
		}
	}
	return res
}

func (s *State) AddBundledRelations(keys ...bundle.RelationKey) {
	links := make([]*model.RelationLink, 0, len(keys))
	for _, key := range keys {
		rel := bundle.MustGetRelation(key)
		links = append(links, &model.RelationLink{Format: rel.Format, Key: rel.Key})
	}
	s.AddRelationLinks(links...)
}

type linkSource interface {
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
}
