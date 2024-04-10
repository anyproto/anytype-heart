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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	versionId := "versionId"
	blockId := "blockId"
	blockDivId := "blockDivID"
	blockLinkId := "blockLinkId"
	blockLatexId := "blockLatexId"
	blockFileId := "blockFileId"
	blockBookmarkId := "blockBookmarkId"
	blockRelationId := "blockRelationId"

	t.Run("object without blocks", func(t *testing.T) {
		history := New()
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, nil)
		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 0)
	})
	t.Run("object with 1 created block", func(t *testing.T) {
		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		keys, _ := accountdata.NewRandom()
		participantId := domain.NewParticipantId(spaceID, keys.SignKey.GetPublic().Account())
		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: versionId,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes: []*objecttree.Change{
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{bl},
									},
								},
							},
						},
					},
				},
			},
		}, nil)
		space.EXPECT().TreeBuilder().Return(treeBuilder)
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
		history := history{
			spaceService: spaceService,
		}
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl})

		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 1)
		assert.Equal(t, bl.Id, blocksModifiers[0].BlockId)
		assert.Equal(t, participantId, blocksModifiers[0].ParticipantId)
	})
	t.Run("object with 1 modified block", func(t *testing.T) {
		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		keys, _ := accountdata.NewRandom()
		participantId := domain.NewParticipantId(spaceID, keys.SignKey.GetPublic().Account())
		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: versionId,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes: []*objecttree.Change{
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{bl},
									},
								},
							},
						},
					},
				},
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetText{
													BlockSetText: &pb.EventBlockSetText{
														Id: bl.Id,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, nil)
		space.EXPECT().TreeBuilder().Return(treeBuilder)
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
		history := history{
			spaceService: spaceService,
		}
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl})

		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 1)
		assert.Equal(t, bl.Id, blocksModifiers[0].BlockId)
		assert.Equal(t, participantId, blocksModifiers[0].ParticipantId)
	})
	t.Run("object with simple blocks changes by 1 participant", func(t *testing.T) {
		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		blDiv := &model.Block{Id: blockDivId, Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}
		blLink := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
		blLatex := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
		blFile := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
		blBookmark := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
		blRelation := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}

		keys, _ := accountdata.NewRandom()
		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: versionId,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes: []*objecttree.Change{
				// text block
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{bl},
									},
								},
							},
						},
					},
				},
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetText{
													BlockSetText: &pb.EventBlockSetText{
														Id: bl.Id,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},

				// div block
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blDiv},
									},
								},
							},
						},
					},
				},
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetDiv{
													BlockSetDiv: &pb.EventBlockSetDiv{
														Id: blDiv.Id,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				// link block
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blLink},
									},
								},
							},
						},
					},
				},
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetLink{
													BlockSetLink: &pb.EventBlockSetLink{
														Id: blLink.Id,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},

				// latex block
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blLatex},
									},
								},
							},
						},
					},
				},
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetLatex{
													BlockSetLatex: &pb.EventBlockSetLatex{
														Id: blLatex.Id,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},

				// file block
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blFile},
									},
								},
							},
						},
					},
				},
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetFile{
													BlockSetFile: &pb.EventBlockSetFile{
														Id: blFile.Id,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},

				// bookmark block
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blBookmark},
									},
								},
							},
						},
					},
				},
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetBookmark{
													BlockSetBookmark: &pb.EventBlockSetBookmark{
														Id: blBookmark.Id,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},

				// relation block
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blRelation},
									},
								},
							},
						},
					},
				},
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetRelation{
													BlockSetRelation: &pb.EventBlockSetRelation{
														Id: blRelation.Id,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, nil)
		space.EXPECT().TreeBuilder().Return(treeBuilder)
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
		history := history{
			spaceService: spaceService,
		}
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl, blBookmark, blLatex, blDiv, blFile, blLink, blRelation})

		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 7)
	})
	t.Run("object with modified blocks changes by 1 participant", func(t *testing.T) {
		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		blDiv := &model.Block{Id: blockDivId, Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}
		blLink := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
		blLatex := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
		blFile := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
		blBookmark := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
		blRelation := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}
		keys, _ := accountdata.NewRandom()
		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: versionId,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes: []*objecttree.Change{
				// set vertical align
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetVerticalAlign{
													BlockSetVerticalAlign: &pb.EventBlockSetVerticalAlign{
														Id: blockId,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				// set align
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetAlign{
													BlockSetAlign: &pb.EventBlockSetAlign{
														Id: blockDivId,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				// set childrenIds
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetChildrenIds{
													BlockSetChildrenIds: &pb.EventBlockSetChildrenIds{
														Id: blockBookmarkId,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				// set childrenIds
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetChildrenIds{
													BlockSetChildrenIds: &pb.EventBlockSetChildrenIds{
														Id: blockBookmarkId,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				// set background color
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetBackgroundColor{
													BlockSetBackgroundColor: &pb.EventBlockSetBackgroundColor{
														Id: blockRelationId,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				// set fields
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetFields{
													BlockSetFields: &pb.EventBlockSetFields{
														Id: blockLatexId,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, nil)
		space.EXPECT().TreeBuilder().Return(treeBuilder)
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
		history := history{
			spaceService: spaceService,
		}
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl, blBookmark, blLatex, blDiv, blFile, blLink, blRelation})

		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 5)
	})
	t.Run("object with moved block changes by 1 participant", func(t *testing.T) {
		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		keys, _ := accountdata.NewRandom()
		participantId := domain.NewParticipantId(spaceID, keys.SignKey.GetPublic().Account())

		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: versionId,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes: []*objecttree.Change{
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{bl},
									},
								},
							},
						},
					},
				},
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockMove{
									BlockMove: &pb.ChangeBlockMove{
										Ids: []string{bl.Id},
									},
								},
							},
						},
					},
				},
			},
		}, nil)
		space.EXPECT().TreeBuilder().Return(treeBuilder)
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
		history := history{
			spaceService: spaceService,
		}
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl})

		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 1)
		assert.Equal(t, bl.Id, blocksModifiers[0].BlockId)
		assert.Equal(t, participantId, blocksModifiers[0].ParticipantId)
	})
	t.Run("object block was deleted, don't add it in response", func(t *testing.T) {
		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		keys, _ := accountdata.NewRandom()
		participantId := domain.NewParticipantId(spaceID, keys.SignKey.GetPublic().Account())

		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: versionId,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes: []*objecttree.Change{
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{bl},
									},
								},
							},
						},
					},
				},
				{
					Identity: keys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockRemove{
									BlockRemove: &pb.ChangeBlockRemove{
										Ids: []string{blockBookmarkId},
									},
								},
							},
						},
					},
				},
			},
		}, nil)
		space.EXPECT().TreeBuilder().Return(treeBuilder)
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
		history := history{
			spaceService: spaceService,
		}
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl})

		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 1)
		assert.Equal(t, bl.Id, blocksModifiers[0].BlockId)
		assert.Equal(t, participantId, blocksModifiers[0].ParticipantId)
	})
	t.Run("object with block changes by 2 participants", func(t *testing.T) {
		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		blBookmark := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
		blLatex := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
		blRelation := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}

		firstAccountKeys, _ := accountdata.NewRandom()
		secondAccountKeys, _ := accountdata.NewRandom()

		firstParticipantId := domain.NewParticipantId(spaceID, firstAccountKeys.SignKey.GetPublic().Account())
		secondParticipantId := domain.NewParticipantId(spaceID, secondAccountKeys.SignKey.GetPublic().Account())

		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: versionId,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes: []*objecttree.Change{
				{
					Identity: firstAccountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{bl},
									},
								},
							},
						},
					},
				},
				{
					Identity: secondAccountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetText{
													BlockSetText: &pb.EventBlockSetText{
														Id: bl.Id,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Identity: firstAccountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blBookmark},
									},
								},
							},
						},
					},
				},
				{
					Identity: secondAccountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetVerticalAlign{
													BlockSetVerticalAlign: &pb.EventBlockSetVerticalAlign{
														Id: blockLatexId,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Identity: secondAccountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockAdd{
													BlockAdd: &pb.EventBlockAdd{
														Blocks: []*model.Block{blRelation},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, nil)
		space.EXPECT().TreeBuilder().Return(treeBuilder)
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
		history := history{
			spaceService: spaceService,
		}
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl, blBookmark, blLatex, blRelation})

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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	versionId := "versionId"
	previousVersion := "previousVersion"
	blockId := "blockId"
	blockDivId := "blockDivID"
	blockLinkId := "blockLinkId"
	blockLatexId := "blockLatexId"
	blockFileId := "blockFileId"
	blockBookmarkId := "blockBookmarkId"
	blockRelationId := "blockRelationId"

	blDiv := &model.Block{Id: blockDivId, Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}
	blLink := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
	blLatex := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
	blFile := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
	blBookmark := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
	blRelation := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}
	bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}

	t.Run("object diff - new created block", func(t *testing.T) {
		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
		accountKeys, _ := accountdata.NewRandom()

		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: versionId,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes: []*objecttree.Change{
				{
					Id:       objectId,
					Identity: accountKeys.SignKey.GetPublic(),
					Model:    &pb.Change{},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{
											{
												Id: objectId,
												Content: &model.BlockContentOfSmartblock{
													Smartblock: &model.BlockContentSmartblock{},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{bl},
									},
								},
							},
						},
					},
				},
			},
		}, nil)
		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: previousVersion,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes: []*objecttree.Change{
				{
					Id:       objectId,
					Identity: accountKeys.SignKey.GetPublic(),
					Model:    &pb.Change{},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{
											{
												Id: objectId,
												Content: &model.BlockContentOfSmartblock{
													Smartblock: &model.BlockContentSmartblock{},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, nil)

		space.EXPECT().TreeBuilder().Return(treeBuilder)
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
		history := history{
			spaceService: spaceService,
			objectStore:  objectstore.NewStoreFixture(t),
		}
		changes, _, err := history.DiffVersions(&pb.RpcHistoryDiffVersionsRequest{
			ObjectId:        objectId,
			SpaceId:         spaceID,
			CurrentVersion:  versionId,
			PreviousVersion: previousVersion,
		})

		assert.Nil(t, err)
		assert.Len(t, changes, 2)

		assert.NotNil(t, changes[0].GetBlockSetChildrenIds())
		assert.Equal(t, objectId, changes[0].GetBlockSetChildrenIds().Id)
		assert.Equal(t, []string{bl.Id}, changes[0].GetBlockSetChildrenIds().ChildrenIds)

		assert.NotNil(t, changes[1].GetBlockAdd())
		assert.Len(t, changes[1].GetBlockAdd().Blocks, 1)
		assert.Equal(t, bl, changes[1].GetBlockAdd().Blocks[0])
	})

	t.Run("object diff - simple block changes", func(t *testing.T) {
		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
		accountKeys, _ := accountdata.NewRandom()

		blDivCopy := &model.Block{Id: blockDivId, Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}
		blLinkCopy := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
		blLatexCopy := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
		blFileCopy := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
		blBookmarkCopy := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
		blRelationCopy := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}
		blCopy := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}

		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: versionId,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes: []*objecttree.Change{
				{
					Id:       objectId,
					Identity: accountKeys.SignKey.GetPublic(),
					Model:    &pb.Change{},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{
											{
												Id: objectId,
												Content: &model.BlockContentOfSmartblock{
													Smartblock: &model.BlockContentSmartblock{},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blDivCopy},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blLatexCopy},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blLinkCopy},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blBookmarkCopy},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blRelationCopy},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blDivCopy},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blFileCopy},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blCopy},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetFile{
													BlockSetFile: &pb.EventBlockSetFile{
														Id:   blFile.Id,
														Name: &pb.EventBlockSetFileName{Value: "new file name"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetRelation{
													BlockSetRelation: &pb.EventBlockSetRelation{
														Id:  blRelation.Id,
														Key: &pb.EventBlockSetRelationKey{Value: "new key"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetBookmark{
													BlockSetBookmark: &pb.EventBlockSetBookmark{
														Id:  blBookmark.Id,
														Url: &pb.EventBlockSetBookmarkUrl{Value: "new url"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetLatex{
													BlockSetLatex: &pb.EventBlockSetLatex{
														Id:   blLatex.Id,
														Text: &pb.EventBlockSetLatexText{Value: "new latex text"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetLink{
													BlockSetLink: &pb.EventBlockSetLink{
														Id:       blLink.Id,
														IconSize: &pb.EventBlockSetLinkIconSize{Value: model.BlockContentLink_SizeSmall},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetText{
													BlockSetText: &pb.EventBlockSetText{
														Id: blockId,
														Text: &pb.EventBlockSetTextText{
															Value: "new text",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockUpdate{
									BlockUpdate: &pb.ChangeBlockUpdate{
										Events: []*pb.EventMessage{
											{
												Value: &pb.EventMessageValueOfBlockSetDiv{
													BlockSetDiv: &pb.EventBlockSetDiv{
														Id:    blockDivId,
														Style: &pb.EventBlockSetDivStyle{Value: model.BlockContentDiv_Dots},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}, nil)
		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
			BeforeId: previousVersion,
			Include:  true,
		}).Return(&historyStub{
			objectId: objectId,
			changes: []*objecttree.Change{
				{
					Id:       objectId,
					Identity: accountKeys.SignKey.GetPublic(),
					Model:    &pb.Change{},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{
											{
												Id: objectId,
												Content: &model.BlockContentOfSmartblock{
													Smartblock: &model.BlockContentSmartblock{},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{bl},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blLatex},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blLink},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blBookmark},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blRelation},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blDiv},
									},
								},
							},
						},
					},
				},
				{
					Identity: accountKeys.SignKey.GetPublic(),
					Model: &pb.Change{
						Content: []*pb.ChangeContent{
							{
								Value: &pb.ChangeContentValueOfBlockCreate{
									BlockCreate: &pb.ChangeBlockCreate{
										Blocks: []*model.Block{blFile},
									},
								},
							},
						},
					},
				},
			},
		}, nil)

		space.EXPECT().TreeBuilder().Return(treeBuilder)
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
		history := history{
			spaceService: spaceService,
			objectStore:  objectstore.NewStoreFixture(t),
		}
		changes, _, err := history.DiffVersions(&pb.RpcHistoryDiffVersionsRequest{
			ObjectId:        objectId,
			SpaceId:         spaceID,
			CurrentVersion:  versionId,
			PreviousVersion: previousVersion,
		})

		assert.Nil(t, err)
		assert.Len(t, changes, 8)

		assert.NotNil(t, changes[0].GetBlockSetChildrenIds())
		assert.Equal(t, objectId, changes[0].GetBlockSetChildrenIds().Id)

		assert.NotNil(t, changes[1].GetBlockSetLatex())
		assert.NotNil(t, changes[2].GetBlockSetLink())
		assert.NotNil(t, changes[3].GetBlockSetBookmark())
		assert.NotNil(t, changes[4].GetBlockSetRelation())
		assert.NotNil(t, changes[5].GetBlockSetDiv())
		assert.NotNil(t, changes[6].GetBlockSetFile())
		assert.NotNil(t, changes[7].GetBlockSetText())
	})

	t.Run("object diff - block properties changes", func(t *testing.T) {

	})

	t.Run("object diff - different changes, but we don't include them because they aren't blocks and relation changes", func(t *testing.T) {

	})

	t.Run("object diff - dataview changes", func(t *testing.T) {

	})

	t.Run("object diff - relations and details changes", func(t *testing.T) {

	})

	t.Run("object diff - no changes", func(t *testing.T) {

	})
}
