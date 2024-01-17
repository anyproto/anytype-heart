//go:build !nogrpcserver && !_test

package main

import (
	"fmt"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/relation"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type validator func(snapshot *pb.SnapshotWithType, info *useCaseInfo) error

var validators = []validator{
	validateRelationLinks,
	validateRelationBlocks,
	validateDetails,
	validateObjectTypes,
	validateBlockLinks,
	validateFileKeys,
	validateDeleted,
}

func validateRelationLinks(s *pb.SnapshotWithType, info *useCaseInfo) error {
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
	invalidRelationFound := false
	for _, rel := range s.Snapshot.Data.RelationLinks {
		if _, found := info.customTypesAndRelations[rel.Key]; !bundle.HasRelation(rel.Key) && !found {
			invalidRelationFound = true
			fmt.Printf("object '%s' contains link to unknown relation: %s(%s)\n", id,
				rel.Key, pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyName.String()))
		}
	}
	if invalidRelationFound {
		return fmt.Errorf("object '%s' contains invalid relation link", id)
	}
	return nil
}

func validateRelationBlocks(s *pb.SnapshotWithType, info *useCaseInfo) error {
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
	invalidRelationFound := false
	var relKeys []string
	for _, b := range s.Snapshot.Data.Blocks {
		if rb, ok := simple.New(b).(relation.Block); ok {
			relKeys = append(relKeys, rb.Model().GetRelation().Key)
		}
	}
	relLinks := pbtypes.RelationLinks(s.Snapshot.Data.GetRelationLinks())
	for _, rk := range relKeys {
		if !relLinks.Has(rk) {
			invalidRelationFound = true
			fmt.Printf("relation '%v' exists in relation block but not in relation links of object %s\n", rk, id)
		}
	}
	if invalidRelationFound {
		return fmt.Errorf("relation block of object '%s' contains invalid relation", id)
	}
	return nil
}

func validateDetails(s *pb.SnapshotWithType, info *useCaseInfo) error {
	var invalidDetailFound bool
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
	for k, v := range s.Snapshot.Data.Details.Fields {
		if k == bundle.RelationKeyLinks.String() || k == bundle.RelationKeySourceObject.String() || k == bundle.RelationKeyBacklinks.String() {
			continue
		}
		var (
			rel relationWithFormat
			err error
		)
		rel, err = bundle.GetRelation(domain.RelationKey(k))
		if err != nil {
			rel = getRelationLinkByKey(s.Snapshot.Data.RelationLinks, k)
			if rel == nil {
				invalidDetailFound = true
				fmt.Printf("relation '%s' exists in details of object '%s', but not in relation links\n", k, id)
				continue
			}

		}
		if rel.GetFormat() != model.RelationFormat_object && rel.GetFormat() != model.RelationFormat_tag && rel.GetFormat() != model.RelationFormat_status {
			continue
		}

		vals := pbtypes.GetStringListValue(v)
		for _, val := range vals {
			if bundle.HasRelation(strings.TrimPrefix(val, addr.RelationKeyToIdPrefix)) ||
				bundle.HasObjectTypeByKey(domain.TypeKey(strings.TrimPrefix(val, addr.ObjectTypeKeyToIdPrefix))) || val == addr.AnytypeProfileId {
				continue
			}
			_, found := info.objects[val]
			if !found {
				invalidDetailFound = true
				fmt.Printf("failed to find target id for detail '%s: %s' of object %s\n", k, val, id)
			}
		}
	}
	if invalidDetailFound {
		return fmt.Errorf("object '%s' contains invalid detail", id)
	}
	return nil
}

func validateObjectTypes(s *pb.SnapshotWithType, info *useCaseInfo) error {
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
	typeNotFound := false
	for _, ot := range s.Snapshot.Data.ObjectTypes {
		typeId := strings.TrimPrefix(ot, addr.ObjectTypeKeyToIdPrefix)
		if !bundle.HasObjectTypeByKey(domain.TypeKey(typeId)) {
			if _, found := info.customTypesAndRelations[typeId]; found {
				continue
			}
			typeNotFound = true
			fmt.Printf("object '%s' contains unknown object type: %s\n", id, ot)
		}
	}
	if typeNotFound {
		return fmt.Errorf("object '%s' contains unknown object type", id)
	}
	return nil
}

