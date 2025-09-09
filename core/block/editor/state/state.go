package state

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
	textutil "github.com/anyproto/anytype-heart/util/text"
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
	ErrRestricted             = errors.New("restricted")
	ErrSystemBlockDelete      = errors.New("deletion of system block is prohibited")
	ErrInternalRelationDelete = errors.New("deletion of internal relation is prohibited")

	systemBlocks = map[string]struct{}{
		HeaderLayoutID:      {},
		TitleBlockID:        {},
		FeaturedRelationsID: {},
	}
)

type Doc interface {
	RootId() string
	NewState() *State
	NewStateCtx(ctx session.Context) *State
	Blocks() []*model.Block
	Pick(id string) (b simple.Block)
	Details() *domain.Details
	CombinedDetails() *domain.Details
	LocalDetails() *domain.Details

	GetRelationLinks() pbtypes.RelationLinks

	ObjectTypeKeys() []domain.TypeKey
	ObjectTypeKey() domain.TypeKey
	Layout() (model.ObjectTypeLayout, bool)

	Iterate(f func(b simple.Block) (isContinue bool)) (err error)
	Snippet() (snippet string)
	UniqueKeyInternal() string

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
	return s
}

// NewDocWithUniqueKey creates a new state with the given uniqueKey.
// it is used for creating new objects which ID is derived from the uniqueKey(smartblockType+key)
func NewDocWithUniqueKey(rootId string, blocks map[string]simple.Block, key domain.UniqueKey) Doc {
	return NewDocWithInternalKey(rootId, blocks, key.InternalKey())
}

// NewDocWithInternalKey creates a new state with the given internal key.
// prefer creating new objects using NewDocWithUniqueKey instead, for the extra checks during the unique key creation
func NewDocWithInternalKey(rootId string, blocks map[string]simple.Block, internalKey string) Doc {
	if blocks == nil {
		blocks = make(map[string]simple.Block)
	}
	s := &State{
		rootId:            rootId,
		blocks:            blocks,
		uniqueKeyInternal: internalKey,
	}
	return s
}

type State struct {
	ctx    session.Context
	parent *State
	blocks map[string]simple.Block
	rootId string
	// uniqueKeyInternal is used together with smartblock type for the ID derivation
	// which will be unique and reproducible within the same space
	uniqueKeyInternal string
	newIds            []string
	changeId          string
	changes           []*pb.ChangeContent
	fileInfo          FileInfo
	fileKeys          []pb.ChangeFileKeys // Deprecated
	details           *domain.Details
	localDetails      *domain.Details
	relationLinks     pbtypes.RelationLinks
	notifications     map[string]*model.Notification
	deviceStore       map[string]*model.DeviceInfo

	migrationVersion uint32

	store                   *types.Struct
	storeKeyRemoved         map[string]struct{}
	storeLastChangeIdByPath map[string]string // accumulated during the state build, always passing by reference to the new state

	objectTypeKeys []domain.TypeKey // here we store object type keys, not IDs

	changesStructureIgnoreIds []string

	stringBuf []string

	groupId                  string
	noObjectType             bool
	originalCreatedTimestamp int64 // pass here from snapshots when importing objects or used for derived objects such as relations, types and etc
}

type RelationsByLayout map[model.ObjectTypeLayout][]domain.RelationKey

type Filters struct {
	RelationsWhiteList RelationsByLayout
	RemoveBlocks       bool
}

// Filter should be called with state copy
func (s *State) Filter(filters *Filters) *State {
	if filters == nil {
		return s
	}
	if filters.RemoveBlocks {
		s.filterBlocks()
	}
	if len(filters.RelationsWhiteList) > 0 {
		s.filterRelations(filters)
	}
	return s
}

func (s *State) filterBlocks() {
	resultBlocks := make(map[string]simple.Block)
	if block, ok := s.blocks[s.rootId]; ok {
		resultBlocks[s.rootId] = block
	}
	s.blocks = resultBlocks
}

