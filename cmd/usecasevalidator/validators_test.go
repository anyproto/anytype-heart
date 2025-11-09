//go:build !nogrpcserver && !_test

package main

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
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
		skip, err := validateDetails(s, info, FixConfig{}, &reporter{make(map[string][]string)})

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
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
		skip, err := validateDetails(s, info, FixConfig{}, &reporter{make(map[string][]string)})

		// then
		assert.Error(t, err)
		assert.False(t, skip)
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
		skip, err := validateDetails(s, info, FixConfig{}, &reporter{make(map[string][]string)})

		// then
		assert.NoError(t, err)
		assert.True(t, skip)
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
		skip, err := validateDetails(s, info, FixConfig{}, &reporter{make(map[string][]string)})

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
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

func TestValidateRelationBlocks(t *testing.T) {
	t.Run("all relation blocks have corresponding details", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("obj1"),
				bundle.RelationKeyName.String(): pbtypes.String("Test Object"),
				bundle.RelationKeyTag.String():  pbtypes.StringList([]string{"tag1"}),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{Key: bundle.RelationKeyName.String()}}},
				{Id: "b2", Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{Key: bundle.RelationKeyTag.String()}}},
			},
		}}}
		info := &useCaseInfo{customTypesAndRelations: make(map[string]customInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateRelationBlocks(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("relation block without detail - bundled relation adds null value", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{Key: bundle.RelationKeyDescription.String()}}},
			},
		}}}
		info := &useCaseInfo{customTypesAndRelations: make(map[string]customInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateRelationBlocks(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
		assert.NotNil(t, s.Snapshot.Data.Details.Fields[bundle.RelationKeyDescription.String()])
	})

	t.Run("relation block for custom relation adds null value", func(t *testing.T) {
		// given
		customRelKey := "customRelation"
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{Key: customRelKey}}},
			},
		}}}
		info := &useCaseInfo{
			customTypesAndRelations: map[string]customInfo{
				customRelKey: {id: "customRel1", relationFormat: model.RelationFormat_shorttext},
			},
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateRelationBlocks(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
		assert.NotNil(t, s.Snapshot.Data.Details.Fields[customRelKey])
	})

	t.Run("relation block with unknown relation - delete with config", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "root", ChildrenIds: []string{"b1"}},
				{Id: "b1", Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{Key: "unknownRelation"}}},
			},
		}}}
		info := &useCaseInfo{customTypesAndRelations: make(map[string]customInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateRelationBlocks(s, info, FixConfig{DeleteInvalidRelationBlocks: true}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
		assert.Len(t, s.Snapshot.Data.Blocks, 1)
		assert.Equal(t, "root", s.Snapshot.Data.Blocks[0].Id)
	})

	t.Run("relation block with unknown relation - error without config", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{Key: "unknownRelation"}}},
			},
		}}}
		info := &useCaseInfo{customTypesAndRelations: make(map[string]customInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateRelationBlocks(s, info, FixConfig{}, reporter)

		// then
		assert.Error(t, err)
		assert.False(t, skip)
	})
}

func TestValidateObjectTypes(t *testing.T) {
	t.Run("all object types are valid - bundled types", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			ObjectTypes: []string{addr.ObjectTypeKeyToIdPrefix + bundle.TypeKeyPage.String()},
		}}}
		info := &useCaseInfo{customTypesAndRelations: make(map[string]customInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateObjectTypes(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("all object types are valid - custom types", func(t *testing.T) {
		// given
		customTypeKey := "customType"
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			ObjectTypes: []string{addr.ObjectTypeKeyToIdPrefix + customTypeKey},
		}}}
		info := &useCaseInfo{
			customTypesAndRelations: map[string]customInfo{
				customTypeKey: {id: "customType1"},
			},
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateObjectTypes(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("unknown object type - skip with config", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			ObjectTypes: []string{addr.ObjectTypeKeyToIdPrefix + "unknownType"},
		}}}
		info := &useCaseInfo{customTypesAndRelations: make(map[string]customInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateObjectTypes(s, info, FixConfig{SkipInvalidTypes: true}, reporter)

		// then
		assert.NoError(t, err)
		assert.True(t, skip)
	})

	t.Run("unknown object type - error without config", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			ObjectTypes: []string{addr.ObjectTypeKeyToIdPrefix + "unknownType"},
		}}}
		info := &useCaseInfo{customTypesAndRelations: make(map[string]customInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateObjectTypes(s, info, FixConfig{}, reporter)

		// then
		assert.Error(t, err)
		assert.False(t, skip)
	})
}

