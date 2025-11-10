//go:build !nogrpcserver && !_test

package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type (
	validator func(snapshot *pb.SnapshotWithType, info *useCaseInfo, fixConfig FixConfig, reporter *reporter) (skip bool, err error)

	relationWithFormat interface {
		GetFormat() model.RelationFormat
	}
)

var validators = []validator{
	validateRelationBlocks,
	validateDetails,
	validateObjectTypes,
	validateBlockLinks,
	validateDeleted,
	validateRelationOption,
	validateCollection,
}

func validateRelationBlocks(s *pb.SnapshotWithType, info *useCaseInfo, fixConfig FixConfig, reporter *reporter) (skip bool, err error) {
	blockIdsByKey := make(map[string][]string)
	for _, b := range s.Snapshot.Data.Blocks {
		if rel := simple.New(b).Model().GetRelation(); rel != nil {
			blockIds := blockIdsByKey[rel.Key]
			blockIds = append(blockIds, b.Id)
			blockIdsByKey[rel.Key] = blockIds
		}
	}
	details := s.Snapshot.Data.Details.Fields
	var absentKeys, blocksToDelete []string
	for rk, ids := range blockIdsByKey {
		_, ok := details[rk]
		if ok {
			continue
		}

		if _, found := info.customTypesAndRelations[rk]; found || bundle.HasRelation(domain.RelationKey(rk)) {
			s.Snapshot.Data.Details.Fields[rk] = pbtypes.Null()
			continue
		}

		if fixConfig.DeleteInvalidRelationBlocks {
			absentKeys = append(absentKeys, rk)
			blocksToDelete = append(blocksToDelete, ids...)
		} else {
			err = multierror.Append(err, fmt.Errorf("relation '%v' exists in relation block but not in details", rk))
		}
	}

	if len(absentKeys) > 0 {
		reporter.addRelBlockDeletionMsg(getId(s), absentKeys, blocksToDelete)
		removeBlocks(s, blocksToDelete)
	}

	return false, err
}

func validateDetails(s *pb.SnapshotWithType, info *useCaseInfo, fixConfig FixConfig, reporter *reporter) (skip bool, err error) {
	id := getId(s)

	var relationsToDelete []string
	for k, v := range s.Snapshot.Data.Details.Fields {
		if isLinkRelation(k) {
			continue
		}
		var (
			rel relationWithFormat
			e   error
		)
		rel, e = bundle.GetRelation(domain.RelationKey(k))
		if e != nil {
			var found bool
			rel, found = info.customTypesAndRelations[k]
			if !found {
				if fixConfig.DeleteInvalidDetails || isDesktopRelation(k) {
					relationsToDelete = append(relationsToDelete, k)
				} else {
					err = multierror.Append(err, fmt.Errorf("relation '%s' exists in details of object '%s', but not in the archive", k, id))
				}
				continue
			}
		}
		if !isObjectRelation(rel.GetFormat()) && !isCover(k, s.Snapshot.Data.Details) {
			continue
		}

		var (
			values        = pbtypes.GetStringListValue(v)
			skippedValues = make([]string, 0, len(values))
			newValues     = make([]string, 0, len(values))
		)

		for _, val := range values {
			if bundle.HasRelation(domain.RelationKey(strings.TrimPrefix(val, addr.RelationKeyToIdPrefix))) ||
				bundle.HasObjectTypeByKey(domain.TypeKey(strings.TrimPrefix(val, addr.ObjectTypeKeyToIdPrefix))) || val == addr.AnytypeProfileId {
				continue
			}

			if k == bundle.RelationKeyFeaturedRelations.String() {
				if _, found := info.customTypesAndRelations[val]; found {
					continue
				}
			}

			if k == bundle.RelationKeySpaceDashboardId.String() && val == "lastOpened" {
				continue
			}

			_, found := info.objects[val]
			if found {
				newValues = append(newValues, val)
				continue
			}
			if isBrokenTemplate(k, val) {
				reporter.addSkipMsg(id, "template for a missing type")
				return true, nil
			}
			skippedValues = append(skippedValues, val)
			if fixConfig.DeleteInvalidDetailValues || isRecommendedRelationsKey(k) {
				continue
			}
			err = multierror.Append(err, fmt.Errorf("failed to find target id for detail '%s: %s' of object %s", k, val, id))
		}

		if len(skippedValues) > 0 {
			reporter.addDetailUpdateMsg(id, k, skippedValues)
			s.Snapshot.Data.Details.Fields[k] = pbtypes.StringList(newValues)
		}
	}

	if len(relationsToDelete) > 0 {
		reporter.addMsg(id, fmt.Sprintf("details [%s] were deleted from state", strings.Join(relationsToDelete, ",")))
		for _, key := range relationsToDelete {
			delete(s.Snapshot.Data.Details.Fields, key)
		}
	}

	return false, err
}