func (s *State) filterRelations(filters *Filters) {
	resultDetails := domain.NewDetails()
	layout, _ := s.Layout()
	relationKeys := filters.RelationsWhiteList[layout]
	var updatedRelationLinks pbtypes.RelationLinks
	for key, value := range s.details.Iterate() {
		if slices.Contains(relationKeys, key) {
			resultDetails.Set(key, value)
			updatedRelationLinks = append(updatedRelationLinks, s.relationLinks.Get(key.String()))
			continue
		}
	}
	s.details = resultDetails
	if resultDetails.Len() == 0 {
		s.details = nil
	}
	resultLocalDetails := domain.NewDetails()
	for key, value := range s.localDetails.Iterate() {
		if slices.Contains(relationKeys, key) {
			resultLocalDetails.Set(key, value)
			updatedRelationLinks = append(updatedRelationLinks, s.relationLinks.Get(key.String()))
			continue
		}
	}
	s.localDetails = resultLocalDetails
	if resultLocalDetails.Len() == 0 {
		s.localDetails = nil
	}
	s.relationLinks = updatedRelationLinks
}

func (s *State) MigrationVersion() uint32 {
	return s.migrationVersion
}

func (s *State) SetMigrationVersion(v uint32) {
	s.migrationVersion = v
}

func (s *State) RootId() string {
	if s.rootId == "" {
		subIds := map[string]struct{}{}
		for _, block := range s.blocks {
			for _, id := range block.Model().ChildrenIds {
				subIds[id] = struct{}{}
			}
		}

		for id := range s.blocks {
			if _, isSub := subIds[id]; !isSub {
				s.rootId = id
			}
		}
	}
	return s.rootId
}

func (s *State) NewState() *State {
	return s.NewStateCtx(nil)
}

func (s *State) NewStateCtx(ctx session.Context) *State {
	return &State{
		parent:                   s,
		blocks:                   make(map[string]simple.Block),
		rootId:                   s.rootId,
		ctx:                      ctx,
		noObjectType:             s.noObjectType,
		migrationVersion:         s.migrationVersion,
		uniqueKeyInternal:        s.uniqueKeyInternal,
		originalCreatedTimestamp: s.originalCreatedTimestamp,
		fileInfo:                 s.fileInfo,
	}
}

func (s *State) Context() session.Context {
	return s.ctx
}

func (s *State) SetGroupId(groupId string) *State {
	s.groupId = groupId
	return s
}

func (s *State) GroupId() string {
	return s.groupId
}

func (s *State) SpaceID() string {
	return s.LocalDetails().GetString(bundle.RelationKeySpaceId)
}

func (s *State) Add(b simple.Block) (ok bool) {
	id := b.Model().Id
	if s.Pick(id) == nil {
		s.blocks[id] = b
		s.blockInit(b)
		s.setChildrenIds(b.Model(), b.Model().ChildrenIds)
		return true
	}
	return false
}

