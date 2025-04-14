//go:build !nogrpcserver && !_test

package main

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestValidateDetails(t *testing.T) {
	t.Run("snapshot is valid", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyName.String():     pbtypes.String("snap shot"),
				bundle.RelationKeyType.String():     pbtypes.String(bundle.TypeKeyTask.URL()),
				bundle.RelationKeyAssignee.String(): pbtypes.String("kirill"),
				bundle.RelationKeyMentions.String(): pbtypes.StringList([]string{"task1", "task2"}),
				bundle.RelationKeyFeaturedRelations.String(): pbtypes.StringList([]string{
					bundle.RelationKeyType.URL(), "rel-customTag",
				}),
			}},
		}}}
		info := &useCaseInfo{
			objects: map[string]objectInfo{
				bundle.TypeKeyTask.URL(): {},
				"kirill":                 {},
				"task1":                  {},
				"task2":                  {},
			},
			customTypesAndRelations: map[string]customInfo{
				"rel-customTag": {},
			},
		}

		// when
		err := validateDetails(s, info)

		// then
		assert.NoError(t, err)
	})

	t.Run("some object is missing", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyAssignee.String(): pbtypes.String("kirill"),
			}},
		}}}
		info := &useCaseInfo{}

		// when
		err := validateDetails(s, info)

		// then
		assert.Error(t, err)
	})

	t.Run("broken template", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyTargetObjectType.String(): pbtypes.String(addr.MissingObject),
			}},
		}}}
		info := &useCaseInfo{}

		// when
		err := validateDetails(s, info)

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, errSkipObject, err)
	})

	t.Run("exclude missing recommendedRelations", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList([]string{
					bundle.RelationKeyCreator.BundledURL(),
					bundle.RelationKeyCreatedDate.BundledURL(),
				}),
				bundle.RelationKeyRecommendedFeaturedRelations.String(): pbtypes.StringList([]string{
					bundle.RelationKeyType.BundledURL(),
					bundle.RelationKeyTag.BundledURL(),
				}),
			}},
		}}}
		info := &useCaseInfo{
			objects: map[string]objectInfo{
				bundle.RelationKeyCreator.BundledURL(): {},
				bundle.RelationKeyTag.BundledURL():     {},
			},
		}

		// when
		err := validateDetails(s, info)

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{bundle.RelationKeyCreator.BundledURL()}, pbtypes.GetStringList(s.Snapshot.Data.Details, bundle.RelationKeyRecommendedRelations.String()))
		assert.Equal(t, []string{bundle.RelationKeyTag.BundledURL()}, pbtypes.GetStringList(s.Snapshot.Data.Details, bundle.RelationKeyRecommendedFeaturedRelations.String()))
	})
}

func TestRemoveWidgetBlock(t *testing.T) {
	rootId := "root"
	t.Run("blocks were removed", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{
				{Id: rootId, ChildrenIds: []string{"w1", "w2", "w3"}},
				{Id: "w1", ChildrenIds: []string{"l1"}},
				{Id: "w2", ChildrenIds: []string{"l2"}},
				{Id: "w3", ChildrenIds: []string{"l3"}},
				{Id: "l1"}, {Id: "l2"}, {Id: "l3"},
			},
		}}}

		// when
		err := removeWidgetBlocks(s, rootId, []string{"l2", "l3"})

		// then
		assert.NoError(t, err)
		assert.Len(t, s.Snapshot.Data.Blocks, 3)
		assert.Equal(t, []string{"w1"}, s.Snapshot.Data.Blocks[0].ChildrenIds)
	})

	t.Run("no root found", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{
				{Id: "wrong root id", ChildrenIds: []string{"w1"}},
				{Id: "w1", ChildrenIds: []string{"l1"}}, {Id: "l1"},
			},
		}}}

		// when
		err := removeWidgetBlocks(s, rootId, []string{"l1"})

		// then
		assert.Error(t, err)
	})
}