func validateObjectTypes(s *pb.SnapshotWithType, info *useCaseInfo, fixConfig FixConfig, reporter *reporter) (skip bool, err error) {
	for _, ot := range s.Snapshot.Data.ObjectTypes {
		typeId := strings.TrimPrefix(ot, addr.ObjectTypeKeyToIdPrefix)
		_, found := info.customTypesAndRelations[typeId]
		if bundle.HasObjectTypeByKey(domain.TypeKey(typeId)) || found {
			continue
		}
		formattedMsg := "object contains unknown object type: %s"
		if fixConfig.SkipInvalidTypes {
			reporter.addMsg(getId(s), fmt.Sprintf(formattedMsg, ot))
			return true, nil
		}
		err = multierror.Append(err, fmt.Errorf(formattedMsg, ot))
	}
	return false, err
}

func validateBlockLinks(s *pb.SnapshotWithType, info *useCaseInfo, _ FixConfig, reporter *reporter) (skip bool, err error) {
	id := getId(s)
	widgetLinkBlocksToDelete := make(map[string]string)

	for _, b := range s.Snapshot.Data.Blocks {
		switch a := simple.New(b).(type) {
		case link.Block:
			target := a.Model().GetLink().TargetBlockId
			_, found := info.objects[target]
			if found {
				continue
			}
			if s.SbType == model.SmartBlockType_Widget {
				widgetLinkBlocksToDelete[b.Id] = target
				continue
			}
			err = multierror.Append(err, fmt.Errorf("failed to find target id for link '%s' in block '%s' of object '%s'",
				a.Model().GetLink().TargetBlockId, a.Model().Id, id))
		case bookmark.Block:
			target := a.Model().GetBookmark().TargetObjectId
			if target == "" {
				continue
			}
			_, found := info.objects[target]
			if !found {
				err = multierror.Append(err, fmt.Errorf("failed to find target id for bookmark '%s' in block '%s' of object '%s'", target, a.Model().Id, id))
			}
		case text.Block:
			for _, mark := range a.Model().GetText().GetMarks().GetMarks() {
				if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
					continue
				}
				_, found := info.objects[mark.Param]
				if !found {
					err = multierror.Append(err, fmt.Errorf("failed to find target id for mention '%s' in block '%s' of object '%s'",
						mark.Param, a.Model().Id, id))
				}
			}
		case dataview.Block:
			if a.Model().GetDataview().TargetObjectId == "" {
				continue
			}
			_, found := info.objects[a.Model().GetDataview().TargetObjectId]
			if !found {
				err = multierror.Append(err, fmt.Errorf("failed to find target id for dataview '%s' in block '%s' of object '%s'",
					a.Model().GetDataview().TargetObjectId, a.Model().Id, id))
			}
		case file.Block:
			if a.Model().GetFile().TargetObjectId == "" {
				continue
			}
			_, found := info.objects[a.Model().GetFile().TargetObjectId]
			if !found {
				err = multierror.Append(err, fmt.Errorf("failed to find target id for file '%s' in block '%s' of object '%s'",
					a.Model().GetFile().TargetObjectId, a.Model().Id, id))
			}
		}
	}
	if err == nil && len(widgetLinkBlocksToDelete) > 0 {
		reporter.addWidgetBlockDeletionMsg(id, widgetLinkBlocksToDelete)
		err = removeWidgetBlocks(s, id, widgetLinkBlocksToDelete)
	}

	return false, err
}

func validateDeleted(s *pb.SnapshotWithType, _ *useCaseInfo, _ FixConfig, _ *reporter) (skip bool, err error) {
	isArchived := pbtypes.GetBool(s.Snapshot.Data.Details, bundle.RelationKeyIsArchived.String())
	isDeleted := pbtypes.GetBool(s.Snapshot.Data.Details, bundle.RelationKeyIsDeleted.String())
	isUninstalled := pbtypes.GetBool(s.Snapshot.Data.Details, bundle.RelationKeyIsUninstalled.String())
	return isArchived || isDeleted || isUninstalled, nil
}

