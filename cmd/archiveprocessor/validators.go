//go:build !nogrpcserver && !_test

package main

import (
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/relation"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type validator func(snapshot *pb.ChangeSnapshot, info *useCaseInfo) error

var validators = []validator{
	validateRelationLinks,
	validateRelationBlocks,
	validateObjectDetails,
	validateObjectCustomTypes,
	validateBlockLinks,
}

func validateRelationLinks(s *pb.ChangeSnapshot, info *useCaseInfo) error {
	id := pbtypes.GetString(s.Data.Details, bundle.RelationKeyId.String())
	invalidRelationFound := false
	for _, rel := range s.Data.RelationLinks {
		if _, found := info.relsIds[rel.Key]; !bundle.HasRelation(rel.Key) && !found {
			invalidRelationFound = true
			fmt.Printf("object '%s' contains link to unknown relation: %s(%s)\n", id,
				rel.Key, pbtypes.GetString(s.Data.Details, bundle.RelationKeyName.String()))
		}
	}
	if invalidRelationFound {
		return fmt.Errorf("object '%s' contains invalid relation link", id)
	}
	return nil
}

func validateRelationBlocks(s *pb.ChangeSnapshot, info *useCaseInfo) error {
	id := pbtypes.GetString(s.Data.Details, bundle.RelationKeyId.String())
	invalidRelationFound := false
	var relKeys []string
	for _, b := range s.Data.Blocks {
		if rb, ok := simple.New(b).(relation.Block); ok {
			relKeys = append(relKeys, rb.Model().GetRelation().Key)
		}
	}
	relLinks := pbtypes.RelationLinks(s.Data.GetRelationLinks())
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

func validateObjectDetails(s *pb.ChangeSnapshot, info *useCaseInfo) error {
	var invalidDetailFound bool
	id := pbtypes.GetString(s.Data.Details, bundle.RelationKeyId.String())
	for k, v := range s.Data.Details.Fields {
		if k == bundle.RelationKeyLinks.String() {
			continue
		}
		var (
			rel relationWithFormat
			err error
		)
		rel, err = bundle.GetRelation(bundle.RelationKey(k))
		if err != nil {
			rel = getRelationLinkByKey(s.Data.RelationLinks, k)
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
				bundle.HasObjectType(strings.TrimPrefix(val, addr.ObjectTypeKeyToIdPrefix)) || val == addr.AnytypeProfileId {
				continue
			}
			_, found := info.ids[val]
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

func validateObjectCustomTypes(s *pb.ChangeSnapshot, info *useCaseInfo) error {
	id := pbtypes.GetString(s.Data.Details, bundle.RelationKeyId.String())
	customObjectFound := false
	for _, ot := range s.Data.ObjectTypes {
		if !bundle.HasObjectType(strings.TrimPrefix(ot, addr.ObjectTypeKeyToIdPrefix)) {
			customObjectFound = true
			fmt.Printf("object '%s' contains unknown object type: %s\n", id, ot)
		}
	}
	if customObjectFound {
		return fmt.Errorf("object '%s' contains custom object type", id)
	}
	return nil
}

func validateBlockLinks(s *pb.ChangeSnapshot, info *useCaseInfo) error {
	id := pbtypes.GetString(s.Data.Details, bundle.RelationKeyId.String())
	invalidBlockFound := false
	for _, b := range s.Data.Blocks {
		switch a := simple.New(b).(type) {
		case link.Block:
			_, found := info.ids[a.Model().GetLink().TargetBlockId]
			if !found {
				invalidBlockFound = true
				fmt.Printf("failed to find target id for link '%s' in block '%s' of object '%s'\n",
					a.Model().GetLink().TargetBlockId, a.Model().Id, id)
			}
		case bookmark.Block:
			_, found := info.ids[a.Model().GetBookmark().TargetObjectId]
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
				_, found := info.ids[mark.Param]
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
			_, found := info.ids[a.Model().GetDataview().TargetObjectId]
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
