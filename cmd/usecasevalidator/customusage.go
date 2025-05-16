//go:build !nogrpcserver && !_test

package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func collectCustomObjectsUsageInfo(s *pb.SnapshotWithType, info *useCaseInfo) {
	collectInfoFromRelationLinks(s, info)
	collectInfoFromObjectTypes(s, info)
	collectInfoFromDetails(s, info)
}

func collectInfoFromRelationLinks(s *pb.SnapshotWithType, info *useCaseInfo) {
	for _, rel := range s.Snapshot.Data.RelationLinks {
		if v, found := info.customTypesAndRelations[rel.Key]; found {
			v.isUsed = true
			info.customTypesAndRelations[rel.Key] = v
			continue
		}
	}
}

func collectInfoFromObjectTypes(s *pb.SnapshotWithType, info *useCaseInfo) {
	for _, ot := range s.Snapshot.Data.ObjectTypes {
		typeId := strings.TrimPrefix(ot, addr.ObjectTypeKeyToIdPrefix)
		if ct, found := info.customTypesAndRelations[typeId]; found {
			ct.isUsed = true
			info.customTypesAndRelations[typeId] = ct
			continue
		}
	}
}

func collectInfoFromDetails(s *pb.SnapshotWithType, info *useCaseInfo) {
	for k, v := range s.Snapshot.Data.Details.Fields {
		if cr, found := info.customTypesAndRelations[k]; found {
			cr.isUsed = true
			info.customTypesAndRelations[k] = cr
		}
		if slices.Contains([]domain.RelationKey{
			bundle.RelationKeyRecommendedRelations, bundle.RelationKeyRecommendedFeaturedRelations,
			bundle.RelationKeyRecommendedHiddenRelations, bundle.RelationKeyRecommendedFileRelations,
		}, domain.RelationKey(k)) {
			values := pbtypes.GetStringListValue(v)
			for _, val := range values {
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

func printCustomObjectsUsageInfo(info *useCaseInfo) {
	fmt.Println("\n- Custom Types and Relations usage:")
	fmt.Println("Is used\t\tKey\t\t\t\tName\t\t\t\tId")
	for key, cInfo := range info.customTypesAndRelations {
		fmt.Printf("%v\t\t%24s%24s\t\t%s\n", cInfo.isUsed, key, cInfo.name, cInfo.id)
	}
}
