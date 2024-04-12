package history

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder/mock_objecttreebuilder"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

// todo: reimplement

type historyStub struct {
	changes  []*objecttree.Change
	objectId string
}

func (h historyStub) Lock() {}

func (h historyStub) Unlock() {}
func (h historyStub) RLock()  {}

func (h historyStub) RUnlock() {}

func (h historyStub) TryRLock() bool { return false }

func (h historyStub) TryLock() bool { return false }

func (h historyStub) Id() string { return h.objectId }

func (h historyStub) Header() *treechangeproto.RawTreeChangeWithId {
	objectChange := &model.ObjectChangePayload{SmartBlockType: model.SmartBlockType_Page}
	objectChangeRaw, err := objectChange.Marshal()
	if err != nil {
		return nil
	}
	createChange := &treechangeproto.RootChange{
		ChangePayload: objectChangeRaw,
		ChangeType:    spacecore.ChangeType,
	}
	createChangeRaw, err := createChange.Marshal()
	if err != nil {
		return nil
	}
	rootChange := &treechangeproto.RawTreeChange{
		Payload: createChangeRaw,
	}
	rootChangeBytes, err := rootChange.Marshal()
	if err != nil {
		return nil
	}
	return &treechangeproto.RawTreeChangeWithId{
		Id:        h.objectId,
		RawChange: rootChangeBytes,
	}
}

func (h historyStub) UnmarshalledHeader() *objecttree.Change { return nil }

func (h historyStub) ChangeInfo() *treechangeproto.TreeChangeInfo {
	objectChangePayload := &model.ObjectChangePayload{SmartBlockType: model.SmartBlockType_Page}
	changePayload, _ := objectChangePayload.Marshal()
	return &treechangeproto.TreeChangeInfo{
		ChangePayload: changePayload,
	}
}

func (h historyStub) Heads() []string { return nil }

func (h historyStub) Root() *objecttree.Change {
	return &objecttree.Change{
		Id: h.objectId,
	}
}

func (h historyStub) Len() int { return 0 }

func (h historyStub) IsDerived() bool { return false }

func (h historyStub) AclList() list.AclList { return nil }

func (h historyStub) HasChanges(s ...string) bool { return false }

