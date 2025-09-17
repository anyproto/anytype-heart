//go:build !nogrpcserver && !_test

package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"

	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type validator func(snapshot *pb.SnapshotWithType, info *useCaseInfo) error

type keyWithIndex struct {
	key   string
	index int
}

var validators = []validator{
	validateRelationBlocks,
	validateDetails,
	validateObjectTypes,
	validateBlockLinks,
	validateDeleted,
	validateRelationOption,
}

func validateRelationBlocks(s *pb.SnapshotWithType, info *useCaseInfo) (err error) {
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
	var relKeys []string
	for _, b := range s.Snapshot.Data.Blocks {
		if rel := simple.New(b).Model().GetRelation(); rel != nil {
			relKeys = append(relKeys, rel.Key)
		}
	}
	relLinks := pbtypes.RelationLinks(s.Snapshot.Data.GetRelationLinks())
	for _, rk := range relKeys {
		if !relLinks.Has(rk) {
			if rel, errFound := bundle.GetRelation(domain.RelationKey(rk)); errFound == nil {
				s.Snapshot.Data.RelationLinks = append(s.Snapshot.Data.RelationLinks, &model.RelationLink{
					Key:    rk,
					Format: rel.Format,
				})
				continue
			}
			if relInfo, found := info.customTypesAndRelations[rk]; found {
				s.Snapshot.Data.RelationLinks = append(s.Snapshot.Data.RelationLinks, &model.RelationLink{
					Key:    rk,
					Format: relInfo.relationFormat,
				})
				continue
			}
			err = multierror.Append(err, fmt.Errorf("relation '%v' exists in relation block but not in relation links of object %s", rk, id))
		}
	}
	return err
}

func validateDetails(s *pb.SnapshotWithType, info *useCaseInfo) (err error) {
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())

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
				err = multierror.Append(err, fmt.Errorf("relation '%s' exists in details of object '%s', but not in the archive", k, id))
				continue
			}
		}
		if !canRelationContainObjectValues(rel.GetFormat()) {
			continue
		}

		var (
			values         = pbtypes.GetStringListValue(v)
			isUpdateNeeded bool
			newValues      = make([]string, 0, len(values))
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

			if k == bundle.RelationKeyAutoWidgetTargets.String() && val == "bin" {
				continue
			}

			_, found := info.objects[val]
			if !found {
				if isBrokenTemplate(k, val) {
					fmt.Println("WARNING: object", id, "is a template with no target type included in the archive, so it will be skipped")
					return errSkipObject
				}
				if isRecommendedRelationsKey(k) {
					// we can exclude recommended relations that are not found, because the majority of types are not imported
					fmt.Println("WARNING: type", id, "contains relation", val, "that is not included in the archive, so this relation will be excluded from the list")
					isUpdateNeeded = true
					continue
				}
				err = multierror.Append(err, fmt.Errorf("failed to find target id for detail '%s: %s' of object %s", k, val, id))
			} else {
				newValues = append(newValues, val)
			}
		}

		if isUpdateNeeded {
			s.Snapshot.Data.Details.Fields[k] = pbtypes.StringList(newValues)
		}
	}
	return err
}

func validateObjectTypes(s *pb.SnapshotWithType, info *useCaseInfo) (err error) {
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
	for _, ot := range s.Snapshot.Data.ObjectTypes {
		typeId := strings.TrimPrefix(ot, addr.ObjectTypeKeyToIdPrefix)
		if !bundle.HasObjectTypeByKey(domain.TypeKey(typeId)) {
			if _, found := info.customTypesAndRelations[typeId]; !found {
				err = multierror.Append(err, fmt.Errorf("object '%s' contains unknown object type: %s", id, ot))
			}
		}
	}
	return err
}

func validateBlockLinks(s *pb.SnapshotWithType, info *useCaseInfo) (err error) {
	var (
		id                       = pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
		widgetLinkBlocksToDelete []string
	)

	for _, b := range s.Snapshot.Data.Blocks {
		switch a := simple.New(b).(type) {
		case link.Block:
			target := a.Model().GetLink().TargetBlockId
			_, found := info.objects[target]
			if !found {
				if s.SbType == model.SmartBlockType_Widget {
					if isDefaultWidget(target) {
						continue
					}
					widgetLinkBlocksToDelete = append(widgetLinkBlocksToDelete, b.Id)
					continue
				}
				err = multierror.Append(err, fmt.Errorf("failed to find target id for link '%s' in block '%s' of object '%s'",
					a.Model().GetLink().TargetBlockId, a.Model().Id, id))
			}
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
		}
	}
	if err == nil && len(widgetLinkBlocksToDelete) > 0 {
		err = removeWidgetBlocks(s, id, widgetLinkBlocksToDelete)
	}

	return err
}