func validateBlockLinks(s *pb.SnapshotWithType, info *useCaseInfo) error {
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
	invalidBlockFound := false
	for _, b := range s.Snapshot.Data.Blocks {
		switch a := simple.New(b).(type) {
		case link.Block:
			target := a.Model().GetLink().TargetBlockId
			_, found := info.objects[target]
			if !found {
				if s.SbType == model.SmartBlockType_Widget && lo.Contains([]string{widget.DefaultWidgetFavorite, widget.DefaultWidgetSet, widget.DefaultWidgetRecent, widget.DefaultWidgetCollection}, target) {
					continue
				}
				invalidBlockFound = true
				fmt.Printf("failed to find target id for link '%s' in block '%s' of object '%s'\n",
					a.Model().GetLink().TargetBlockId, a.Model().Id, id)
			}
		case bookmark.Block:
			_, found := info.objects[a.Model().GetBookmark().TargetObjectId]
			if !found {
				invalidBlockFound = true
				fmt.Printf("failed to find target id for bookmark '%s' in block '%s' of object '%s'\n",
					a.Model().GetBookmark().TargetObjectId, a.Model().Id, id)
			}
		case text.Block:
			for _, mark := range a.Model().GetText().GetMarks().GetMarks() {
				if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
					continue
				}
				_, found := info.objects[mark.Param]
				if !found {
					invalidBlockFound = true
					fmt.Printf("failed to find target id for mention '%s' in block '%s' of object '%s'\n",
						mark.Param, a.Model().Id, id)
				}
			}
		case dataview.Block:
			if a.Model().GetDataview().TargetObjectId == "" {
				continue
			}
			_, found := info.objects[a.Model().GetDataview().TargetObjectId]
			if !found {
				invalidBlockFound = true
				fmt.Printf("failed to find target id for dataview '%s' in block '%s' of object '%s'\n",
					a.Model().GetDataview().TargetObjectId, a.Model().Id, id)
			}
		}
	}
	if invalidBlockFound {
		return fmt.Errorf("block of object %s contains links to non-existent objects", id)
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

func validateFileKeys(s *pb.SnapshotWithType, _ *useCaseInfo) error {
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
	invalidKeyFound := false
	for _, r := range s.Snapshot.Data.RelationLinks {
		if r.Format == model.RelationFormat_file || r.Key == bundle.RelationKeyCoverId.String() {
			for _, hash := range pbtypes.GetStringList(s.Snapshot.GetData().GetDetails(), r.Key) {
				if r.Format != model.RelationFormat_file {
					_, err := cid.Parse(hash)
					if err != nil {
						continue
					}
				}
				if !snapshotHasKeyForHash(s, hash) {
					fmt.Printf("object '%s' has file detail '%s' has hash '%s' which keys are not in the snapshot\n", id, r.Key, hash)
					invalidKeyFound = true
				}
			}
		}
	}
	for _, b := range s.Snapshot.Data.Blocks {
		if v, ok := simple.New(b).(simple.FileHashes); ok {
			var hashes []string
			hashes = v.FillFileHashes(hashes)
			if len(hashes) == 0 {
				continue
			}
			for _, hash := range hashes {
				if !snapshotHasKeyForHash(s, hash) {
					fmt.Printf("file block '%s' of object '%s' has hash '%s' which keys are not in the snapshot\n", b.Id, id, hash)
					invalidKeyFound = true
				}
			}
		}
	}
	if invalidKeyFound {
		return fmt.Errorf("found invalid blocks with hashes")
	}
	return nil
}

func validateDeleted(s *pb.SnapshotWithType, _ *useCaseInfo) error {
	if pbtypes.GetBool(s.Snapshot.Data.Details, bundle.RelationKeyIsArchived.String()) {
		return fmt.Errorf("object is archived")
	}

	if pbtypes.GetBool(s.Snapshot.Data.Details, bundle.RelationKeyIsDeleted.String()) {
		return fmt.Errorf("object is deleted")
	}

	if pbtypes.GetBool(s.Snapshot.Data.Details, bundle.RelationKeyIsUninstalled.String()) {
		return fmt.Errorf("object is uninstalled")
	}

	return nil
}
