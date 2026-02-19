//go:build !nogrpcserver && !_test

package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type (
	validator func(snapshot *common.SnapshotModel, info *useCaseInfo, fixConfig FixConfig, reporter *reporter) (skip bool, err error)

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

func validateRelationBlocks(s *common.SnapshotModel, info *useCaseInfo, fixConfig FixConfig, reporter *reporter) (skip bool, err error) {
	blockIdsByKey := make(map[domain.RelationKey][]string)
	for _, b := range s.Data.Blocks {
		if rel := simple.New(b).Model().GetRelation(); rel != nil {
			blockIds := blockIdsByKey[domain.RelationKey(rel.Key)]
			blockIds = append(blockIds, b.Id)
			blockIdsByKey[domain.RelationKey(rel.Key)] = blockIds
		}
	}
	details := s.Data.Details
	var absentKeys, blocksToDelete []string
	for rk, ids := range blockIdsByKey {
		if details.Has(rk) {
			continue
		}

		if _, found := info.customTypesAndRelations[rk.String()]; found || bundle.HasRelation(rk) {
			details.SetNull(rk)
			continue
		}

		if fixConfig.DeleteInvalidRelationBlocks {
			absentKeys = append(absentKeys, rk.String())
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

func validateDetails(s *common.SnapshotModel, info *useCaseInfo, fixConfig FixConfig, reporter *reporter) (skip bool, err error) {
	id := getId(s)

	var relationsToDelete []string
	for k, v := range s.Data.Details.Iterate() {
		if isLinkRelation(k) {
			continue
		}
		var (
			rel relationWithFormat
			e   error
		)
		rel, e = bundle.GetRelation(k)
		if e != nil {
			var found bool
			rel, found = info.customTypesAndRelations[k.String()]
			if !found {
				if fixConfig.DeleteInvalidDetails || isDesktopRelation(k.String()) {
					relationsToDelete = append(relationsToDelete, k.String())
				} else {
					err = multierror.Append(err, fmt.Errorf("relation '%s' exists in details of object '%s', but not in the archive", k, id))
				}
				continue
			}
		}
		if !isObjectRelation(rel.GetFormat()) && !isCover(k, s.Data.Details) {
			continue
		}

		var (
			values        = v.StringList()
			skippedValues = make([]string, 0, len(values))
			newValues     = make([]string, 0, len(values))
		)

		for _, val := range values {
			if bundle.HasRelation(domain.RelationKey(strings.TrimPrefix(val, addr.RelationKeyToIdPrefix))) ||
				bundle.HasObjectTypeByKey(domain.TypeKey(strings.TrimPrefix(val, addr.ObjectTypeKeyToIdPrefix))) || val == addr.AnytypeProfileId {
				continue
			}

			if k == bundle.RelationKeyFeaturedRelations {
				if _, found := info.customTypesAndRelations[val]; found {
					continue
				}
			}

			if k == bundle.RelationKeySpaceDashboardId && val == "lastOpened" {
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
			reporter.addDetailUpdateMsg(id, k.String(), skippedValues)
			s.Data.Details.SetStringList(k, newValues)
		}
	}

	if len(relationsToDelete) > 0 {
		reporter.addMsg(id, fmt.Sprintf("details [%s] were deleted from state", strings.Join(relationsToDelete, ",")))
		for _, key := range relationsToDelete {
			s.Data.Details.Delete(domain.RelationKey(key))
		}
	}

	return false, err
}

func validateObjectTypes(s *common.SnapshotModel, info *useCaseInfo, fixConfig FixConfig, reporter *reporter) (skip bool, err error) {
	for _, ot := range s.Data.ObjectTypes {
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

func validateBlockLinks(s *common.SnapshotModel, info *useCaseInfo, _ FixConfig, reporter *reporter) (skip bool, err error) {
	id := getId(s)
	widgetLinkBlocksToDelete := make(map[string]string)

	for _, b := range s.Data.Blocks {
		switch a := simple.New(b).(type) {
		case link.Block:
			target := a.Model().GetLink().TargetBlockId
			_, found := info.objects[target]
			if found {
				continue
			}
			if s.SbType == smartblock.SmartBlockTypeWidget {
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

func validateDeleted(s *common.SnapshotModel, _ *useCaseInfo, _ FixConfig, _ *reporter) (skip bool, err error) {
	isArchived := s.Data.Details.GetBool(bundle.RelationKeyIsArchived)
	isDeleted := s.Data.Details.GetBool(bundle.RelationKeyIsDeleted)
	isUninstalled := s.Data.Details.GetBool(bundle.RelationKeyIsUninstalled)
	return isArchived || isDeleted || isUninstalled, nil
}

func validateRelationOption(s *common.SnapshotModel, info *useCaseInfo, _ FixConfig, reporter *reporter) (skip bool, err error) {
	if s.SbType != smartblock.SmartBlockTypeRelationOption {
		return false, nil
	}

	key := s.Data.Details.GetString(bundle.RelationKeyRelationKey)
	if bundle.HasRelation(domain.RelationKey(key)) {
		return false, nil
	}

	if _, found := info.customTypesAndRelations[key]; !found {
		reporter.addSkipMsg(getId(s), fmt.Sprintf("relation '%s' does not exist", key))
		return true, nil
	}
	return false, nil
}

func validateCollection(s *common.SnapshotModel, info *useCaseInfo, fix FixConfig, reporter *reporter) (skip bool, err error) {
	if s.Data.Collections == nil {
		return false, nil
	}

	id := getId(s)
	collection := pbtypes.GetStringList(s.Data.Collections, template.CollectionStoreKey)
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
		s.Data.Collections.Fields[template.CollectionStoreKey] = pbtypes.StringList(newCollection)
	}

	return
}

// these relations will be overwritten on import
func isLinkRelation(k domain.RelationKey) bool {
	return slices.Contains([]domain.RelationKey{
		bundle.RelationKeyLinks,
		bundle.RelationKeySourceObject,
		bundle.RelationKeyBacklinks,
		bundle.RelationKeyMentions,
	}, k)
}

func isObjectRelation(format model.RelationFormat) bool {
	return format == model.RelationFormat_status || format == model.RelationFormat_object || format == model.RelationFormat_tag
}

func isBrokenTemplate(key domain.RelationKey, value string) bool {
	return key == bundle.RelationKeyTargetObjectType && value == addr.MissingObject
}

func isRecommendedRelationsKey(key domain.RelationKey) bool {
	// we can exclude recommended relations that are not found, because the majority of types are not imported
	return slices.Contains([]domain.RelationKey{
		bundle.RelationKeyRecommendedRelations,
		bundle.RelationKeyRecommendedFeaturedRelations,
		bundle.RelationKeyRecommendedHiddenRelations,
		bundle.RelationKeyRecommendedFileRelations,
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
func removeWidgetBlocks(s *common.SnapshotModel, rootId string, blocks map[string]string) error {
	var rootBlock *model.Block

	for _, b := range s.Data.Blocks {
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

	s.Data.Blocks = slices.DeleteFunc(s.Data.Blocks, func(b *model.Block) bool {
		_, found := blocks[b.Id]
		return found
	})

	return nil
}

func removeBlocks(s *common.SnapshotModel, blockIds []string) {
	if len(blockIds) == 0 {
		return
	}

	s.Data.Blocks = lo.FilterMap(s.Data.Blocks, func(block *model.Block, _ int) (*model.Block, bool) {
		if slices.Contains(blockIds, block.Id) {
			return nil, false
		}
		block.ChildrenIds = slices.DeleteFunc(block.ChildrenIds, func(id string) bool {
			return slices.Contains(blockIds, id)
		})
		return block, true
	})
}

func getId(s *common.SnapshotModel) string {
	return s.Data.Details.GetString(bundle.RelationKeyId)
}