func validateDeleted(s *pb.SnapshotWithType, _ *useCaseInfo) error {
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())

	if pbtypes.GetBool(s.Snapshot.Data.Details, bundle.RelationKeyIsArchived.String()) {
		fmt.Println("WARNING: object", id, " is archived, so it will be skipped")
		return errSkipObject
	}

	if pbtypes.GetBool(s.Snapshot.Data.Details, bundle.RelationKeyIsDeleted.String()) {
		fmt.Println("WARNING: object", id, " is deleted, so it will be skipped")
		return errSkipObject
	}

	if pbtypes.GetBool(s.Snapshot.Data.Details, bundle.RelationKeyIsUninstalled.String()) {
		fmt.Println("WARNING: object", id, " is uninstalled, so it will be skipped")
		return errSkipObject
	}

	return nil
}

func validateRelationOption(s *pb.SnapshotWithType, info *useCaseInfo) error {
	if s.SbType != model.SmartBlockType_STRelationOption {
		return nil
	}

	key := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String())
	if bundle.HasRelation(domain.RelationKey(key)) {
		return nil
	}

	if _, found := info.customTypesAndRelations[key]; !found {
		id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
		fmt.Println("WARNING: relation key", key, "of relation option", id, "is not presented in the archive, so it will be skipped")
		return errSkipObject
	}
	return nil
}

func getRelationLinkByKey(links []*model.RelationLink, key string) *model.RelationLink {
	for _, l := range links {
		if l.Key == key {
			return l
		}
	}
	return nil
}

func snapshotHasKeyForHash(s *pb.SnapshotWithType, hash string) bool {
	for _, k := range s.Snapshot.FileKeys {
		if k.Hash == hash && len(k.Keys) > 0 {
			return true
		}
	}
	return false
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

func canRelationContainObjectValues(format model.RelationFormat) bool {
	switch format {
	case
		model.RelationFormat_status,
		model.RelationFormat_tag,
		model.RelationFormat_object:
		return true
	default:
		return false
	}
}

func isDefaultWidget(target string) bool {
	return slices.Contains([]string{
		widget.DefaultWidgetFavorite,
		widget.DefaultWidgetSet,
		widget.DefaultWidgetRecentlyEdited,
		widget.DefaultWidgetRecentlyOpened,
		widget.DefaultWidgetCollection,
	}, target)
}

func isBrokenTemplate(key, value string) bool {
	return key == bundle.RelationKeyTargetObjectType.String() && value == addr.MissingObject
}

func isRecommendedRelationsKey(key string) bool {
	return slices.Contains([]string{
		bundle.RelationKeyRecommendedRelations.String(),
		bundle.RelationKeyRecommendedFeaturedRelations.String(),
		bundle.RelationKeyRecommendedHiddenRelations.String(),
		bundle.RelationKeyRecommendedFileRelations.String(),
	}, key)
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
func removeWidgetBlocks(s *pb.SnapshotWithType, rootId string, linkBlockIds []string) error {
	widgetBlockIds := make([]string, 0, len(linkBlockIds))
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
		if slices.Contains(linkBlockIds, b.ChildrenIds[0]) {
			widgetBlockIds = append(widgetBlockIds, b.Id)
		}
	}

	if rootBlock == nil {
		return fmt.Errorf("root block not found")
	}

	rootBlock.ChildrenIds = slices.DeleteFunc(rootBlock.ChildrenIds, func(id string) bool {
		return slices.Contains(widgetBlockIds, id)
	})

	blocksToDelete := slices.Concat(widgetBlockIds, linkBlockIds)
	s.Snapshot.Data.Blocks = slices.DeleteFunc(s.Snapshot.Data.Blocks, func(b *model.Block) bool {
		return slices.Contains(blocksToDelete, b.Id)
	})

	return nil
}
