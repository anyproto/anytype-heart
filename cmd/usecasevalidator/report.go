//go:build !nogrpcserver && !_test

package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type reporter struct {
	changes map[string][]string
}

func (r *reporter) addMsg(id string, msg string) {
	r.changes[id] = append(r.changes[id], msg)
}

func (r *reporter) addRelBlockDeletionMsg(id string, absentKeys, blocksToDelete []string) {
	r.addMsg(id, fmt.Sprintf("relation blocks [%s] are removed as keys [%s] do not present in details",
		strings.Join(absentKeys, ","), strings.Join(blocksToDelete, ",")))
}

func (r *reporter) addWidgetBlockDeletionMsg(id string, blocksToDelete map[string]string) {
	r.addMsg(id, fmt.Sprintf("widget blocks deleted as target objects are not found: %v", blocksToDelete))
}

func (r *reporter) addCollectionUpdateMsg(id string, missedItems []string) {
	r.addMsg(id, fmt.Sprintf("items [%s] were removed from collection as they are not presented in the archive", strings.Join(missedItems, ",")))
}

func (r *reporter) addDetailUpdateMsg(id string, key string, removedValues []string) {
	r.addMsg(id, fmt.Sprintf("values [%s] were removed from detail '%s' as these objects are not presented in the archive",
		strings.Join(removedValues, ","), key))
}

func (r *reporter) addSkipMsg(id string, msg string) {
	r.addMsg(id, fmt.Sprintf("object is skipped: %s", msg))
}

func (r *reporter) print(info *useCaseInfo) {
	for id, msgs := range r.changes {
		objInfo := info.objects[id]
		fmt.Printf("\nChanges applied to '%s' object (name: '%s', type: '%s', sbType: '%s'):\n",
			id, objInfo.Name, objInfo.Type, objInfo.SbType.String())
		for _, msg := range msgs {
			fmt.Printf("* %s\n", msg)
		}
	}
}

func (r *reporter) report(config ReportConfig, info *useCaseInfo) {
	if config.ListObjects {
		listObjects(info)
	}

	if config.Changes {
		r.print(info)
	}

	if config.CustomUsage {
		printCustomObjectsUsageInfo(info)
	}

	if config.FileUsage {
		printFileUsageInfo(info)
	}
}

func listObjects(info *useCaseInfo) {
	fmt.Println("\nUsecase '" + info.useCase + "' content:\n\n- General objects:")
	fmt.Println("Id:  " + strings.Repeat(" ", 12) + "Smartblock Type -" + strings.Repeat(" ", 17) + "Type Key - Name")
	for id, obj := range info.objects {
		if lo.Contains([]smartblock.SmartBlockType{
			smartblock.SmartBlockTypeObjectType,
			smartblock.SmartBlockTypeRelation,
			smartblock.SmartBlockTypeSubObject,
			smartblock.SmartBlockTypeTemplate,
			smartblock.SmartBlockTypeRelationOption,
		}, obj.SbType) {
			continue
		}
		fmt.Printf("%s:\t%24s - %24s - %s\n", id[len(id)-4:], obj.SbType.String(), obj.Type, obj.Name)
	}

	fmt.Println("\n- Types:")
	fmt.Println("Id:  " + strings.Repeat(" ", 24) + "Key - Name")
	for id, key := range info.types {
		obj := info.objects[id]
		fmt.Printf("%s:\t%24s - %s\n", id[len(id)-4:], key, obj.Name)
	}

	fmt.Println("\n- Relations:")
	fmt.Println("Id:  " + strings.Repeat(" ", 24) + "Key - Name")
	for id, key := range info.relations {
		obj := info.objects[id]
		fmt.Printf("%s:\t%24s - %s\n", id[len(id)-4:], key, obj.Name)
	}

	fmt.Println("\n- Templates:")
	fmt.Println("Id:  " + strings.Repeat(" ", 31) + "Name - Target object type id")
	for id, target := range info.templates {
		obj := info.objects[id]
		fmt.Printf("%s:\t%32s - %s\n", id[len(id)-4:], obj.Name, target)
	}

	fmt.Println("\n- Relation Options:")
	fmt.Println("Id:  " + strings.Repeat(" ", 31) + "Name - Relation key")
	for id, key := range info.options {
		obj := info.objects[id]
		fmt.Printf("%s:\t%32s - %s\n", id[len(id)-4:], obj.Name, key)
	}

	fmt.Println("\n- File Objects:")
	fmt.Println("Id:  " + strings.Repeat(" ", 31) + "Name")
	for id := range info.fileObjects {
		obj := info.objects[id]
		fmt.Printf("%s:\t%32s\n", id[len(id)-4:], obj.Name)
	}
}