func (s *State) Set(b simple.Block) {
	if !s.Exists(b.Model().Id) {
		s.Add(b)
	} else {
		s.setChildrenIds(b.Model(), b.Model().ChildrenIds)
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

func (s *State) Unlink(blockId string) (ok bool) {
	if parent := s.GetParentOf(blockId); parent != nil {
		s.removeChildren(parent.Model(), blockId)
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

func (s *State) SearchText() string {
	var builder strings.Builder
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if tb := b.Model().GetText(); tb != nil {
			builder.WriteString(tb.Text)
			builder.WriteRune('\n')
		}
		return true
	})
	return builder.String()
}

func ApplyState(spaceId string, s *State, withLayouts bool) (msgs []simple.EventMessage, action undo.Action, err error) {
	return s.apply(spaceId, false, false, withLayouts)
}

func ApplyStateFast(spaceId string, s *State) (msgs []simple.EventMessage, action undo.Action, err error) {
	return s.apply(spaceId, true, false, false)
}

func ApplyStateFastOne(spaceId string, s *State) (msgs []simple.EventMessage, action undo.Action, err error) {
	return s.apply(spaceId, true, true, false)
}

func (s *State) apply(spaceId string, fast, one, withLayouts bool) (msgs []simple.EventMessage, action undo.Action, err error) {
	if s.parent != nil && (s.parent.parent != nil || fast) {
		s.intermediateApply()
		if one {
			return
		}
		return s.parent.apply("", fast, one, withLayouts)
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

	if s.parent != nil {
		s.parent.uniqueKeyInternal = s.uniqueKeyInternal

		// apply snippet
		if s.Snippet() != s.parent.Snippet() {
			s.SetLocalDetail(bundle.RelationKeySnippet, domain.String(s.Snippet()))
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
			msgs = append(msgs, simple.EventMessage{Msg: event.NewMessage(spaceId,
				&pb.EventMessageValueOfBlockAdd{
					BlockAdd: &pb.EventBlockAdd{
						Blocks: newBlocks,
					},
				}),
			})
		}
		newBlocks = nil
	}

	// new and changed blocks
	// we need to create events with affectedIds order for correct changes generation
	for _, id := range affectedIds {
		orig := s.PickOrigin(id)
		if orig == nil {
			bc := s.blocks[id].Copy()
			if db, ok := bc.(simple.DetailsHandler); ok {
				db.DetailsInit(s)
			}
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
			diff, err := orig.Diff(spaceId, b)
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
			if _, isSystem := systemBlocks[id]; isSystem {
				return nil, undo.Action{}, ErrSystemBlockDelete
			}
			toRemove = append(toRemove, id)
		}
	}
	if len(toRemove) > 0 {
		msgs = append(msgs, simple.EventMessage{Msg: event.NewMessage(s.SpaceID(), &pb.EventMessageValueOfBlockDelete{
			BlockDelete: &pb.EventBlockDelete{BlockIds: toRemove},
		})})
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
				event.NewMessage(s.SpaceID(), &pb.EventMessageValueOfObjectRelationsRemove{
					ObjectRelationsRemove: &pb.EventObjectRelationsRemove{
						Id:           s.RootId(),
						RelationKeys: removed,
					},
				},
				),
			})...)
		}
		if len(added) > 0 {
			msgs = append(msgs, WrapEventMessages(false, []*pb.EventMessage{
				event.NewMessage(s.SpaceID(), &pb.EventMessageValueOfObjectRelationsAmend{
					ObjectRelationsAmend: &pb.EventObjectRelationsAmend{
						Id:            s.RootId(),
						RelationLinks: added,
					},
				},
				),
			})...)
		}
	}

	// generate changes
	s.fillChanges(msgs)

	// apply to parent
	if s.parent != nil {
		for _, id := range toRemove {
			action.Remove = append(action.Remove, s.PickOrigin(id).Copy())
			delete(s.parent.blocks, id)
		}
	} else {
		for _, id := range toRemove {
			delete(s.blocks, id)
		}
	}
	if s.parent != nil {
		for _, b := range s.blocks {
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
		if diff, keysToUnset := domain.StructDiff(prev, s.details); diff != nil || len(keysToUnset) != 0 {
			if slices.ContainsFunc(keysToUnset, func(key domain.RelationKey) bool {
				return bundle.IsInternalRelation(key)
			}) {
				return nil, undo.Action{}, ErrInternalRelationDelete
			}
			action.Details = &undo.Details{Before: prev.Copy(), After: s.details.Copy()}
			msgs = append(msgs, WrapEventMessages(false, StructDiffIntoEvents(s.SpaceID(), s.RootId(), diff, keysToUnset))...)
			s.parent.details = s.details
		} else if !s.details.Equal(s.parent.details) {
			s.parent.details = s.details
		}
	}

	if s.parent != nil && s.objectTypeKeys != nil {
		prev := s.parent.ObjectTypeKeys()
		if !slice.UnsortedEqual(prev, s.objectTypeKeys) {
			action.ObjectTypes = &undo.ObjectType{Before: prev, After: s.ObjectTypeKeys()}
			s.parent.objectTypeKeys = s.objectTypeKeys
		}
	}

	if s.parent != nil {
		s.parent.fileInfo = s.fileInfo
	}

	if s.parent != nil && len(s.fileKeys) > 0 {
		s.parent.fileKeys = append(s.parent.fileKeys, s.fileKeys...)
	}

	if s.parent != nil && s.relationLinks != nil {
		s.parent.relationLinks = s.relationLinks
	}

	if s.parent != nil && s.localDetails != nil {
		prev := s.parent.LocalDetails()
		if diff, keysToUnset := domain.StructDiff(prev, s.localDetails); diff != nil || len(keysToUnset) != 0 {
			if slices.ContainsFunc(keysToUnset, func(key domain.RelationKey) bool {
				return bundle.IsInternalRelation(key)
			}) {
				return nil, undo.Action{}, ErrInternalRelationDelete
			}
			msgs = append(msgs, WrapEventMessages(true, StructDiffIntoEvents(spaceId, s.RootId(), diff, keysToUnset))...)
			s.parent.localDetails = s.localDetails
		} else if !s.localDetails.Equal(s.parent.localDetails) {
			s.parent.localDetails = s.localDetails
		}
	}

	if s.parent != nil && s.store != nil {
		s.parent.store = s.store
	}

	if s.parent != nil && s.storeLastChangeIdByPath != nil {
		s.parent.storeLastChangeIdByPath = s.storeLastChangeIdByPath
	}

	if s.parent != nil && s.storeKeyRemoved != nil {
		s.parent.storeKeyRemoved = s.storeKeyRemoved
	}

	if s.parent != nil && s.originalCreatedTimestamp > 0 {
		s.parent.originalCreatedTimestamp = s.originalCreatedTimestamp
	}

	if s.parent != nil && s.notifications != nil {
		s.parent.notifications = s.notifications
	}

	if s.parent != nil && s.deviceStore != nil {
		s.parent.deviceStore = s.deviceStore
	}

	msgs = s.processTrailingDuplicatedEvents(msgs)

	sortEventMessages(msgs)
	log.Debugf("middle: state apply: %d affected; %d for remove; %d copied; %d changes; for a %v", len(affectedIds), len(toRemove), len(s.blocks), len(s.changes), time.Since(st))
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

	if s.relationLinks != nil {
		s.parent.relationLinks = s.relationLinks
	}

	if s.objectTypeKeys != nil {
		s.parent.objectTypeKeys = s.objectTypeKeys
	}

	if s.store != nil {
		s.parent.store = s.store
	}
	if s.storeLastChangeIdByPath != nil {
		s.parent.storeLastChangeIdByPath = s.storeLastChangeIdByPath
	}
	if len(s.fileKeys) > 0 {
		s.parent.fileKeys = append(s.parent.fileKeys, s.fileKeys...)
	}
	if s.notifications != nil {
		s.parent.notifications = s.notifications
	}
	s.parent.changes = append(s.parent.changes, s.changes...)
	s.parent.fileInfo = s.fileInfo
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
			log.With("objectID", s.RootId()).Debugf("found trailing duplicated event %T", e.Msg.GetValue())
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
	fmt.Fprintf(buf, "ObjectTypeKeys: %v\n", s.ObjectTypeKeys())
	fmt.Fprintf(buf, "Relations:\n")
	for _, rel := range s.relationLinks {
		fmt.Fprintf(buf, "\t%v\n", rel)
	}

	fmt.Fprintf(buf, "\nDetails:\n")
	arena := &anyenc.Arena{}
	for k, v := range s.Details().IterateSorted() {
		raw := string(v.ToAnyEnc(arena).MarshalTo(nil))
		fmt.Fprintf(buf, "\t%s:\t%v\n", k, raw)
	}
	fmt.Fprintf(buf, "\nLocal details:\n")
	for k, v := range s.LocalDetails().IterateSorted() {
		raw := string(v.ToAnyEnc(arena).MarshalTo(nil))
		fmt.Fprintf(buf, "\t%s:\t%v\n", k, raw)
	}
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

func (s *State) SetDetails(d *domain.Details) *State {
	// TODO: GO-2062 Need to refactor details shortening, as it could cut string incorrectly
	// if d != nil && d.Fields != nil {
	//	shortenDetailsToLimit(s.rootId, d.Fields)
	// }

	local := d.CopyOnlyKeys(bundle.LocalAndDerivedRelationKeys...)
	if local != nil && local.Len() > 0 {
		for k, v := range local.Iterate() {
			s.SetLocalDetail(k, v)
		}
		s.details = d.CopyWithoutKeys(bundle.LocalAndDerivedRelationKeys...)
		return s
	}
	s.details = d
	return s
}

// SetDetailAndBundledRelation sets the detail value and bundled relation in case it is missing
func (s *State) SetDetailAndBundledRelation(key domain.RelationKey, value domain.Value) {
	s.AddBundledRelationLinks(key)
	s.SetDetail(key, value)
	return
}

func (s *State) SetLocalDetail(key domain.RelationKey, value domain.Value) {
	if s.localDetails == nil && s.parent != nil {
		d := s.parent.Details()
		if d != nil {
			// optimisation so we don't need to copy the struct if nothing has changed
			if prev := d.Get(key); prev.Ok() && prev.Equal(value) {
				return
			}
		}
		s.localDetails = s.parent.LocalDetails().Copy()
	}
	if s.localDetails == nil {
		s.localDetails = domain.NewDetails()
	}
	s.localDetails.Set(key, value)
	return
}

func (s *State) SetLocalDetails(d *domain.Details) {
	s.localDetails = d
}

func (s *State) AddDetails(details *domain.Details) {
	for k, v := range details.Iterate() {
		s.SetDetail(k, v)
	}
}

func (s *State) SetDetail(key domain.RelationKey, value domain.Value) {
	// TODO: GO-2062 Need to refactor details shortening, as it could cut string incorrectly
	// value = shortenValueToLimit(s.rootId, key, value)

	if slice.FindPos(bundle.LocalAndDerivedRelationKeys, key) > -1 {
		s.SetLocalDetail(key, value)
		return
	}

	if s.details == nil && s.parent != nil {
		d := s.parent.Details()
		if d != nil {
			// optimisation so we don't need to copy the struct if nothing has changed
			if prev := d.Get(key); prev.Ok() && prev.Equal(value) {
				return
			}
			s.details = d.Copy()
		}
	}
	if s.details == nil {
		s.details = domain.NewDetails()
	}
	s.details.Set(key, value)
	return
}

func (s *State) SetAlign(align model.BlockAlign, ids ...string) (err error) {
	if len(ids) == 0 {
		s.SetDetail(bundle.RelationKeyLayoutAlign, domain.Int64(align))
		ids = []string{TitleBlockID, DescriptionBlockID, FeaturedRelationsID}
	}
	for _, id := range ids {
		if b := s.Get(id); b != nil {
			b.Model().Align = align
		}
	}
	return
}

func (s *State) setStoreChangeId(path string, changeId string) *State {
	// do not copy map in purpose
	// we don't need to make diffs with parent stat
	s.storeLastChangeIdByPath = s.StoreLastChangeIdByPath()
	if s.storeLastChangeIdByPath == nil {
		s.storeLastChangeIdByPath = map[string]string{}
	}
	s.storeLastChangeIdByPath[path] = changeId
	return s
}

func (s *State) StoreLastChangeIdByPath() map[string]string {
	if s.storeLastChangeIdByPath == nil && s.parent != nil {
		return s.parent.StoreLastChangeIdByPath()
	}
	return s.storeLastChangeIdByPath
}

func (s *State) StoreChangeIdForPath(path string) string {
	m := s.StoreLastChangeIdByPath()
	if m == nil {
		return ""
	}
	return m[path]
}

type ObjectTypePair struct {
	ID  string
	Key domain.TypeKey
}

// SetObjectTypeKey sets the object type key. Smartblocks derive Type relation from it.
func (s *State) SetObjectTypeKey(objectTypeKey domain.TypeKey) *State {
	return s.SetObjectTypeKeys([]domain.TypeKey{objectTypeKey})
}

// SetObjectTypeKeys sets the object type keys. Smartblocks derive Type relation from it.
func (s *State) SetObjectTypeKeys(objectTypeKeys []domain.TypeKey) *State {
	s.objectTypeKeys = objectTypeKeys
	// we don't set it in the localDetails here
	return s
}

func (s *State) InjectLocalDetails(localDetails *domain.Details) {
	for k, v := range localDetails.Iterate() {
		s.SetDetailAndBundledRelation(k, v)
	}
}

func (s *State) LocalDetails() *domain.Details {
	if s.localDetails == nil && s.parent != nil {
		return s.parent.LocalDetails()
	}

	return s.localDetails
}

func (s *State) CombinedDetails() *domain.Details {
	// TODO Implement combined details struct with two underlying details
	return s.Details().Merge(s.LocalDetails())
}

func (s *State) Details() *domain.Details {
	if s.details == nil && s.parent != nil {
		return s.parent.Details()
	}
	return s.details
}

// ObjectTypeKeys returns the object types keys of the object
// in order to get object type id you need to derive it for the space
func (s *State) ObjectTypeKeys() []domain.TypeKey {
	if s.objectTypeKeys == nil && s.parent != nil {
		return s.parent.ObjectTypeKeys()
	}
	return s.objectTypeKeys
}

// ObjectTypeKey returns only the first objectType key and produce warning in case the state has more than 1 object type
// this method is useful because we have decided that currently objects can have only one object type, while preserving the ability to unlock this later
func (s *State) ObjectTypeKey() domain.TypeKey {
	objTypes := s.ObjectTypeKeys()
	if len(objTypes) == 0 && !s.noObjectType {
		log.Debugf("object %s (%s) has %d object types instead of 1",
			s.RootId(),
			s.Details().GetString(bundle.RelationKeyName),
			len(objTypes),
		)
	}

	if len(objTypes) > 0 {
		return objTypes[0]
	}

	return ""
}

func (s *State) Snippet() string {
	var builder strings.Builder
	var snippetSize int
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if text := b.Model().GetText(); text != nil &&
			text.Style != model.BlockContentText_Title &&
			text.Style != model.BlockContentText_Description {
			nextText := strings.TrimSpace(text.Text)
			if nextText != "" {
				if snippetSize > 0 {
					builder.WriteString("\n")
				}
				builder.WriteString(nextText)
				snippetSize += textutil.UTF16RuneCountString(nextText)
				if snippetSize >= snippetMaxSize {
					return false
				}
			}
		}
		return true
	})
	return textutil.TruncateEllipsized(builder.String(), snippetMaxSize)
}