func (h historyStub) GetChange(s string) (*objecttree.Change, error) {
	for _, change := range h.changes {
		if change.Id == s {
			return change, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (h historyStub) Debug(parser objecttree.DescriptionParser) (objecttree.DebugInfo, error) {
	return objecttree.DebugInfo{}, nil
}

func (h historyStub) IterateRoot(convert objecttree.ChangeConvertFunc, iterate objecttree.ChangeIterateFunc) error {
	return nil
}

func (h historyStub) IterateFrom(id string, convert objecttree.ChangeConvertFunc, iterate objecttree.ChangeIterateFunc) error {
	for _, change := range h.changes {
		if !iterate(change) {
			return nil
		}
	}
	return nil
}

func TestHistory_GetBlocksModifiers(t *testing.T) {
	objectId := "objectId"
	spaceID := "spaceId"
	versionId := "versionId"
	blockId := "blockId"
	blockDivId := "blockDivID"
	blockLinkId := "blockLinkId"
	blockLatexId := "blockLatexId"
	blockFileId := "blockFileId"
	blockBookmarkId := "blockBookmarkId"
	blockRelationId := "blockRelationId"

	t.Run("object without blocks", func(t *testing.T) {
		// given
		history := newFixture(t, nil, objectId, spaceID, versionId)

		// when
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, nil)

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 0)
	})
	t.Run("object with 1 created block", func(t *testing.T) {
		// given
		keys, _ := accountdata.NewRandom()
		participantId := domain.NewParticipantId(spaceID, keys.SignKey.GetPublic().Account())
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		expectedChanges := []*objecttree.Change{provideBlockCreateChange(bl, keys.SignKey.GetPublic())}
		history := newFixture(t, expectedChanges, objectId, spaceID, versionId)

		// when
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 1)
		assert.Equal(t, bl.Id, blocksModifiers[0].BlockId)
		assert.Equal(t, participantId, blocksModifiers[0].ParticipantId)
	})
	t.Run("object with 1 modified block", func(t *testing.T) {
		// given
		keys, _ := accountdata.NewRandom()
		participantId := domain.NewParticipantId(spaceID, keys.SignKey.GetPublic().Account())
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		expectedChanges := []*objecttree.Change{
			provideBlockCreateChange(bl, keys.SignKey.GetPublic()),
			provideBlockSetTextChange(bl, keys.SignKey.GetPublic()),
		}
		history := newFixture(t, expectedChanges, objectId, spaceID, versionId)

		// when
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 1)
		assert.Equal(t, bl.Id, blocksModifiers[0].BlockId)
		assert.Equal(t, participantId, blocksModifiers[0].ParticipantId)
	})
	t.Run("object with simple blocks changes by 1 participant", func(t *testing.T) {
		// given
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		blDiv := &model.Block{Id: blockDivId, Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}
		blLink := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
		blLatex := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
		blFile := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
		blBookmark := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
		blRelation := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}

		keys, _ := accountdata.NewRandom()
		account := keys.SignKey.GetPublic()
		expectedChanges := []*objecttree.Change{
			// create block changes
			provideBlockCreateChange(bl, account),
			provideBlockCreateChange(blDiv, account),
			provideBlockCreateChange(blLink, account),
			provideBlockCreateChange(blLatex, account),
			provideBlockCreateChange(blFile, account),
			provideBlockCreateChange(blBookmark, account),
			provideBlockSetTextChange(blRelation, account),

			// update block changes
			provideBlockSetTextChange(bl, account),
			provideBlockSetDivChange(blDiv, account),
			provideBlockSetLinkChange(blLink, account),
			provideBlockSetLatexChange(blLatex, account),
			provideBlockSetFileChange(blFile, account),
			provideBlockSetBookmarkChange(blBookmark, account),
			provideBlockSetRelationChange(blRelation, account),
		}
		history := newFixture(t, expectedChanges, objectId, spaceID, versionId)

		// when
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl, blBookmark, blLatex, blDiv, blFile, blLink, blRelation})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 7)
	})
	t.Run("object with modified blocks changes by 1 participant", func(t *testing.T) {
		// given
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		blDiv := &model.Block{Id: blockDivId, Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}
		blLink := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
		blLatex := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
		blFile := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
		blBookmark := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
		blRelation := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}
		keys, _ := accountdata.NewRandom()

		account := keys.SignKey.GetPublic()
		expectedChanges := []*objecttree.Change{
			// update block changes
			provideBlockSetVerticalAlignChange(bl, account),
			provideBlockSetAlignChange(blDiv, account),
			provideBlockSetChildrenIdsChange(blBookmark, account),
			provideBlockBackgroundColorChange(blRelation, account),
			provideBlockFieldChange(blLink, account),
		}

		// when
		history := newFixture(t, expectedChanges, objectId, spaceID, versionId)
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl, blBookmark, blLatex, blDiv, blFile, blLink, blRelation})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 5)
	})
	t.Run("object with moved block changes by 1 participant", func(t *testing.T) {
		// given
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		keys, _ := accountdata.NewRandom()
		account := keys.SignKey.GetPublic()
		expectedChanges := []*objecttree.Change{
			provideBlockCreateChange(bl, account),
			provideBlockMoveChange(bl, account),
		}

		// when
		history := newFixture(t, expectedChanges, objectId, spaceID, versionId)
		participantId := domain.NewParticipantId(spaceID, keys.SignKey.GetPublic().Account())
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 1)
		assert.Equal(t, bl.Id, blocksModifiers[0].BlockId)
		assert.Equal(t, participantId, blocksModifiers[0].ParticipantId)
	})
	t.Run("object block was deleted, don't add it in response", func(t *testing.T) {
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		keys, _ := accountdata.NewRandom()
		account := keys.SignKey.GetPublic()
		expectedChanges := []*objecttree.Change{
			provideBlockCreateChange(bl, account),
			provideBlockRemoveChange(blockBookmarkId, account),
		}

		// when
		history := newFixture(t, expectedChanges, objectId, spaceID, versionId)
		participantId := domain.NewParticipantId(spaceID, keys.SignKey.GetPublic().Account())
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 1)
		assert.Equal(t, bl.Id, blocksModifiers[0].BlockId)
		assert.Equal(t, participantId, blocksModifiers[0].ParticipantId)
	})
	t.Run("object with block changes by 2 participants", func(t *testing.T) {
		// given
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		blBookmark := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
		blLatex := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
		blRelation := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}

		firstAccountKeys, _ := accountdata.NewRandom()
		secondAccountKeys, _ := accountdata.NewRandom()
		firstAccount := firstAccountKeys.SignKey.GetPublic()
		secondAccount := secondAccountKeys.SignKey.GetPublic()
		firstParticipantId := domain.NewParticipantId(spaceID, firstAccount.Account())
		secondParticipantId := domain.NewParticipantId(spaceID, secondAccountKeys.SignKey.GetPublic().Account())

		expectedChanges := []*objecttree.Change{
			provideBlockCreateChange(bl, firstAccount),
			provideBlockSetTextChange(bl, secondAccount),
			provideBlockCreateChange(blBookmark, firstAccount),
			provideBlockSetVerticalAlignChange(blLatex, secondAccount),
			provideBlockAddChange(blRelation, secondAccount),
		}
		history := newFixture(t, expectedChanges, objectId, spaceID, versionId)

		// when
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl, blBookmark, blLatex, blRelation})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 4)
		assert.Contains(t, blocksModifiers, &model.ObjectViewBlockModifier{
			BlockId:       bl.Id,
			ParticipantId: secondParticipantId,
		})
		assert.Contains(t, blocksModifiers, &model.ObjectViewBlockModifier{
			BlockId:       blBookmark.Id,
			ParticipantId: firstParticipantId,
		})
		assert.Contains(t, blocksModifiers, &model.ObjectViewBlockModifier{
			BlockId:       blLatex.Id,
			ParticipantId: secondParticipantId,
		})
		assert.Contains(t, blocksModifiers, &model.ObjectViewBlockModifier{
			BlockId:       blRelation.Id,
			ParticipantId: secondParticipantId,
		})
	})
}

