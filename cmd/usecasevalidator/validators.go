//go:build !nogrpcserver && !_test

package main

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	"github.com/samber/lo"

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

var validators = []validator{
	validateRelationLinks,
	validateRelationBlocks,
	validateDetails,
	validateObjectTypes,
	validateBlockLinks,
	validateFileKeys,
	validateDeleted,
	validateRelationOption,
}

func validateRelationLinks(s *pb.SnapshotWithType, info *useCaseInfo) (err error) {
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
	for _, rel := range s.Snapshot.Data.RelationLinks {
		if bundle.HasRelation(rel.Key) {
			continue
		}
		if _, found := info.customTypesAndRelations[rel.Key]; found {
			continue
		}
		err = multierror.Append(err, fmt.Errorf("object '%s' contains link to unknown relation: %s(%s)", id,
			rel.Key, pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyName.String())))
	}
	return err
}

func validateRelationBlocks(s *pb.SnapshotWithType, _ *useCaseInfo) (err error) {
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
			rel = getRelationLinkByKey(s.Snapshot.Data.RelationLinks, k)
			if rel == nil {
				err = multierror.Append(err, fmt.Errorf("relation '%s' exists in details of object '%s', but not in relation links", k, id))
				continue
			}
		}
		if !canRelationContainObjectValues(rel.GetFormat()) {
			continue
		}

		values := pbtypes.GetStringListValue(v)
		for _, val := range values {
			if bundle.HasRelation(strings.TrimPrefix(val, addr.RelationKeyToIdPrefix)) ||
				bundle.HasObjectTypeByKey(domain.TypeKey(strings.TrimPrefix(val, addr.ObjectTypeKeyToIdPrefix))) || val == addr.AnytypeProfileId {
				continue
			}

			if k == bundle.RelationKeyFeaturedRelations.String() {
				if _, found := info.customTypesAndRelations[val]; found {
					continue
				}
			}

			_, found := info.objects[val]
			if !found {
				err = multierror.Append(err, fmt.Errorf("failed to find target id for detail '%s: %s' of object %s", k, val, id))
			}
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
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
	for _, b := range s.Snapshot.Data.Blocks {
		switch a := simple.New(b).(type) {
		case link.Block:
			target := a.Model().GetLink().TargetBlockId
			_, found := info.objects[target]
			if !found {
				if s.SbType == model.SmartBlockType_Widget && isDefaultWidget(target) {
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
	return err
}

func validateFileKeys(s *pb.SnapshotWithType, _ *useCaseInfo) (err error) {
	id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
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
					err = multierror.Append(err, fmt.Errorf("object '%s' has file detail '%s' has hash '%s' which keys are not in the snapshot", id, r.Key, hash))
				}
			}
		}
	}
	for _, b := range s.Snapshot.Data.Blocks {
		if v, ok := simple.New(b).(simple.FileHashes); ok {
			hashes := v.FillFileHashes([]string{})
			if len(hashes) == 0 {
				continue
			}
			for _, hash := range hashes {
				if !snapshotHasKeyForHash(s, hash) {
					err = multierror.Append(err, fmt.Errorf("file block '%s' of object '%s' has hash '%s' which keys are not in the snapshot", b.Id, id, hash))
				}
			}
		}
	}
	return err
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

func validateRelationOption(s *pb.SnapshotWithType, info *useCaseInfo) error {
	if s.SbType != model.SmartBlockType_STRelationOption {
		return nil
	}

	key := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String())
	if bundle.HasRelation(key) {
		return nil
	}

	if _, found := info.customTypesAndRelations[key]; !found {
		id := pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyId.String())
		return fmt.Errorf("failed to find relation key %s of relation option %s", key, id)
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

func isLinkRelation(k string) bool {
	return k == bundle.RelationKeyLinks.String() || k == bundle.RelationKeySourceObject.String() || k == bundle.RelationKeyBacklinks.String()
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
	return lo.Contains([]string{
		widget.DefaultWidgetFavorite,
		widget.DefaultWidgetSet,
		widget.DefaultWidgetRecent,
		widget.DefaultWidgetCollection,
	}, target)
}