func (s *State) FileRelationKeys() []domain.RelationKey {
	var keys []domain.RelationKey
	for _, rel := range s.GetRelationLinks() {
		// coverId can contain both hash or predefined cover id
		if rel.Format == model.RelationFormat_file {
			key := domain.RelationKey(rel.Key)
			if slice.FindPos(keys, key) == -1 {
				keys = append(keys, key)
			}
		}
		if rel.Key == bundle.RelationKeyCoverId.String() {
			coverType := s.Details().GetInt64(bundle.RelationKeyCoverType)
			if (coverType == 1 || coverType == 4 || coverType == 5) && slice.FindPos(keys, domain.RelationKey(rel.Key)) == -1 {
				keys = append(keys, domain.RelationKey(rel.Key))
			}
		}
	}
	return keys
}

// IterateLinkedFiles iterates over all file object ids in blocks and details
func (s *State) IterateLinkedFiles(proc func(id string)) {
	s.Iterate(func(block simple.Block) (isContinue bool) {
		if iter, ok := block.(simple.LinkedFilesIterator); ok {
			iter.IterateLinkedFiles(proc)
		}
		return true
	})
	s.IterateLinkedFilesInDetails(proc)
}

func (s *State) IterateLinkedFilesInDetails(proc func(id string)) {
	s.ModifyLinkedFilesInDetails(func(id string) string {
		proc(id)
		return id
	})
}