func collectCustomObjectsUsageInfo(s *common.SnapshotModel, info *useCaseInfo) {
	collectInfoFromRelationLinks(s.Data, info)
	collectInfoFromObjectTypes(s.Data, info)
	collectInfoFromDetails(s.Data, info)
	collectFileUsageInfo(s, info)
}

func collectInfoFromRelationLinks(s *common.StateSnapshot, info *useCaseInfo) {
	for _, rel := range s.RelationLinks {
		if v, found := info.customTypesAndRelations[rel.Key]; found {
			v.isUsed = true
			info.customTypesAndRelations[rel.Key] = v
			continue
		}
	}
}

func collectInfoFromObjectTypes(s *common.StateSnapshot, info *useCaseInfo) {
	for _, ot := range s.ObjectTypes {
		typeId := strings.TrimPrefix(ot, addr.ObjectTypeKeyToIdPrefix)
		if ct, found := info.customTypesAndRelations[typeId]; found {
			ct.isUsed = true
			info.customTypesAndRelations[typeId] = ct
			continue
		}
	}
}

func collectInfoFromDetails(s *common.StateSnapshot, info *useCaseInfo) {
	for k, v := range s.Details.Iterate() {
		if cr, found := info.customTypesAndRelations[k.String()]; found {
			cr.isUsed = true
			info.customTypesAndRelations[k.String()] = cr
		}
		if slices.Contains([]domain.RelationKey{
			bundle.RelationKeyRecommendedRelations, bundle.RelationKeyRecommendedFeaturedRelations,
			bundle.RelationKeyRecommendedHiddenRelations, bundle.RelationKeyRecommendedFileRelations,
		}, k) {
			for _, val := range v.StringList() {
				if key, found := info.relations[val]; found {
					if cr, foundToo := info.customTypesAndRelations[string(key)]; foundToo {
						cr.isUsed = true
						info.customTypesAndRelations[string(key)] = cr
					}
				}
			}
		}
	}
}

func collectFileUsageInfo(s *common.SnapshotModel, info *useCaseInfo) {
	if s.SbType == smartblock.SmartBlockTypeFileObject {
		return
	}

	for _, b := range s.Data.Blocks {
		fb := b.GetFile()
		if fb == nil || fb.TargetObjectId == "" {
			continue
		}
		fInfo, found := info.fileObjects[fb.TargetObjectId]
		if found {
			fInfo.isUsed = true
			info.fileObjects[fb.TargetObjectId] = fInfo
		}
	}

	for k, v := range s.Data.Details.Iterate() {
		var format model.RelationFormat
		rel, err := bundle.GetRelation(k)
		if err != nil {
			rel, found := info.customTypesAndRelations[k.String()]
			if !found {
				continue
			}
			format = rel.relationFormat
		} else {
			format = rel.Format
		}

		if format != model.RelationFormat_file && !isCover(k, s.Data.Details) {
			continue
		}

		values, ok := v.TryWrapToStringList()
		if !ok {
			continue
		}

		for _, val := range values {
			fInfo, found := info.fileObjects[val]
			if found {
				fInfo.isUsed = true
				info.fileObjects[val] = fInfo
			}
		}
	}
}

func printCustomObjectsUsageInfo(info *useCaseInfo) {
	fmt.Println("\n- Custom Types and Relations usage:")
	fmt.Println("Is used\t\tKey\t\t\t\tName\t\t\t\tId")
	for key, cInfo := range info.customTypesAndRelations {
		fmt.Printf("%v\t\t%24s%24s\t\t%s\n", cInfo.isUsed, key, cInfo.name, cInfo.id)
	}
}

func printFileUsageInfo(info *useCaseInfo) {
	fmt.Println("\n- File Usage:")

	sources := make(map[string]struct{})
	for id, fInfo := range info.fileObjects {
		old := ""
		if fInfo.isOld {
			old = "[OLD] "
		}
		fmt.Printf("%s'%s' - src: '%s', used: %v\n", old, id, fInfo.source, fInfo.isUsed)
		sources[fInfo.source] = struct{}{}
	}

	missingFileObjects := make([]string, 0, len(info.files))
	for name := range info.files {
		if _, ok := sources[name]; !ok {
			missingFileObjects = append(missingFileObjects, name)
		}
	}

	if len(missingFileObjects) > 0 {
		fmt.Println("\n- Files with no corresponding file objects:")
		for _, name := range missingFileObjects {
			fmt.Printf("* %s\n", name)
		}
	}
}

func isCover(k domain.RelationKey, details *domain.Details) bool {
	if k != bundle.RelationKeyCoverId {
		return false
	}
	coverType := details.GetInt64(bundle.RelationKeyCoverType)
	return coverType == 1 || coverType == 4 || coverType == 5
}