func TestValidateBlockLinks(t *testing.T) {
	t.Run("all link blocks target existing objects", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "target1"}}},
			},
		}}}
		info := &useCaseInfo{
			objects: map[string]objectInfo{
				"target1": {Name: "Target"},
			},
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateBlockLinks(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("link block with missing target - error", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "missingTarget"}}},
			},
		}}, SbType: model.SmartBlockType_Page}
		info := &useCaseInfo{objects: make(map[string]objectInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateBlockLinks(s, info, FixConfig{}, reporter)

		// then
		assert.Error(t, err)
		assert.False(t, skip)
	})

	t.Run("widget link block with missing target - deleted", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String(): pbtypes.String("widget1"),
				}},
				Blocks: []*model.Block{
					{Id: "widget1", ChildrenIds: []string{"w1"}},
					{Id: "w1", ChildrenIds: []string{"l1"}},
					{Id: "l1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "missingTarget"}}},
				},
			}},
			SbType: model.SmartBlockType_Widget,
		}
		info := &useCaseInfo{objects: make(map[string]objectInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateBlockLinks(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
		assert.Len(t, s.Snapshot.Data.Blocks, 1)
		assert.Equal(t, "widget1", s.Snapshot.Data.Blocks[0].Id)
	})

	t.Run("bookmark block with target", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{TargetObjectId: "target1"}}},
			},
		}}}
		info := &useCaseInfo{
			objects: map[string]objectInfo{"target1": {}},
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateBlockLinks(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("bookmark block with missing target - error", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{TargetObjectId: "missingTarget"}}},
			},
		}}}
		info := &useCaseInfo{objects: make(map[string]objectInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateBlockLinks(s, info, FixConfig{}, reporter)

		// then
		assert.Error(t, err)
		assert.False(t, skip)
	})

	t.Run("text block with mention", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Marks: &model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{
							{Type: model.BlockContentTextMark_Mention, Param: "user1"},
						},
					},
				}}},
			},
		}}}
		info := &useCaseInfo{
			objects: map[string]objectInfo{"user1": {}},
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateBlockLinks(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("text block with missing mention - error", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
					Marks: &model.BlockContentTextMarks{
						Marks: []*model.BlockContentTextMark{
							{Type: model.BlockContentTextMark_Mention, Param: "missingUser"},
						},
					},
				}}},
			},
		}}}
		info := &useCaseInfo{objects: make(map[string]objectInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateBlockLinks(s, info, FixConfig{}, reporter)

		// then
		assert.Error(t, err)
		assert.False(t, skip)
	})

	t.Run("dataview block with target", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{TargetObjectId: "set1"}}},
			},
		}}}
		info := &useCaseInfo{
			objects: map[string]objectInfo{"set1": {}},
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateBlockLinks(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("file block with target", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
			Blocks: []*model.Block{
				{Id: "b1", Content: &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: "file1"}}},
			},
		}}}
		info := &useCaseInfo{
			objects: map[string]objectInfo{"file1": {}},
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateBlockLinks(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})
}

func TestValidateDeleted(t *testing.T) {
	t.Run("object is not deleted", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():          pbtypes.String("obj1"),
				bundle.RelationKeyIsArchived.String():  pbtypes.Bool(false),
				bundle.RelationKeyIsDeleted.String():   pbtypes.Bool(false),
				bundle.RelationKeyIsUninstalled.String(): pbtypes.Bool(false),
			}},
		}}}

		// when
		skip, err := validateDeleted(s, nil, FixConfig{}, nil)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("object is archived", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():         pbtypes.String("obj1"),
				bundle.RelationKeyIsArchived.String(): pbtypes.Bool(true),
			}},
		}}}

		// when
		skip, err := validateDeleted(s, nil, FixConfig{}, nil)

		// then
		assert.NoError(t, err)
		assert.True(t, skip)
	})

	t.Run("object is deleted", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():        pbtypes.String("obj1"),
				bundle.RelationKeyIsDeleted.String(): pbtypes.Bool(true),
			}},
		}}}

		// when
		skip, err := validateDeleted(s, nil, FixConfig{}, nil)

		// then
		assert.NoError(t, err)
		assert.True(t, skip)
	})

	t.Run("object is uninstalled", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():            pbtypes.String("obj1"),
				bundle.RelationKeyIsUninstalled.String(): pbtypes.Bool(true),
			}},
		}}}

		// when
		skip, err := validateDeleted(s, nil, FixConfig{}, nil)

		// then
		assert.NoError(t, err)
		assert.True(t, skip)
	})
}