func TestHistory_DiffVersions(t *testing.T) {
	objectId := "objectId"
	spaceID := "spaceId"

	versionId := "versionId"
	previousVersion := "previousVersion"
	blockId := "blockId"
	// blockDivId := "blockDivID"
	// blockLinkId := "blockLinkId"
	// blockLatexId := "blockLatexId"
	// blockFileId := "blockFileId"
	// blockBookmarkId := "blockBookmarkId"
	// blockRelationId := "blockRelationId"

	bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
	blSmartBlock := &model.Block{Id: objectId, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}
	// blDiv := &model.Block{Id: blockDivId, Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}
	// blLink := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
	// blLatex := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
	// blFile := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
	// blBookmark := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
	// blRelation := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}

	t.Run("object diff - new created block", func(t *testing.T) {
		// given
		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()
		currChange := []*objecttree.Change{
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(bl, account),
		}
		prevChanges := []*objecttree.Change{
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
		}
		history := newFixtureDiffVersions(t, currChange, prevChanges, objectId, spaceID, versionId, previousVersion)

		// when
		changes, _, err := history.DiffVersions(&pb.RpcHistoryDiffVersionsRequest{
			ObjectId:        objectId,
			SpaceId:         spaceID,
			CurrentVersion:  versionId,
			PreviousVersion: previousVersion,
		})

		// then
		assert.Nil(t, err)
		assert.Len(t, changes, 1)

		assert.NotNil(t, changes[0].GetBlockAdd())
		assert.Len(t, changes[0].GetBlockAdd().Blocks, 1)
		assert.Equal(t, bl, changes[0].GetBlockAdd().Blocks[0])
	})

	// t.Run("object diff - simple block changes", func(t *testing.T) {
	// 	spaceService := mock_space.NewMockService(t)
	// 	space := mock_clientspace.NewMockSpace(t)
	// 	treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
	// 	accountKeys, _ := accountdata.NewRandom()
	//
	// 	blDivCopy := &model.Block{Id: blockDivId, Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}
	// 	blLinkCopy := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
	// 	blLatexCopy := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
	// 	blFileCopy := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
	// 	blBookmarkCopy := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
	// 	blRelationCopy := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}
	// 	blCopy := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
	//
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: versionId,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blDivCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blLatexCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blLinkCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blBookmarkCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blRelationCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blDivCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blFileCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetFile{
	// 												BlockSetFile: &pb.EventBlockSetFile{
	// 													Id:   blFile.Id,
	// 													Name: &pb.EventBlockSetFileName{Value: "new file name"},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetRelation{
	// 												BlockSetRelation: &pb.EventBlockSetRelation{
	// 													Id:  blRelation.Id,
	// 													Key: &pb.EventBlockSetRelationKey{Value: "new key"},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetBookmark{
	// 												BlockSetBookmark: &pb.EventBlockSetBookmark{
	// 													Id:  blBookmark.Id,
	// 													Url: &pb.EventBlockSetBookmarkUrl{Value: "new url"},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetLatex{
	// 												BlockSetLatex: &pb.EventBlockSetLatex{
	// 													Id:   blLatex.Id,
	// 													Text: &pb.EventBlockSetLatexText{Value: "new latex text"},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetLink{
	// 												BlockSetLink: &pb.EventBlockSetLink{
	// 													Id:       blLink.Id,
	// 													IconSize: &pb.EventBlockSetLinkIconSize{Value: model.BlockContentLink_SizeSmall},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetText{
	// 												BlockSetText: &pb.EventBlockSetText{
	// 													Id: blockId,
	// 													Text: &pb.EventBlockSetTextText{
	// 														Value: "new text",
	// 													},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetDiv{
	// 												BlockSetDiv: &pb.EventBlockSetDiv{
	// 													Id:    blockDivId,
	// 													Style: &pb.EventBlockSetDivStyle{Value: model.BlockContentDiv_Dots},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: previousVersion,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{bl},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blLatex},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blLink},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blBookmark},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blRelation},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blDiv},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blFile},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	//
	// 	space.EXPECT().TreeBuilder().Return(treeBuilder)
	// 	spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
	// 	history := history{
	// 		spaceService: spaceService,
	// 		objectStore:  objectstore.NewStoreFixture(t),
	// 	}
	// 	changes, _, err := history.DiffVersions(&pb.RpcHistoryDiffVersionsRequest{
	// 		ObjectId:        objectId,
	// 		SpaceId:         spaceID,
	// 		CurrentVersion:  versionId,
	// 		PreviousVersion: previousVersion,
	// 	})
	//
	// 	assert.Nil(t, err)
	// 	assert.Len(t, changes, 8)
	//
	// 	assert.NotNil(t, changes[0].GetBlockSetChildrenIds())
	// 	assert.Equal(t, objectId, changes[0].GetBlockSetChildrenIds().Id)
	//
	// 	assert.NotNil(t, changes[1].GetBlockSetLatex())
	// 	assert.NotNil(t, changes[2].GetBlockSetLink())
	// 	assert.NotNil(t, changes[3].GetBlockSetBookmark())
	// 	assert.NotNil(t, changes[4].GetBlockSetRelation())
	// 	assert.NotNil(t, changes[5].GetBlockSetDiv())
	// 	assert.NotNil(t, changes[6].GetBlockSetFile())
	// 	assert.NotNil(t, changes[7].GetBlockSetText())
	// })

	// t.Run("object diff - block properties changes", func(t *testing.T) {
	// 	spaceService := mock_space.NewMockService(t)
	// 	space := mock_clientspace.NewMockSpace(t)
	// 	treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
	// 	accountKeys, _ := accountdata.NewRandom()
	//
	// 	blFileCopy := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
	// 	blBookmarkCopy := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
	// 	blRelationCopy := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}
	// 	blLinkCopy := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
	//
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: versionId,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blBookmarkCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blRelationCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blFileCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blLinkCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetAlign{
	// 												BlockSetAlign: &pb.EventBlockSetAlign{
	// 													Id:    blFile.Id,
	// 													Align: model.Block_AlignCenter,
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetBackgroundColor{
	// 												BlockSetBackgroundColor: &pb.EventBlockSetBackgroundColor{
	// 													Id:              blRelation.Id,
	// 													BackgroundColor: "gray",
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetFields{
	// 												BlockSetFields: &pb.EventBlockSetFields{
	// 													Id: blBookmark.Id,
	// 													Fields: &types.Struct{
	// 														Fields: map[string]*types.Value{"key": pbtypes.String("value")},
	// 													},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetVerticalAlign{
	// 												BlockSetVerticalAlign: &pb.EventBlockSetVerticalAlign{
	// 													Id:            blLink.Id,
	// 													VerticalAlign: model.Block_VerticalAlignBottom,
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: previousVersion,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blBookmark},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blRelation},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blFile},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blLink},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	//
	// 	space.EXPECT().TreeBuilder().Return(treeBuilder)
	// 	spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
	// 	history := history{
	// 		spaceService: spaceService,
	// 		objectStore:  objectstore.NewStoreFixture(t),
	// 	}
	// 	changes, _, err := history.DiffVersions(&pb.RpcHistoryDiffVersionsRequest{
	// 		ObjectId:        objectId,
	// 		SpaceId:         spaceID,
	// 		CurrentVersion:  versionId,
	// 		PreviousVersion: previousVersion,
	// 	})
	//
	// 	assert.Nil(t, err)
	// 	assert.Len(t, changes, 4)
	//
	// 	assert.NotNil(t, changes[0].GetBlockSetFields())
	// 	assert.NotNil(t, changes[1].GetBlockSetBackgroundColor())
	// 	assert.NotNil(t, changes[2].GetBlockSetAlign())
	// 	assert.NotNil(t, changes[3].GetBlockSetVerticalAlign())
	// })

	// t.Run("object diff - block change and timestamp change, return only block change", func(t *testing.T) {
	// 	spaceService := mock_space.NewMockService(t)
	// 	space := mock_clientspace.NewMockSpace(t)
	// 	treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
	// 	accountKeys, _ := accountdata.NewRandom()
	// 	blLinkCopy := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
	//
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: versionId,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blLinkCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfOriginalCreatedTimestampSet{
	// 								OriginalCreatedTimestampSet: &pb.ChangeOriginalCreatedTimestampSet{Ts: 1},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockSetVerticalAlign{
	// 												BlockSetVerticalAlign: &pb.EventBlockSetVerticalAlign{
	// 													Id:            blLink.Id,
	// 													VerticalAlign: model.Block_VerticalAlignBottom,
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: previousVersion,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blLink},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	//
	// 	space.EXPECT().TreeBuilder().Return(treeBuilder)
	// 	spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
	// 	history := history{
	// 		spaceService: spaceService,
	// 		objectStore:  objectstore.NewStoreFixture(t),
	// 	}
	// 	changes, _, err := history.DiffVersions(&pb.RpcHistoryDiffVersionsRequest{
	// 		ObjectId:        objectId,
	// 		SpaceId:         spaceID,
	// 		CurrentVersion:  versionId,
	// 		PreviousVersion: previousVersion,
	// 	})
	//
	// 	assert.Nil(t, err)
	// 	assert.Len(t, changes, 1)
	//
	// 	assert.NotNil(t, changes[0].GetBlockSetVerticalAlign())
	// })
	//
	// t.Run("object diff - dataview changes", func(t *testing.T) {
	// 	spaceService := mock_space.NewMockService(t)
	// 	space := mock_clientspace.NewMockSpace(t)
	// 	treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
	// 	accountKeys, _ := accountdata.NewRandom()
	// 	blDataviewId := "blDataviewId"
	// 	blDataview := &model.Block{
	// 		Id: blDataviewId,
	// 		Content: &model.BlockContentOfDataview{
	// 			Dataview: &model.BlockContentDataview{
	// 				Views: []*model.BlockContentDataviewView{
	// 					{
	// 						Id:   "viewId1",
	// 						Name: "view1",
	// 						Sorts: []*model.BlockContentDataviewSort{
	// 							{
	// 								RelationKey: "key",
	// 							},
	// 						},
	// 					},
	// 				},
	// 				RelationLinks: []*model.RelationLink{
	// 					{
	// 						Key: "key",
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}
	//
	// 	blDataviewCopy := &model.Block{
	// 		Id: blDataviewId,
	// 		Content: &model.BlockContentOfDataview{
	// 			Dataview: &model.BlockContentDataview{
	// 				Views: []*model.BlockContentDataviewView{
	// 					{
	// 						Id:   "viewId1",
	// 						Name: "view1",
	// 						Sorts: []*model.BlockContentDataviewSort{
	// 							{
	// 								RelationKey: "key",
	// 							},
	// 						},
	// 					},
	// 				},
	// 				RelationLinks: []*model.RelationLink{
	// 					{
	// 						Key: "key",
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}
	//
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: versionId,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blDataviewCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataviewViewSet{
	// 												BlockDataviewViewSet: &pb.EventBlockDataviewViewSet{
	// 													Id:     blDataviewId,
	// 													ViewId: "viewId1",
	// 													View: &model.BlockContentDataviewView{
	// 														Id:   "viewId1",
	// 														Name: "view1",
	// 														Sorts: []*model.BlockContentDataviewSort{
	// 															{
	// 																RelationKey: "key",
	// 															},
	// 														},
	// 													},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataviewViewSet{
	// 												BlockDataviewViewSet: &pb.EventBlockDataviewViewSet{
	// 													Id:     blDataviewId,
	// 													ViewId: "viewId2",
	// 													View: &model.BlockContentDataviewView{
	// 														Id:   "viewId2",
	// 														Name: "view2",
	// 														Sorts: []*model.BlockContentDataviewSort{
	// 															{
	// 																RelationKey: "key",
	// 															},
	// 														},
	// 													},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataviewSourceSet{
	// 												BlockDataviewSourceSet: &pb.EventBlockDataviewSourceSet{
	// 													Id:     blDataviewId,
	// 													Source: []string{"source"},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataviewRelationSet{
	// 												BlockDataviewRelationSet: &pb.EventBlockDataviewRelationSet{
	// 													Id:            blDataviewId,
	// 													RelationLinks: []*model.RelationLink{{Key: "key1"}},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataviewViewSet{
	// 												BlockDataviewViewSet: &pb.EventBlockDataviewViewSet{
	// 													Id:     blDataviewId,
	// 													ViewId: "viewId",
	// 													View: &model.BlockContentDataviewView{
	// 														Id:   "viewId",
	// 														Name: "view",
	// 														Sorts: []*model.BlockContentDataviewSort{
	// 															{
	// 																RelationKey: "key",
	// 															},
	// 														},
	// 													},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataviewViewOrder{
	// 												BlockDataviewViewOrder: &pb.EventBlockDataviewViewOrder{
	// 													Id:      blDataviewId,
	// 													ViewIds: []string{"viewId", "viewId1"},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataviewViewDelete{
	// 												BlockDataviewViewDelete: &pb.EventBlockDataviewViewDelete{
	// 													Id:     blDataviewId,
	// 													ViewId: "viewId2",
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataviewRelationDelete{
	// 												BlockDataviewRelationDelete: &pb.EventBlockDataviewRelationDelete{
	// 													Id:           blDataviewId,
	// 													RelationKeys: []string{"key"},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataViewObjectOrderUpdate{
	// 												BlockDataViewObjectOrderUpdate: &pb.EventBlockDataviewObjectOrderUpdate{
	// 													Id:     blDataviewId,
	// 													ViewId: "viewId",
	// 													SliceChanges: []*pb.EventBlockDataviewSliceChange{
	// 														{
	// 															Op:      pb.EventBlockDataview_SliceOperationNone,
	// 															Ids:     []string{"objectId"},
	// 															AfterId: "objectId1",
	// 														},
	// 													},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataViewGroupOrderUpdate{
	// 												BlockDataViewGroupOrderUpdate: &pb.EventBlockDataviewGroupOrderUpdate{
	// 													Id: blDataviewId,
	// 													GroupOrder: &model.BlockContentDataviewGroupOrder{
	// 														ViewId: "viewId",
	// 														ViewGroups: []*model.BlockContentDataviewViewGroup{
	// 															{
	// 																GroupId:         "group1",
	// 																BackgroundColor: "pink",
	// 															},
	// 															{
	// 																GroupId:         "group3",
	// 																BackgroundColor: "blue",
	// 															},
	// 														},
	// 													},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataviewViewUpdate{
	// 												BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
	// 													Id:     blDataviewId,
	// 													ViewId: "viewId",
	// 													Filter: []*pb.EventBlockDataviewViewUpdateFilter{
	// 														{
	// 															Operation: &pb.EventBlockDataviewViewUpdateFilterOperationOfAdd{
	// 																Add: &pb.EventBlockDataviewViewUpdateFilterAdd{
	// 																	AfterId: "",
	// 																	Items: []*model.BlockContentDataviewFilter{
	// 																		{
	// 																			Id:          "filterId",
	// 																			Operator:    model.BlockContentDataviewFilter_Or,
	// 																			RelationKey: "key1",
	// 																			Value:       pbtypes.String("value"),
	// 																		},
	// 																	},
	// 																},
	// 															},
	// 														},
	// 													},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	//
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataviewTargetObjectIdSet{
	// 												BlockDataviewTargetObjectIdSet: &pb.EventBlockDataviewTargetObjectIdSet{
	// 													Id:             blDataviewId,
	// 													TargetObjectId: "targetObjectId",
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: previousVersion,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blDataview},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockUpdate{
	// 								BlockUpdate: &pb.ChangeBlockUpdate{
	// 									Events: []*pb.EventMessage{
	// 										{
	// 											Value: &pb.EventMessageValueOfBlockDataviewViewSet{
	// 												BlockDataviewViewSet: &pb.EventBlockDataviewViewSet{
	// 													Id:     blDataviewId,
	// 													ViewId: "viewId2",
	// 													View: &model.BlockContentDataviewView{
	// 														Id:   "viewId2",
	// 														Name: "view2",
	// 														Sorts: []*model.BlockContentDataviewSort{
	// 															{
	// 																RelationKey: "key",
	// 															},
	// 														},
	// 													},
	// 												},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	//
	// 	space.EXPECT().TreeBuilder().Return(treeBuilder)
	// 	spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
	// 	history := history{
	// 		spaceService: spaceService,
	// 		objectStore:  objectstore.NewStoreFixture(t),
	// 	}
	// 	changes, _, err := history.DiffVersions(&pb.RpcHistoryDiffVersionsRequest{
	// 		ObjectId:        objectId,
	// 		SpaceId:         spaceID,
	// 		CurrentVersion:  versionId,
	// 		PreviousVersion: previousVersion,
	// 	})
	//
	// 	assert.Nil(t, err)
	// 	assert.Len(t, changes, 10)
	// })
	// t.Run("object diff - relations and details changes", func(t *testing.T) {
	// 	spaceService := mock_space.NewMockService(t)
	// 	space := mock_clientspace.NewMockSpace(t)
	// 	treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
	// 	accountKeys, _ := accountdata.NewRandom()
	//
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: versionId,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfRelationAdd{
	// 								RelationAdd: &pb.ChangeRelationAdd{
	// 									RelationLinks: []*model.RelationLink{
	// 										{
	// 											Key:    "key",
	// 											Format: model.RelationFormat_tag,
	// 										},
	// 										{
	// 											Key:    "key1",
	// 											Format: model.RelationFormat_longtext,
	// 										},
	// 										{
	// 											Key:    "key2",
	// 											Format: model.RelationFormat_longtext,
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfDetailsSet{
	// 								DetailsSet: &pb.ChangeDetailsSet{
	// 									Key:   "key",
	// 									Value: pbtypes.String("value"),
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfRelationRemove{
	// 								RelationRemove: &pb.ChangeRelationRemove{
	// 									RelationKey: []string{"key1"},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfDetailsUnset{
	// 								DetailsUnset: &pb.ChangeDetailsUnset{
	// 									Key: "key2",
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfDetailsSet{
	// 								DetailsSet: &pb.ChangeDetailsSet{
	// 									Key:   "key",
	// 									Value: pbtypes.String("value1"),
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfRelationAdd{
	// 								RelationAdd: &pb.ChangeRelationAdd{
	// 									RelationLinks: []*model.RelationLink{
	// 										{
	// 											Key:    "key3",
	// 											Format: model.RelationFormat_tag,
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: previousVersion,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfRelationAdd{
	// 								RelationAdd: &pb.ChangeRelationAdd{
	// 									RelationLinks: []*model.RelationLink{
	// 										{
	// 											Key:    "key",
	// 											Format: model.RelationFormat_tag,
	// 										},
	// 										{
	// 											Key:    "key1",
	// 											Format: model.RelationFormat_longtext,
	// 										},
	// 										{
	// 											Key:    "key2",
	// 											Format: model.RelationFormat_longtext,
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfDetailsSet{
	// 								DetailsSet: &pb.ChangeDetailsSet{
	// 									Key:   "key2",
	// 									Value: pbtypes.String("value"),
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	//
	// 	space.EXPECT().TreeBuilder().Return(treeBuilder)
	// 	spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
	// 	history := history{
	// 		spaceService: spaceService,
	// 		objectStore:  objectstore.NewStoreFixture(t),
	// 	}
	// 	changes, _, err := history.DiffVersions(&pb.RpcHistoryDiffVersionsRequest{
	// 		ObjectId:        objectId,
	// 		SpaceId:         spaceID,
	// 		CurrentVersion:  versionId,
	// 		PreviousVersion: previousVersion,
	// 	})
	//
	// 	assert.Nil(t, err)
	// 	assert.Len(t, changes, 4)
	// })

	// t.Run("object diff - no changes", func(t *testing.T) {
	//
	// 	accountKeys, _ := accountdata.NewRandom()
	//
	// 	blFileCopy := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
	// 	blBookmarkCopy := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
	// 	blRelationCopy := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}
	// 	blLinkCopy := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
	//
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: versionId,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blBookmarkCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blRelationCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blFileCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blLinkCopy},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	// 	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
	// 		BeforeId: previousVersion,
	// 		Include:  true,
	// 	}).Return(&historyStub{
	// 		objectId: objectId,
	// 		changes: []*objecttree.Change{
	// 			{
	// 				Id:       objectId,
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model:    &pb.Change{},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{
	// 										{
	// 											Id: objectId,
	// 											Content: &model.BlockContentOfSmartblock{
	// 												Smartblock: &model.BlockContentSmartblock{},
	// 											},
	// 										},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blBookmark},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blRelation},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blFile},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 			{
	// 				Identity: accountKeys.SignKey.GetPublic(),
	// 				Model: &pb.Change{
	// 					Content: []*pb.ChangeContent{
	// 						{
	// 							Value: &pb.ChangeContentValueOfBlockCreate{
	// 								BlockCreate: &pb.ChangeBlockCreate{
	// 									Blocks: []*model.Block{blLink},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	}, nil)
	//
	// 	space.EXPECT().TreeBuilder().Return(treeBuilder)
	// 	spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
	// 	history := history{
	// 		spaceService: spaceService,
	// 		objectStore:  objectstore.NewStoreFixture(t),
	// 	}
	// 	changes, _, err := history.DiffVersions(&pb.RpcHistoryDiffVersionsRequest{
	// 		ObjectId:        objectId,
	// 		SpaceId:         spaceID,
	// 		CurrentVersion:  versionId,
	// 		PreviousVersion: previousVersion,
	// 	})
	//
	// 	assert.Nil(t, err)
	// 	assert.Len(t, changes, 4)
	// })
}

func provideBlockEmptyChange(objectId string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Id:       objectId,
		Identity: account,
		Model:    &pb.Change{},
	}
}

type historyFixture struct {
	*history
	space       *mock_clientspace.MockSpace
	treeBuilder *mock_objecttreebuilder.MockTreeBuilder
}

func newFixture(t *testing.T, expectedChanges []*objecttree.Change, objectId, spaceID, versionId string) *historyFixture {
	spaceService := mock_space.NewMockService(t)
	space := mock_clientspace.NewMockSpace(t)
	ctrl := gomock.NewController(t)
	treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)

	if len(expectedChanges) > 0 {
		treeBuilder = configureTreeBuilder(treeBuilder, objectId, versionId, spaceID, expectedChanges, space, spaceService)
		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: versionId,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes:  expectedChanges,
		}, nil)
		space.EXPECT().TreeBuilder().Return(treeBuilder)
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
	}
	history := &history{
		objectStore:  objectstore.NewStoreFixture(t),
		spaceService: spaceService,
	}
	return &historyFixture{
		history:     history,
		space:       space,
		treeBuilder: treeBuilder,
	}
}

func newFixtureDiffVersions(t *testing.T,
	currChanges, prevChanges []*objecttree.Change,
	objectId, spaceID, currVersionId, prevVersionId string,
) *historyFixture {
	spaceService := mock_space.NewMockService(t)
	space := mock_clientspace.NewMockSpace(t)
	ctrl := gomock.NewController(t)
	treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
	if len(currChanges) > 0 {
		configureTreeBuilder(treeBuilder, objectId, currVersionId, spaceID, currChanges, space, spaceService)
	}
	if len(prevChanges) > 0 {
		configureTreeBuilder(treeBuilder, objectId, prevVersionId, spaceID, prevChanges, space, spaceService)
	}
	history := &history{
		objectStore:  objectstore.NewStoreFixture(t),
		spaceService: spaceService,
	}
	return &historyFixture{
		history:     history,
		space:       space,
		treeBuilder: treeBuilder,
	}
}

func configureTreeBuilder(treeBuilder *mock_objecttreebuilder.MockTreeBuilder, objectId, currVersionId, spaceID string, expectedChanges []*objecttree.Change, space *mock_clientspace.MockSpace, spaceService *mock_space.MockService) *mock_objecttreebuilder.MockTreeBuilder {
	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
		BeforeId: currVersionId,
		Include:  true,
	}).Return(&historyStub{
		objectId: objectId,
		changes:  expectedChanges,
	}, nil)
	space.EXPECT().TreeBuilder().Return(treeBuilder)
	spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
	return treeBuilder
}

func provideBlockCreateChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockCreate{
						BlockCreate: &pb.ChangeBlockCreate{
							Blocks: []*model.Block{block},
						},
					},
				},
			},
		},
	}
}

func provideBlockSetTextChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetText{
										BlockSetText: &pb.EventBlockSetText{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockSetDivChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetDiv{
										BlockSetDiv: &pb.EventBlockSetDiv{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockSetLinkChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetLink{
										BlockSetLink: &pb.EventBlockSetLink{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockSetLatexChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetLatex{
										BlockSetLatex: &pb.EventBlockSetLatex{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockSetFileChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetFile{
										BlockSetFile: &pb.EventBlockSetFile{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockSetBookmarkChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetBookmark{
										BlockSetBookmark: &pb.EventBlockSetBookmark{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockSetRelationChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetRelation{
										BlockSetRelation: &pb.EventBlockSetRelation{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockSetVerticalAlignChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetVerticalAlign{
										BlockSetVerticalAlign: &pb.EventBlockSetVerticalAlign{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockSetAlignChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetAlign{
										BlockSetAlign: &pb.EventBlockSetAlign{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockSetChildrenIdsChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetChildrenIds{
										BlockSetChildrenIds: &pb.EventBlockSetChildrenIds{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockBackgroundColorChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetBackgroundColor{
										BlockSetBackgroundColor: &pb.EventBlockSetBackgroundColor{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockFieldChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetFields{
										BlockSetFields: &pb.EventBlockSetFields{
											Id: block.Id,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func provideBlockMoveChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockMove{
						BlockMove: &pb.ChangeBlockMove{
							Ids: []string{block.Id},
						},
					},
				},
			},
		},
	}
}

func provideBlockRemoveChange(blockId string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockRemove{
						BlockRemove: &pb.ChangeBlockRemove{
							Ids: []string{blockId},
						},
					},
				},
			},
		},
	}
}

func provideBlockAddChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockAdd{
										BlockAdd: &pb.EventBlockAdd{
											Blocks: []*model.Block{block},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