func validateRelationOption(s *pb.SnapshotWithType, info *useCaseInfo, _ FixConfig, reporter *reporter) (skip bool, err error) {
	if s.SbType != model.SmartBlockType_STRelationOption {
		return false, nil
	}

	key := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String())
	if bundle.HasRelation(domain.RelationKey(key)) {
		return false, nil
	}

	if _, found := info.customTypesAndRelations[key]; !found {
		reporter.addSkipMsg(getId(s), fmt.Sprintf("relation '%s' does not exist", key))
		return true, nil
	}
	return false, nil
}

func validateCollection(s *pb.SnapshotWithType, info *useCaseInfo, fix FixConfig, reporter *reporter) (skip bool, err error) {
	if s.Snapshot.Data.Collections == nil {
		return false, nil
	}

	id := getId(s)
	collection := pbtypes.GetStringList(s.Snapshot.Data.Collections, template.CollectionStoreKey)
	newCollection := make([]string, 0, len(collection))
	missedItems := make([]string, 0, len(collection))

	for _, item := range collection {
		if _, found := info.objects[item]; found {
			newCollection = append(newCollection, item)
			continue
		}
		missedItems = append(missedItems, item)
		if !fix.DeleteInvalidCollectionItems {
			err = multierror.Append(err, fmt.Errorf("object '%s' is included in store slice of collection '%s', but not in the archive", item, id))
		}
	}
	if len(missedItems) > 0 && fix.DeleteInvalidCollectionItems {
		reporter.addCollectionUpdateMsg(id, missedItems)
		s.Snapshot.Data.Collections.Fields[template.CollectionStoreKey] = pbtypes.StringList(newCollection)
	}

	return
}

// these relations will be overwritten on import
func isLinkRelation(k string) bool {
	return slices.Contains([]string{
		bundle.RelationKeyLinks.String(),
		bundle.RelationKeySourceObject.String(),
		bundle.RelationKeyBacklinks.String(),
		bundle.RelationKeyMentions.String(),
	}, k)
}

func isObjectRelation(format model.RelationFormat) bool {
	return format == model.RelationFormat_status || format == model.RelationFormat_object || format == model.RelationFormat_tag
}

func isBrokenTemplate(key, value string) bool {
	return key == bundle.RelationKeyTargetObjectType.String() && value == addr.MissingObject
}

func isRecommendedRelationsKey(key string) bool {
	// we can exclude recommended relations that are not found, because the majority of types are not imported
	return slices.Contains([]string{
		bundle.RelationKeyRecommendedRelations.String(),
		bundle.RelationKeyRecommendedFeaturedRelations.String(),
		bundle.RelationKeyRecommendedHiddenRelations.String(),
		bundle.RelationKeyRecommendedFileRelations.String(),
	}, key)
}

func isDesktopRelation(key string) bool {
	return key == "data" || key == "isNew" || key == "layoutFormat"
}

// removeWidgetBlocks removes link blocks and widget blocks from Widget object.
// For each link block we should remove parent widget block and remove its id from root's children.
// Widget object blocks structure:
//
//	root
//	|--- widget1
//	|    |--- link1
//	|
//	|--- widget2
//	     |--- link2
func removeWidgetBlocks(s *pb.SnapshotWithType, rootId string, blocks map[string]string) error {
	var rootBlock *model.Block

	for _, b := range s.Snapshot.Data.Blocks {
		if b.Id == rootId {
			rootBlock = b
			continue
		}
		// widget block has only one child - link block
		if len(b.ChildrenIds) != 1 {
			continue
		}
		if _, found := blocks[b.ChildrenIds[0]]; found {
			blocks[b.Id] = ""
		}
	}

	if rootBlock == nil {
		return fmt.Errorf("root block not found")
	}

	rootBlock.ChildrenIds = slices.DeleteFunc(rootBlock.ChildrenIds, func(id string) bool {
		_, found := blocks[id]
		return found
	})

	s.Snapshot.Data.Blocks = slices.DeleteFunc(s.Snapshot.Data.Blocks, func(b *model.Block) bool {
		_, found := blocks[b.Id]
		return found
	})

	return nil
}

func removeBlocks(s *pb.SnapshotWithType, blockIds []string) {
	if len(blockIds) == 0 {
		return
	}

	s.Snapshot.Data.Blocks = slice.DeleteOrApplyFunc(s.Snapshot.Data.Blocks, func(b *model.Block) bool {
		return slices.Contains(blockIds, b.Id)
	}, func(block *model.Block) *model.Block {
		block.ChildrenIds = slices.DeleteFunc(block.ChildrenIds, func(id string) bool {
			return slices.Contains(blockIds, id)
		})
		return block
	})
}

func getId(s *pb.SnapshotWithType) string {
	return pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
}