func TestValidateRelationOption(t *testing.T) {
	t.Run("not a relation option - skip validation", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String(): pbtypes.String("obj1"),
				}},
			}},
			SbType: model.SmartBlockType_Page,
		}
		info := &useCaseInfo{}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateRelationOption(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("relation option for bundled relation", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String():          pbtypes.String("opt1"),
					bundle.RelationKeyRelationKey.String(): pbtypes.String(bundle.RelationKeyTag.String()),
				}},
			}},
			SbType: model.SmartBlockType_STRelationOption,
		}
		info := &useCaseInfo{customTypesAndRelations: make(map[string]customInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateRelationOption(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("relation option for custom relation", func(t *testing.T) {
		// given
		customRelKey := "customRelation"
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String():          pbtypes.String("opt1"),
					bundle.RelationKeyRelationKey.String(): pbtypes.String(customRelKey),
				}},
			}},
			SbType: model.SmartBlockType_STRelationOption,
		}
		info := &useCaseInfo{
			customTypesAndRelations: map[string]customInfo{
				customRelKey: {id: "customRel1"},
			},
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateRelationOption(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("relation option for non-existent relation - skip", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String():          pbtypes.String("opt1"),
					bundle.RelationKeyRelationKey.String(): pbtypes.String("nonExistentRelation"),
				}},
			}},
			SbType: model.SmartBlockType_STRelationOption,
		}
		info := &useCaseInfo{customTypesAndRelations: make(map[string]customInfo)}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateRelationOption(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.True(t, skip)
	})
}

func TestValidateCollection(t *testing.T) {
	t.Run("no collection data - skip validation", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("obj1"),
			}},
		}}}
		info := &useCaseInfo{}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateCollection(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("all collection items exist", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("collection1"),
			}},
			Collections: &types.Struct{Fields: map[string]*types.Value{
				"objects": pbtypes.StringList([]string{"obj1", "obj2", "obj3"}),
			}},
		}}}
		info := &useCaseInfo{
			objects: map[string]objectInfo{
				"obj1": {},
				"obj2": {},
				"obj3": {},
			},
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateCollection(s, info, FixConfig{}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("some collection items missing - error without config", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("collection1"),
			}},
			Collections: &types.Struct{Fields: map[string]*types.Value{
				"objects": pbtypes.StringList([]string{"obj1", "missing1", "obj3"}),
			}},
		}}}
		info := &useCaseInfo{
			objects: map[string]objectInfo{
				"obj1": {},
				"obj3": {},
			},
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateCollection(s, info, FixConfig{}, reporter)

		// then
		assert.Error(t, err)
		assert.False(t, skip)
	})

	t.Run("some collection items missing - delete with config", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String("collection1"),
			}},
			Collections: &types.Struct{Fields: map[string]*types.Value{
				"objects": pbtypes.StringList([]string{"obj1", "missing1", "obj3", "missing2"}),
			}},
		}}}
		info := &useCaseInfo{
			objects: map[string]objectInfo{
				"obj1": {},
				"obj3": {},
			},
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		skip, err := validateCollection(s, info, FixConfig{DeleteInvalidCollectionItems: true}, reporter)

		// then
		assert.NoError(t, err)
		assert.False(t, skip)
		collectionItems := pbtypes.GetStringList(s.Snapshot.Data.Collections, "objects")
		assert.Equal(t, []string{"obj1", "obj3"}, collectionItems)
	})
}