// ModifyLinkedFilesInDetails iterates over all file object ids in details and modifies them using modifier function.
// Detail is saved only if at least one id is changed
func (s *State) ModifyLinkedFilesInDetails(modifier func(id string) string) {
	details := s.Details()
	if details == nil {
		return
	}

	for _, key := range s.FileRelationKeys() {
		if key == bundle.RelationKeyCoverId {
			v := details.GetString(bundle.RelationKeyCoverId)
			_, err := cid.Decode(v)
			if err != nil {
				// this is an exception cause coverId can contain not a file hash but color
				continue
			}
		}

		s.modifyIdsInDetail(details, key, modifier)
	}
}

// ModifyLinkedObjectsInDetails iterates over all object ids in details and modifies them using modifier function.
// Detail is saved only if at least one id is changed
func (s *State) ModifyLinkedObjectsInDetails(modifier func(id string) string) {
	details := s.Details()
	if details == nil {
		return
	}
	for _, rel := range s.GetRelationLinks() {
		if rel.Format == model.RelationFormat_object {
			s.modifyIdsInDetail(details, domain.RelationKey(rel.Key), modifier)
		}
	}
}

func (s *State) modifyIdsInDetail(details *domain.Details, key domain.RelationKey, modifier func(id string) string) {
	if ids := details.WrapToStringList(key); len(ids) > 0 {
		var anyChanges bool
		for i, oldId := range ids {
			if oldId == "" {
				continue
			}
			newId := modifier(oldId)
			if oldId != newId {
				ids[i] = newId
				anyChanges = true
			}
		}
		if anyChanges {
			v := details.Get(key)
			if _, ok := v.TryString(); ok {
				s.SetDetail(key, domain.String(ids[0]))
			} else if _, ok := v.TryStringList(); ok {
				s.SetDetail(key, domain.StringList(ids))
			}
		}
	}
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
				// SpaceId is empty because only the fact that there is any diff matters here
				if msgs, _ := ob.Diff("", b); len(msgs) > 0 {
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
	if checkTitle && s.Details().GetString(bundle.RelationKeyName) != "" {
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

	if s.Details().GetString(bundle.RelationKeyDescription) != "" {
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
	objTypes := make([]domain.TypeKey, len(s.ObjectTypeKeys()))
	copy(objTypes, s.ObjectTypeKeys())

	storeKeyRemoved := s.StoreKeysRemoved()
	storeKeyRemovedCopy := make(map[string]struct{}, len(storeKeyRemoved))
	for i := range storeKeyRemoved {
		storeKeyRemovedCopy[i] = struct{}{}
	}
	copy := &State{
		ctx:                      s.ctx,
		blocks:                   blocks,
		rootId:                   s.rootId,
		details:                  s.Details().Copy(),
		localDetails:             s.LocalDetails().Copy(),
		relationLinks:            s.GetRelationLinks(), // Get methods copy inside
		objectTypeKeys:           objTypes,
		noObjectType:             s.noObjectType,
		migrationVersion:         s.migrationVersion,
		store:                    pbtypes.CopyStruct(s.Store(), false),
		storeLastChangeIdByPath:  s.StoreLastChangeIdByPath(), // todo: do we need to copy it?
		storeKeyRemoved:          storeKeyRemovedCopy,
		uniqueKeyInternal:        s.uniqueKeyInternal,
		originalCreatedTimestamp: s.originalCreatedTimestamp,
		fileInfo:                 s.fileInfo,
		notifications:            s.notifications,
		deviceStore:              s.deviceStore,
	}
	return copy
}

func (s *State) HasRelation(key string) bool {
	links := s.GetRelationLinks()
	for _, link := range links {
		if link.Key == key {
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

func (s *State) RemoveDetail(keys ...domain.RelationKey) (ok bool) {
	// TODO It could be lazily copied only if actual deletion is happened
	det := s.Details().Copy()
	if det != nil {
		for _, key := range keys {
			if det.Has(key) {
				det.Delete(key)
				ok = true
			}
		}
	}
	if ok {
		s.SetDetails(det)
	}
	return s.RemoveLocalDetail(keys...) || ok
}

func (s *State) RemoveLocalDetail(keys ...domain.RelationKey) (ok bool) {
	// TODO It could be lazily copied only if actual deletion is happened
	det := s.LocalDetails().Copy()
	if det != nil {
		for _, key := range keys {
			if det.Has(key) {
				det.Delete(key)
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
	s.store = pbtypes.CopyStruct(s.Store(), true)
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
		if nestedStore := pbtypes.GetStruct(store, key); nestedStore == nil {
			store.Fields[key] = pbtypes.Struct(&types.Struct{Fields: map[string]*types.Value{}})
		}
		store = pbtypes.GetStruct(store, key)
		storeStack = append(storeStack, store)
	}
	if store.Fields == nil {
		store.Fields = map[string]*types.Value{}
	}

	pathJoined := strings.Join(path, collectionKeysRemovedSeparator)
	if value != nil {
		oldval := store.Fields[path[len(path)-1]]
		changed = oldval.Compare(value) != 0
		store.Fields[path[len(path)-1]] = value
		s.setStoreChangeId(pathJoined, s.changeId)
		// in case we have previously removed this uniqueKeyInternal
		delete(s.storeKeyRemoved, pathJoined)
		return
	}
	changed = true
	delete(store.Fields, path[len(path)-1])

	// store all keys that were removed, so we explicitly know this and can make an additional handling
	s.storeKeyRemoved[strings.Join(path, collectionKeysRemovedSeparator)] = struct{}{}
	// cleaning empty structs from collection to avoid empty pb values
	s.setStoreChangeId(pathJoined, s.changeId)
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
		nestedStore := pbtypes.GetStruct(store, key)
		if nestedStore == nil {
			return false
		}
		store = nestedStore
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
	if det := s.LocalDetails(); det != nil {
		if det.Has(bundle.RelationKeyResolvedLayout) {
			//nolint:gosec
			return model.ObjectTypeLayout(det.GetInt64(bundle.RelationKeyResolvedLayout)), true
		}
	}
	return 0, false
}

func (s *State) SetContext(context session.Context) {
	s.ctx = context
}

// AddRelationLinks adds relation links to the state in case they are not already present
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

func (s *State) RemoveRelation(keys ...domain.RelationKey) {
	relLinks := s.GetRelationLinks()
	relLinksFiltered := make(pbtypes.RelationLinks, 0, len(relLinks))
	for _, link := range relLinks {
		if slice.FindPos(keys, domain.RelationKey(link.Key)) >= 0 {
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
	featuredList := s.Details().GetStringList(bundle.RelationKeyFeaturedRelations)
	featuredList = slice.Filter(featuredList, func(s string) bool {
		if slice.FindPos(keys, domain.RelationKey(s)) == -1 {
			return true
		}
		foundInFeatured = true
		return false
	})
	if foundInFeatured {
		s.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList(featuredList))
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

func (s *State) AddBundledRelationLinks(keys ...domain.RelationKey) {
	existingLinks := s.PickRelationLinks()

	var links []*model.RelationLink
	for _, key := range keys {
		if !existingLinks.Has(key.String()) {
			rel := bundle.MustGetRelation(key)
			links = append(links, &model.RelationLink{Format: rel.Format, Key: rel.Key})
		}
	}
	if len(links) > 0 {
		s.AddRelationLinks(links...)
	}
}

func (s *State) GetNotificationById(id string) *model.Notification {
	iterState := s.findStateWithNonEmptyNotifications()
	if iterState == nil {
		return nil
	}
	if notification, ok := iterState.notifications[id]; ok {
		return notification
	}
	return nil
}

func (s *State) AddNotification(notification *model.Notification) {
	if s.notifications == nil {
		s.notifications = make(map[string]*model.Notification)
	}
	if s.parent != nil {
		for _, n := range s.parent.ListNotifications() {
			if _, ok := s.notifications[n.Id]; !ok {
				s.notifications[n.Id] = pbtypes.CopyNotification(n)
			}
		}
	}
	s.notifications[notification.Id] = notification
}

func (s *State) ListNotifications() map[string]*model.Notification {
	iterState := s.findStateWithNonEmptyNotifications()
	if iterState == nil {
		return nil
	}
	return iterState.notifications
}

func (s *State) findStateWithNonEmptyNotifications() *State {
	iterState := s
	for iterState != nil && iterState.notifications == nil {
		iterState = iterState.parent
	}
	return iterState
}

func (s *State) ListDevices() map[string]*model.DeviceInfo {
	iterState := s.findStateWithDeviceInfo()
	if iterState == nil {
		return nil
	}
	return iterState.deviceStore
}

func (s *State) findStateWithDeviceInfo() *State {
	iterState := s
	for iterState != nil && iterState.deviceStore == nil {
		iterState = iterState.parent
	}
	return iterState
}

func (s *State) AddDevice(device *model.DeviceInfo) {
	if s.deviceStore == nil {
		s.deviceStore = map[string]*model.DeviceInfo{}
	}
	if s.parent != nil {
		for _, d := range s.parent.ListDevices() {
			if _, ok := s.deviceStore[d.Id]; !ok {
				s.deviceStore[d.Id] = pbtypes.CopyDevice(d)
			}
		}
	}
	if _, ok := s.deviceStore[device.Id]; ok {
		return
	}
	s.deviceStore[device.Id] = device
}

func (s *State) SetDeviceName(id, name string) {
	if s.deviceStore == nil {
		s.deviceStore = map[string]*model.DeviceInfo{}
	}
	if s.parent != nil {
		for _, d := range s.parent.ListDevices() {
			if _, ok := s.deviceStore[d.Id]; !ok {
				s.deviceStore[d.Id] = pbtypes.CopyDevice(d)
			}
		}
	}
	if _, ok := s.deviceStore[id]; !ok {
		device := &model.DeviceInfo{
			Id:      id,
			Name:    name,
			AddDate: time.Now().Unix(),
		}
		s.deviceStore[id] = device
		return
	}
	s.deviceStore[id].Name = name
}

func (s *State) GetDevice(id string) *model.DeviceInfo {
	iterState := s.findStateWithDeviceInfo()
	if iterState == nil {
		return nil
	}
	if device, ok := iterState.deviceStore[id]; ok {
		return device
	}
	return nil
}

// UniqueKeyInternal is the second part of uniquekey.UniqueKey. It used together with smartblock type for the ID derivation
// which will be unique and reproducible within the same space
func (s *State) UniqueKeyInternal() string {
	return s.uniqueKeyInternal
}

func (s *State) OriginalCreatedTimestamp() int64 {
	return s.originalCreatedTimestamp
}

// SetOriginalCreatedTimestamp should not be used in the normal flow, because there is no crdt changes for it
func (s *State) SetOriginalCreatedTimestamp(ts int64) {
	s.originalCreatedTimestamp = ts
}

func IsRequiredBlockId(targetId string) bool {
	return targetId == TitleBlockID ||
		targetId == DescriptionBlockID ||
		targetId == FeaturedRelationsID ||
		targetId == HeaderLayoutID
}
