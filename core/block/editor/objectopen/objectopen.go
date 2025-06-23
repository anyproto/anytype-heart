package objectopen

import (
	"errors"
	"fmt"
	"sort"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var log = logging.Logger("core.block.editor.objectopen")

type ObjectOpen interface {
	Show() (obj *model.ObjectView, err error)
	EnabledRelationAsDependentObjects()
	CheckSubscriptions(info smartblock.ApplyInfo) (changed bool)
	CloseSubscription()
}

type smartBlock struct {
	smartblock.SmartBlock
	objectStore     objectstore.ObjectStore
	spaceIdResolver idresolver.Resolver

	depIds                             []string // slice must be sorted
	includeRelationObjectsAsDependents bool     // used by some clients
	lastDepDetails                     map[string]*domain.Details
	recordsSub                         database.Subscription
	closeRecordsSub                    func()
}

func New(sb smartblock.SmartBlock, objectStore objectstore.ObjectStore, spaceIdResolver idresolver.Resolver) ObjectOpen {
	comp := &smartBlock{
		SmartBlock:      sb,
		lastDepDetails:  make(map[string]*domain.Details),
		objectStore:     objectStore,
		spaceIdResolver: spaceIdResolver,
	}
	sb.AddHook(func(info smartblock.ApplyInfo) (err error) {
		comp.CheckSubscriptions(info)
		return nil
	}, smartblock.HookAfterApply)
	sb.AddHook(func(info smartblock.ApplyInfo) (err error) {
		comp.CloseSubscription()
		return nil
	}, smartblock.HookOnClose)

	return comp
}

func (sb *smartBlock) EnabledRelationAsDependentObjects() {
	sb.includeRelationObjectsAsDependents = true
}

func (sb *smartBlock) Show() (*model.ObjectView, error) {
	// TODO Why here?
	// sb.updateRestrictions()

	details, err := sb.fetchMeta()
	if err != nil {
		return nil, err
	}

	undo, redo := sb.History().Counters()

	// todo: sb.Relations() makes extra query to read objectType which we already have here
	// the problem is that we can have an extra object type of the set in the objectTypes so we can't reuse it
	return &model.ObjectView{
		RootId:        sb.RootId(),
		Type:          sb.Type().ToProto(),
		Blocks:        sb.Blocks(),
		Details:       details,
		RelationLinks: sb.GetRelationLinks(),
		Restrictions:  sb.Restrictions().Proto(),
		History: &model.ObjectViewHistorySize{
			Undo: undo,
			Redo: redo,
		},
	}, nil
}

func (sb *smartBlock) fetchMeta() (details []*model.ObjectViewDetailsSet, err error) {
	if sb.closeRecordsSub != nil {
		sb.closeRecordsSub()
		sb.closeRecordsSub = nil
	}

	depIds := sb.dependentSmartIds(sb.includeRelationObjectsAsDependents, true, true)
	sb.setDependentIDs(depIds)

	perSpace := sb.partitionIdsBySpace(sb.depIds)

	recordsCh := make(chan *domain.Details, 10)
	sb.recordsSub = database.NewSubscription(nil, recordsCh)

	var records []database.Record
	closers := make([]func(), 0, len(perSpace))

	for spaceId, perSpaceDepIds := range perSpace {
		spaceIndex := sb.objectStore.SpaceIndex(spaceId)

		recs, closeRecordsSub, err := spaceIndex.QueryByIdsAndSubscribeForChanges(perSpaceDepIds, sb.recordsSub)
		if err != nil {
			for _, closer := range closers {
				closer()
			}
			// datastore unavailable, cancel the subscription
			sb.recordsSub.Close()
			sb.closeRecordsSub = nil
			return nil, fmt.Errorf("subscribe: %w", err)
		}

		closers = append(closers, closeRecordsSub)
		records = append(records, recs...)
	}
	sb.closeRecordsSub = func() {
		for _, closer := range closers {
			closer()
		}
	}

	details = make([]*model.ObjectViewDetailsSet, 0, len(records)+1)

	// add self details
	details = append(details, &model.ObjectViewDetailsSet{
		Id:      sb.Id(),
		Details: sb.CombinedDetails().ToProto(),
	})

	for _, rec := range records {
		details = append(details, &model.ObjectViewDetailsSet{
			Id:      rec.Details.GetString(bundle.RelationKeyId),
			Details: rec.Details.ToProto(),
		})
	}
	go sb.metaListener(recordsCh)
	return
}
func (sb *smartBlock) metaListener(ch chan *domain.Details) {
	for {
		rec, ok := <-ch
		if !ok {
			return
		}
		sb.Lock()
		sb.onMetaChange(rec)
		sb.Unlock()
	}
}

func (sb *smartBlock) onMetaChange(details *domain.Details) {
	if details == nil {
		return
	}
	id := details.GetString(bundle.RelationKeyId)
	var msgs []*pb.EventMessage
	if v, exists := sb.lastDepDetails[id]; exists {
		diff, keysToUnset := domain.StructDiff(v, details)
		if id == sb.Id() {
			// if we've got update for ourselves, we are only interested in local-only details, because the rest details changes will be appended when applying records in the current sb
			diff = diff.CopyOnlyKeys(bundle.LocalRelationsKeys...)
		}

		msgs = append(msgs, state.StructDiffIntoEvents(sb.SpaceID(), id, diff, keysToUnset)...)
	} else {
		msgs = append(msgs, event.NewMessage(sb.SpaceID(), &pb.EventMessageValueOfObjectDetailsSet{
			ObjectDetailsSet: &pb.EventObjectDetailsSet{
				Id:      id,
				Details: details.ToProto(),
			},
		}))
	}
	sb.lastDepDetails[id] = details

	if len(msgs) == 0 {
		return
	}

	sb.SendEvent(msgs)
}

// dependentSmartIds returns list of dependent objects in this order: Simple blocks(Link, mentions in Text), Relations. Both of them are returned in the order of original blocks/relations
func (sb *smartBlock) dependentSmartIds(includeRelations, includeObjTypes, includeCreatorModifier bool) (ids []string) {
	// TODO Change NewState to smth like GetState()
	return objectlink.DependentObjectIDs(sb.SmartBlock.NewState(), sb.Space(), objectlink.Flags{
		Blocks:                   true,
		Details:                  true,
		Relations:                includeRelations,
		Types:                    includeObjTypes,
		CreatorModifierWorkspace: includeCreatorModifier,
	})
}

func (sb *smartBlock) needToCheckSubscriptions(applyInfo smartblock.ApplyInfo) bool {
	return hasDepIds(sb.GetRelationLinks(), applyInfo.UndoAction) || isBacklinksChanged(applyInfo.Events)
}

// TODO Hook afterApply
func (sb *smartBlock) CheckSubscriptions(applyInfo smartblock.ApplyInfo) (changed bool) {
	if !sb.needToCheckSubscriptions(applyInfo) {
		fmt.Println("CHECK: No changes")
		return false
	}
	fmt.Println("CHECK: OK")

	depIDs := sb.dependentSmartIds(sb.includeRelationObjectsAsDependents, true, true)
	changed = sb.setDependentIDs(depIDs)

	if sb.recordsSub == nil {
		return true
	}
	newIDs := sb.recordsSub.Subscribe(sb.depIds)

	perSpace := sb.partitionIdsBySpace(newIDs)

	for spaceId, ids := range perSpace {
		spaceIndex := sb.objectStore.SpaceIndex(spaceId)
		records, err := spaceIndex.QueryByIds(ids)
		if err != nil {
			log.Errorf("queryById error: %v", err)
		}
		for _, rec := range records {
			sb.onMetaChange(rec.Details)
		}
	}

	return true
}

func (sb *smartBlock) partitionIdsBySpace(ids []string) map[string][]string {
	perSpace := map[string][]string{}
	for _, id := range ids {
		if dateObject, parseErr := dateutil.BuildDateObjectFromId(id); parseErr == nil {
			perSpace[sb.SpaceID()] = append(perSpace[sb.SpaceID()], dateObject.Id())
			continue
		}

		spaceId, err := sb.spaceIdResolver.ResolveSpaceID(id)
		if errors.Is(err, domain.ErrObjectNotFound) {
			perSpace[sb.SpaceID()] = append(perSpace[sb.SpaceID()], id)
			continue
		}

		if err != nil {
			perSpace[sb.SpaceID()] = append(perSpace[sb.SpaceID()], id)
			log.With("id", id).Warn("resolve space id", zap.Error(err))
			continue
		}
		perSpace[spaceId] = append(perSpace[spaceId], id)
	}
	return perSpace
}

func (sb *smartBlock) setDependentIDs(depIDs []string) (changed bool) {
	sort.Strings(depIDs)
	if slice.SortedEquals(sb.depIds, depIDs) {
		return false
	}
	// TODO Use algo for sorted strings
	removed, _ := slice.DifferenceRemovedAdded(sb.depIds, depIDs)
	for _, id := range removed {
		delete(sb.lastDepDetails, id)
	}
	sb.depIds = depIDs
	return true
}

// TODO Hook onClose
func (sb *smartBlock) CloseSubscription() {
	fmt.Println("CLOSE OK")
	if sb.closeRecordsSub != nil {
		sb.closeRecordsSub()
		sb.closeRecordsSub = nil
	}
}

func hasDepIds(relations pbtypes.RelationLinks, act *undo.Action) bool {
	if act == nil {
		return true
	}
	if act.ObjectTypes != nil {
		return true
	}
	if act.Details != nil {
		if act.Details.Before == nil || act.Details.After == nil {
			return true
		}

		for k, after := range act.Details.After.Iterate() {
			rel := relations.Get(string(k))
			if rel != nil && (rel.Format == model.RelationFormat_status ||
				rel.Format == model.RelationFormat_tag ||
				rel.Format == model.RelationFormat_object ||
				rel.Format == model.RelationFormat_file ||
				isCoverId(rel)) {

				before := act.Details.Before.Get(k)
				// Check that value is actually changed
				if !before.Ok() || !before.Equal(after) {
					return true
				}
			}
		}
	}

	for _, edit := range act.Change {
		if ls, ok := edit.After.(linkSource); ok && ls.HasSmartIds() {
			return true
		}
		if ls, ok := edit.Before.(linkSource); ok && ls.HasSmartIds() {
			return true
		}
	}
	for _, add := range act.Add {
		if ls, ok := add.(linkSource); ok && ls.HasSmartIds() {
			return true
		}
	}
	for _, rem := range act.Remove {
		if ls, ok := rem.(linkSource); ok && ls.HasSmartIds() {
			return true
		}
	}
	return false
}

type linkSource interface {
	FillSmartIds(ids []string) []string
	HasSmartIds() bool
}

// We need to provide the author's name if we download an image with unsplash
// for the cover image inside an inner smartblock
// CoverId can be either a file, a gradient, an icon, or a color
func isCoverId(rel *model.RelationLink) bool {
	return rel.Key == bundle.RelationKeyCoverId.String()
}

func isBacklinksChanged(msgs []simple.EventMessage) bool {
	for _, msg := range msgs {
		if amend, ok := msg.Msg.Value.(*pb.EventMessageValueOfObjectDetailsAmend); ok {
			for _, detail := range amend.ObjectDetailsAmend.Details {
				if detail.Key == bundle.RelationKeyBacklinks.String() {
					return true
				}
			}
		}
	}
	return false
}