func TestApplyPrimitives(t *testing.T) {
	t.Run("not a page or type - no changes", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String():     pbtypes.String("obj1"),
					bundle.RelationKeyLayout.String(): pbtypes.Int64(int64(model.ObjectType_basic)),
				}},
			}},
			SbType: model.SmartBlockType_File,
		}
		info := &useCaseInfo{}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		applyPrimitives(s, info, reporter)

		// then
		assert.NotNil(t, s.Snapshot.Data.Details.Fields[bundle.RelationKeyLayout.String()])
	})

	t.Run("page - removes layout and layoutAlign details", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String():          pbtypes.String("page1"),
					bundle.RelationKeyLayout.String():      pbtypes.Int64(int64(model.ObjectType_basic)),
					bundle.RelationKeyLayoutAlign.String(): pbtypes.Int64(1),
				}},
			}},
			SbType: model.SmartBlockType_Page,
		}
		info := &useCaseInfo{}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		applyPrimitives(s, info, reporter)

		// then
		assert.Nil(t, s.Snapshot.Data.Details.Fields[bundle.RelationKeyLayout.String()])
		assert.Nil(t, s.Snapshot.Data.Details.Fields[bundle.RelationKeyLayoutAlign.String()])
	})

	t.Run("page - keeps featured relations with description only", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String(): pbtypes.String("page1"),
					bundle.RelationKeyFeaturedRelations.String(): pbtypes.StringList([]string{
						bundle.RelationKeyDescription.String(),
						bundle.RelationKeyName.String(),
						bundle.RelationKeyTag.String(),
					}),
				}},
			}},
			SbType: model.SmartBlockType_Page,
		}
		info := &useCaseInfo{}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		applyPrimitives(s, info, reporter)

		// then
		featuredRels := pbtypes.GetStringList(s.Snapshot.Data.Details, bundle.RelationKeyFeaturedRelations.String())
		assert.Equal(t, []string{bundle.RelationKeyDescription.String()}, featuredRels)
	})

	t.Run("page - removes featured relations without description", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String(): pbtypes.String("page1"),
					bundle.RelationKeyFeaturedRelations.String(): pbtypes.StringList([]string{
						bundle.RelationKeyName.String(),
						bundle.RelationKeyTag.String(),
					}),
				}},
			}},
			SbType: model.SmartBlockType_Page,
		}
		info := &useCaseInfo{}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		applyPrimitives(s, info, reporter)

		// then
		assert.Nil(t, s.Snapshot.Data.Details.Fields[bundle.RelationKeyFeaturedRelations.String()])
	})

	t.Run("type - skip if already has recommended featured relations", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String(): pbtypes.String("type1"),
					bundle.RelationKeyRecommendedFeaturedRelations.String(): pbtypes.StringList([]string{
						bundle.RelationKeyName.String(),
					}),
					bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList([]string{
						bundle.RelationKeyDescription.String(),
					}),
				}},
			}},
			SbType: model.SmartBlockType_STType,
		}
		info := &useCaseInfo{
			relations: make(map[string]domain.RelationKey),
		}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		applyPrimitives(s, info, reporter)

		// then
		// Should not change anything since RecommendedFeaturedRelations already exists
		assert.NotNil(t, s.Snapshot.Data.Details.Fields[bundle.RelationKeyRecommendedRelations.String()])
	})

	t.Run("page - removes all layout details when none present", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String():   pbtypes.String("page1"),
					bundle.RelationKeyName.String(): pbtypes.String("Test Page"),
				}},
			}},
			SbType: model.SmartBlockType_Page,
		}
		info := &useCaseInfo{}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		applyPrimitives(s, info, reporter)

		// then
		assert.Nil(t, s.Snapshot.Data.Details.Fields[bundle.RelationKeyLayout.String()])
		assert.Nil(t, s.Snapshot.Data.Details.Fields[bundle.RelationKeyLayoutAlign.String()])
		assert.Nil(t, s.Snapshot.Data.Details.Fields[bundle.RelationKeyFeaturedRelations.String()])
		assert.Equal(t, "Test Page", pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyName.String()))
	})

	t.Run("page - nil featured relations list handled gracefully", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String():                pbtypes.String("page1"),
					bundle.RelationKeyFeaturedRelations.String(): pbtypes.Null(),
				}},
			}},
			SbType: model.SmartBlockType_Page,
		}
		info := &useCaseInfo{}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		applyPrimitives(s, info, reporter)

		// then
		assert.Nil(t, s.Snapshot.Data.Details.Fields[bundle.RelationKeyLayout.String()])
	})

	t.Run("page - empty featured relations list handled gracefully", func(t *testing.T) {
		// given
		s := &pb.SnapshotWithType{
			Snapshot: &pb.ChangeSnapshot{Data: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyId.String():                pbtypes.String("page1"),
					bundle.RelationKeyFeaturedRelations.String(): pbtypes.StringList([]string{}),
				}},
			}},
			SbType: model.SmartBlockType_Page,
		}
		info := &useCaseInfo{}
		reporter := &reporter{changes: make(map[string][]string)}

		// when
		applyPrimitives(s, info, reporter)

		// then
		assert.Nil(t, s.Snapshot.Data.Details.Fields[bundle.RelationKeyFeaturedRelations.String()])
	})
}
