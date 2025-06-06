package history

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder/mock_objecttreebuilder"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type historyStub struct {
	changes  []*objecttree.Change
	heads    []string
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

func (h historyStub) Heads() []string { return h.heads }

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

func TestHistory_GetBlocksParticipants(t *testing.T) {
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
	blockTableId := "blockTableId"

	t.Run("object without blocks", func(t *testing.T) {
		// given
		history := newFixture(t, nil, objectId, spaceID, versionId)

		// when
		blocksParticipants, err := history.GetBlocksParticipants(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, nil)

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksParticipants, 0)
	})
	t.Run("object with 1 created block", func(t *testing.T) {
		// given
		keys, _ := accountdata.NewRandom()
		participantId := domain.NewParticipantId(spaceID, keys.SignKey.GetPublic().Account())
		bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		expectedChanges := []*objecttree.Change{provideBlockCreateChange(bl, keys.SignKey.GetPublic())}
		history := newFixture(t, expectedChanges, objectId, spaceID, versionId)

		// when
		blocksParticipants, err := history.GetBlocksParticipants(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksParticipants, 1)
		assert.Equal(t, bl.Id, blocksParticipants[0].BlockId)
		assert.Equal(t, participantId, blocksParticipants[0].ParticipantId)
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
		blocksParticipants, err := history.GetBlocksParticipants(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksParticipants, 1)
		assert.Equal(t, bl.Id, blocksParticipants[0].BlockId)
		assert.Equal(t, participantId, blocksParticipants[0].ParticipantId)
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
		blTableRow := &model.Block{Id: blockTableId, Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}}

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
			provideBlockCreateChange(blTableRow, account),
			provideBlockSetTextChange(blRelation, account),

			// update block changes
			provideBlockSetTextChange(bl, account),
			provideBlockSetDivChange(blDiv, account),
			provideBlockSetLinkChange(blLink, account),
			provideBlockSetLatexChange(blLatex, account),
			provideBlockSetFileChange(blFile, account),
			provideBlockSetBookmarkChange(blBookmark, account),
			provideBlockSetRelationChange(blRelation, account),
			provideBlockSetTableRowChange(blTableRow, account),
		}
		history := newFixture(t, expectedChanges, objectId, spaceID, versionId)

		// when
		blocksParticipants, err := history.GetBlocksParticipants(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl, blBookmark, blLatex, blDiv, blFile, blLink, blRelation, blTableRow})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksParticipants, 8)
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
		blocksParticipants, err := history.GetBlocksParticipants(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl, blBookmark, blLatex, blDiv, blFile, blLink, blRelation})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksParticipants, 4)
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
		blocksParticipants, err := history.GetBlocksParticipants(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksParticipants, 1)
		assert.Equal(t, bl.Id, blocksParticipants[0].BlockId)
		assert.Equal(t, participantId, blocksParticipants[0].ParticipantId)
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
		blocksParticipants, err := history.GetBlocksParticipants(domain.FullID{
			ObjectID: objectId,
			SpaceID:  spaceID,
		}, versionId, []*model.Block{bl, blBookmark, blLatex, blRelation})

		// then
		assert.Nil(t, err)
		assert.Len(t, blocksParticipants, 4)
		assert.Contains(t, blocksParticipants, &model.ObjectViewBlockParticipant{
			BlockId:       bl.Id,
			ParticipantId: secondParticipantId,
		})
		assert.Contains(t, blocksParticipants, &model.ObjectViewBlockParticipant{
			BlockId:       blBookmark.Id,
			ParticipantId: firstParticipantId,
		})
		assert.Contains(t, blocksParticipants, &model.ObjectViewBlockParticipant{
			BlockId:       blLatex.Id,
			ParticipantId: secondParticipantId,
		})
		assert.Contains(t, blocksParticipants, &model.ObjectViewBlockParticipant{
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
	blockDivId := "blockDivID"
	blockLinkId := "blockLinkId"
	blockLatexId := "blockLatexId"
	blockFileId := "blockFileId"
	blockBookmarkId := "blockBookmarkId"
	blockRelationId := "blockRelationId"

	bl := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
	blSmartBlock := &model.Block{Id: objectId, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}
	blDiv := &model.Block{Id: blockDivId, Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}
	blLink := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
	blLatex := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
	blFile := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
	blBookmark := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
	blRelation := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}

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
	t.Run("object diff - simple block changes", func(t *testing.T) {
		// given
		blDivCopy := &model.Block{Id: blockDivId, Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}
		blLinkCopy := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
		blLatexCopy := &model.Block{Id: blockLatexId, Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}}}
		blFileCopy := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
		blBookmarkCopy := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
		blRelationCopy := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}
		blCopy := &model.Block{Id: blockId, Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}

		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()
		currChange := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blDivCopy, account),
			provideBlockCreateChange(blLatexCopy, account),
			provideBlockCreateChange(blLinkCopy, account),
			provideBlockCreateChange(blBookmarkCopy, account),
			provideBlockCreateChange(blRelationCopy, account),
			provideBlockCreateChange(blFileCopy, account),
			provideBlockCreateChange(blCopy, account),

			// set block changes
			provideBlockSetFileChange(blFileCopy, account),
			provideBlockSetRelationChange(blRelationCopy, account),
			provideBlockSetBookmarkChange(blBookmarkCopy, account),
			provideBlockSetLatexChange(blLatexCopy, account),
			provideBlockSetLinkChange(blLinkCopy, account),
			provideBlockSetTextChange(blCopy, account),
			provideBlockSetDivChange(blDivCopy, account),
		}

		prevChanges := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blDiv, account),
			provideBlockCreateChange(blLatex, account),
			provideBlockCreateChange(blLink, account),
			provideBlockCreateChange(blBookmark, account),
			provideBlockCreateChange(blRelation, account),
			provideBlockCreateChange(blFile, account),
			provideBlockCreateChange(bl, account),
		}

		// when
		history := newFixtureDiffVersions(t, currChange, prevChanges, objectId, spaceID, versionId, previousVersion)
		changes, _, err := history.DiffVersions(&pb.RpcHistoryDiffVersionsRequest{
			ObjectId:        objectId,
			SpaceId:         spaceID,
			CurrentVersion:  versionId,
			PreviousVersion: previousVersion,
		})

		// then
		assert.Nil(t, err)
		assert.Len(t, changes, 7)

		assert.NotNil(t, changes[0].GetBlockSetDiv())
		assert.NotNil(t, changes[1].GetBlockSetLatex())
		assert.NotNil(t, changes[2].GetBlockSetLink())
		assert.NotNil(t, changes[3].GetBlockSetBookmark())
		assert.NotNil(t, changes[4].GetBlockSetRelation())
		assert.NotNil(t, changes[5].GetBlockSetFile())
		assert.NotNil(t, changes[6].GetBlockSetText())
	})

	t.Run("object diff - block properties changes", func(t *testing.T) {
		// given
		blFileCopy := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
		blBookmarkCopy := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
		blRelationCopy := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}
		blLinkCopy := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}

		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()
		currChange := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blLinkCopy, account),
			provideBlockCreateChange(blBookmarkCopy, account),
			provideBlockCreateChange(blRelationCopy, account),
			provideBlockCreateChange(blFileCopy, account),

			// block properties changes
			provideBlockSetVerticalAlignChange(blLinkCopy, account),
			provideBlockSetAlignChange(blFileCopy, account),
			provideBlockBackgroundColorChange(blRelationCopy, account),
			provideBlockFieldChange(blBookmarkCopy, account),
		}

		prevChanges := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blLink, account),
			provideBlockCreateChange(blBookmark, account),
			provideBlockCreateChange(blRelation, account),
			provideBlockCreateChange(blFile, account),
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
		assert.Len(t, changes, 4)

		assert.NotNil(t, changes[0].GetBlockSetVerticalAlign())
		assert.NotNil(t, changes[1].GetBlockSetFields())
		assert.NotNil(t, changes[2].GetBlockSetBackgroundColor())
		assert.NotNil(t, changes[3].GetBlockSetAlign())
	})

	t.Run("object diff - block change and timestamp change, return only block change", func(t *testing.T) {
		// given
		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()
		blLinkCopy := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}
		currChange := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blLinkCopy, account),

			// block properties changes
			provideBlockSetVerticalAlignChange(blLinkCopy, account),

			// not block change
			provideNonBlockChange(account),
		}

		prevChanges := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blLink, account),
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

		assert.NotNil(t, changes[0].GetBlockSetVerticalAlign())
	})

	t.Run("object diff - dataview changes", func(t *testing.T) {
		// given
		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()

		blDataviewId := "blDataviewId"
		relationKey := "key"

		viewId := "viewId"
		viewName := "view"

		viewId1 := "viewId1"
		view1Name := "view1"

		viewId2 := "viewId2"
		view2Name := "view2"

		blDataview := provideDataviewBlock(viewId1, view1Name, relationKey, blDataviewId)
		blDataviewCopy := provideDataviewBlock(viewId1, view1Name, relationKey, blDataviewId)

		currChange := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blDataviewCopy, account),

			// dataview  changes
			provideBlockDataviewViewSetChange(blDataviewId, viewId1, view1Name, relationKey, account),
			provideBlockDataviewViewSetChange(blDataviewId, viewId2, view2Name, relationKey, account),
			provideBlockDataviewSourceSetChange(blDataviewId, account),
			provideBlockDataviewRelationSetChange(blDataviewId, account),
			provideBlockDataviewViewSetChange(blDataviewId, viewId, viewName, relationKey, account),
			provideBlockDataviewViewOrderChange(blDataviewId, viewId, viewId1, account),
			provideBlockDataviewViewDeleteChange(blDataviewId, viewId2, account),
			provideBlockDataviewRelationDeleteChange(blDataviewId, relationKey, account),
			provideBlockDataviewObjectOrderChange(blDataviewId, viewId, account),
			provideBlockDataviewGroupOrderChange(blDataviewId, viewId, account),
			provideBlockDataviewViewUpdateChange(blDataviewId, viewId, account),
			provideBlockDataviewTargetObjectChange(blDataviewId, account),
		}

		prevChanges := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blDataview, account),

			provideBlockDataviewViewSetChange(blDataviewId, viewId1, view1Name, relationKey, account),
			provideBlockDataviewViewSetChange(blDataviewId, viewId2, view2Name, relationKey, account),
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
		assert.Len(t, changes, 10)
	})
	t.Run("object diff - relations and details changes", func(t *testing.T) {
		// given
		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()

		relationKey := "key"
		relationKey1 := "key1"
		relationKey2 := "key2"

		currChange := []*objecttree.Change{
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideRelationAddChange(account, &model.RelationLink{
				Key:    relationKey,
				Format: model.RelationFormat_tag,
			},
				&model.RelationLink{
					Key:    relationKey1,
					Format: model.RelationFormat_longtext,
				},
				&model.RelationLink{
					Key:    relationKey2,
					Format: model.RelationFormat_longtext,
				}),
			provideDetailsSetChange(account, relationKey2, pbtypes.String("value2")),

			provideDetailsSetChange(account, relationKey, pbtypes.String("value")),
			provideRelationRemoveChange(account, relationKey1),
			provideRelationAddChange(account, &model.RelationLink{
				Key:    "key3",
				Format: model.RelationFormat_tag,
			}),
			provideDetailsUnsetChange(account, relationKey2),
		}

		prevChanges := []*objecttree.Change{
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideRelationAddChange(account, &model.RelationLink{
				Key:    relationKey,
				Format: model.RelationFormat_tag,
			},
				&model.RelationLink{
					Key:    relationKey1,
					Format: model.RelationFormat_longtext,
				},
				&model.RelationLink{
					Key:    relationKey2,
					Format: model.RelationFormat_longtext,
				}),
			provideDetailsSetChange(account, relationKey2, pbtypes.String("value2")),
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
		assert.Len(t, changes, 4)
	})
	t.Run("object diff -local relations changes", func(t *testing.T) {
		// given
		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()

		relationKey := "key"
		relationKey1 := "key1"

		currChange := []*objecttree.Change{
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideRelationAddChange(account, &model.RelationLink{
				Key:    relationKey,
				Format: model.RelationFormat_tag,
			},
				&model.RelationLink{
					Key:    bundle.RelationKeySpaceId.String(),
					Format: model.RelationFormat_longtext,
				},
				&model.RelationLink{
					Key:    bundle.RelationKeySyncStatus.String(),
					Format: model.RelationFormat_number,
				}),
		}

		prevChanges := []*objecttree.Change{
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideRelationAddChange(account, &model.RelationLink{
				Key:    relationKey1,
				Format: model.RelationFormat_tag,
			},
				&model.RelationLink{
					Key:    bundle.RelationKeyRestrictions.String(),
					Format: model.RelationFormat_longtext,
				},
				&model.RelationLink{
					Key:    bundle.RelationKeySyncDate.String(),
					Format: model.RelationFormat_number,
				}),
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
		assert.Len(t, changes, 2)
		assert.Len(t, changes[1].GetObjectRelationsAmend().RelationLinks, 1)
		assert.Equal(t, changes[1].GetObjectRelationsAmend().RelationLinks[0].Key, relationKey)
		assert.Len(t, changes[0].GetObjectRelationsRemove().RelationKeys, 1)
		assert.Equal(t, changes[0].GetObjectRelationsRemove().RelationKeys[0], relationKey1)
	})

	t.Run("object diff - no changes", func(t *testing.T) {
		// given
		blFileCopy := &model.Block{Id: blockFileId, Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}}}
		blBookmarkCopy := &model.Block{Id: blockBookmarkId, Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{}}}
		blRelationCopy := &model.Block{Id: blockRelationId, Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{}}}
		blLinkCopy := &model.Block{Id: blockLinkId, Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{}}}

		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()
		currChange := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blLinkCopy, account),
			provideBlockCreateChange(blBookmarkCopy, account),
			provideBlockCreateChange(blRelationCopy, account),
			provideBlockCreateChange(blFileCopy, account),
		}

		prevChanges := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blLink, account),
			provideBlockCreateChange(blBookmark, account),
			provideBlockCreateChange(blRelation, account),
			provideBlockCreateChange(blFile, account),
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
		assert.Len(t, changes, 0)
	})
}

func TestHistory_Versions(t *testing.T) {
	t.Run("limit 0 - 100 changes", func(t *testing.T) {
		// given
		objectId := "objectId"
		spaceID := "spaceID"
		versionId := "versionId"

		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()

		blDataviewId := "blDataviewId"
		relationKey := "key"

		viewId := "viewId"
		viewName := "view"

		viewId1 := "viewId1"
		view1Name := "view1"

		viewId2 := "viewId2"
		view2Name := "view2"

		blDataview := provideDataviewBlock(viewId1, view1Name, relationKey, blDataviewId)

		blSmartBlock := &model.Block{Id: objectId, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}

		currChange := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blDataview, account),

			// dataview  changes
			provideBlockDataviewViewSetChange(blDataviewId, viewId1, view1Name, relationKey, account),
			provideBlockDataviewViewSetChange(blDataviewId, viewId2, view2Name, relationKey, account),
			provideBlockDataviewSourceSetChange(blDataviewId, account),
			provideBlockDataviewRelationSetChange(blDataviewId, account),
			provideBlockDataviewViewSetChange(blDataviewId, viewId, viewName, relationKey, account),
			provideBlockDataviewViewOrderChange(blDataviewId, viewId, viewId1, account),
			provideBlockDataviewViewDeleteChange(blDataviewId, viewId2, account),
			provideBlockDataviewRelationDeleteChange(blDataviewId, relationKey, account),
			provideBlockDataviewObjectOrderChange(blDataviewId, viewId, account),
			provideBlockDataviewGroupOrderChange(blDataviewId, viewId, account),
			provideBlockDataviewViewUpdateChange(blDataviewId, viewId, account),
			provideBlockDataviewTargetObjectChange(blDataviewId, account),
		}

		history := newFixture(t, currChange, objectId, spaceID, versionId)

		// when
		resp, err := history.Versions(domain.FullID{ObjectID: objectId, SpaceID: spaceID}, versionId, 0, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, resp, 14)
	})
	t.Run("limit 10 - 10 changes", func(t *testing.T) {
		// given
		objectId := "objectId"
		spaceID := "spaceID"
		versionId := "versionId"

		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()

		blDataviewId := "blDataviewId"
		relationKey := "key"

		viewId := "viewId"
		viewName := "view"

		viewId1 := "viewId1"
		view1Name := "view1"

		viewId2 := "viewId2"
		view2Name := "view2"

		blDataview := provideDataviewBlock(viewId1, view1Name, relationKey, blDataviewId)

		blSmartBlock := &model.Block{Id: objectId, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}

		currChange := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blDataview, account),

			// dataview  changes
			provideBlockDataviewViewSetChange(blDataviewId, viewId1, view1Name, relationKey, account),
			provideBlockDataviewViewSetChange(blDataviewId, viewId2, view2Name, relationKey, account),
			provideBlockDataviewSourceSetChange(blDataviewId, account),
			provideBlockDataviewRelationSetChange(blDataviewId, account),
			provideBlockDataviewViewSetChange(blDataviewId, viewId, viewName, relationKey, account),
			provideBlockDataviewViewOrderChange(blDataviewId, viewId, viewId1, account),
			provideBlockDataviewViewDeleteChange(blDataviewId, viewId2, account),
			provideBlockDataviewRelationDeleteChange(blDataviewId, relationKey, account),
			provideBlockDataviewObjectOrderChange(blDataviewId, viewId, account),
			provideBlockDataviewGroupOrderChange(blDataviewId, viewId, account),
			provideBlockDataviewViewUpdateChange(blDataviewId, viewId, account),
			provideBlockDataviewTargetObjectChange(blDataviewId, account),
		}

		history := newFixture(t, currChange, objectId, spaceID, versionId)

		// when
		resp, err := history.Versions(domain.FullID{ObjectID: objectId, SpaceID: spaceID}, versionId, 10, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, resp, 10)
	})
	t.Run("number of changes equals limit", func(t *testing.T) {
		// given
		objectId := "objectId"
		spaceID := "spaceID"
		versionId := "versionId"

		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()

		blDataviewId := "blDataviewId"
		relationKey := "key"

		viewId := "viewId"
		viewName := "view"

		viewId1 := "viewId1"
		view1Name := "view1"

		viewId2 := "viewId2"
		view2Name := "view2"

		blDataview := provideDataviewBlock(viewId1, view1Name, relationKey, blDataviewId)

		blSmartBlock := &model.Block{Id: objectId, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}

		currChange := []*objecttree.Change{
			// create block changes
			provideBlockEmptyChange(objectId, account),
			provideBlockCreateChange(blSmartBlock, account),
			provideBlockCreateChange(blDataview, account),

			// dataview  changes
			provideBlockDataviewViewSetChange(blDataviewId, viewId1, view1Name, relationKey, account),
			provideBlockDataviewViewSetChange(blDataviewId, viewId2, view2Name, relationKey, account),
			provideBlockDataviewSourceSetChange(blDataviewId, account),
			provideBlockDataviewRelationSetChange(blDataviewId, account),
			provideBlockDataviewViewSetChange(blDataviewId, viewId, viewName, relationKey, account),
			provideBlockDataviewViewOrderChange(blDataviewId, viewId, viewId1, account),
			provideBlockDataviewViewDeleteChange(blDataviewId, viewId2, account),
			provideBlockDataviewRelationDeleteChange(blDataviewId, relationKey, account),
		}

		history := newFixture(t, currChange, objectId, spaceID, versionId)

		// when
		resp, err := history.Versions(domain.FullID{ObjectID: objectId, SpaceID: spaceID}, versionId, 10, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, resp, 10)
	})
	t.Run("no changes", func(t *testing.T) {
		// given
		objectId := "objectId"
		spaceID := "spaceID"
		versionId := "versionId"

		var currChange []*objecttree.Change

		history := newFixture(t, currChange, objectId, spaceID, versionId)

		ctrl := gomock.NewController(t)
		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
		configureTreeBuilder(treeBuilder, objectId, versionId, spaceID, currChange, space, spaceService)
		history.treeBuilder = treeBuilder
		history.space = space
		history.spaceService = spaceService

		// when
		resp, err := history.Versions(domain.FullID{ObjectID: objectId, SpaceID: spaceID}, versionId, 10, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, resp, 0)
	})
	t.Run("changes from parallel editing", func(t *testing.T) {
		// given
		objectId := "objectId"
		spaceID := "spaceID"
		versionId := "versionId"

		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()

		ch := &objecttree.Change{
			Id:          "id",
			PreviousIds: []string{"id2"},
			Identity:    account,
		}

		ch1 := &objecttree.Change{
			Identity:    account,
			Id:          "id1",
			PreviousIds: []string{"id2"},
		}

		ch2 := &objecttree.Change{
			Id:       "id2",
			Identity: account,
		}

		currChange := []*objecttree.Change{
			ch2, ch, ch1,
		}

		history := newFixture(t, currChange, objectId, spaceID, versionId)

		// when
		resp, err := history.Versions(domain.FullID{ObjectID: objectId, SpaceID: spaceID}, versionId, 10, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, resp, 3)
	})
}

func TestHistory_injectLocalDetails(t *testing.T) {
	spaceId := "cosmos"
	t.Run("local details are injected to state", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String(bundle.TypeKeyTask.URL()),
			bundle.RelationKeySpaceId:           domain.String(spaceId),
			bundle.RelationKeyRecommendedLayout: domain.Int64(int64(model.ObjectType_todo)),
		}})
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().GetTypeIdByKey(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.TypeKey) (string, error) {
			return key.URL(), nil
		})
		resolver := mock_idresolver.NewMockResolver(t)
		h := &history{
			objectStore: store,
			resolver:    resolver,
		}
		id := domain.FullID{SpaceID: spaceId, ObjectID: "object"}
		st := state.NewDoc(id.ObjectID, nil).NewState().SetObjectTypeKey(bundle.TypeKeyTask).SetDetails(
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyLayout: domain.Int64(int64(model.ObjectType_todo)),
			}),
		)

		// when
		err := h.injectLocalDetails(st, id, space)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, st.LocalDetails())
		assert.Equal(t, bundle.TypeKeyTask.URL(), st.LocalDetails().GetString(bundle.RelationKeyType))
		assert.Equal(t, spaceId, st.LocalDetails().GetString(bundle.RelationKeySpaceId))
		assert.Equal(t, int64(model.ObjectType_todo), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
	})

	t.Run("resolved layout should be retrieved from type", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String(bundle.TypeKeyProject.URL()),
			bundle.RelationKeySpaceId:           domain.String(spaceId),
			bundle.RelationKeyRecommendedLayout: domain.Int64(int64(model.ObjectType_note)),
		}})
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().GetTypeIdByKey(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.TypeKey) (string, error) {
			return key.URL(), nil
		})
		resolver := mock_idresolver.NewMockResolver(t)
		h := &history{
			objectStore: store,
			resolver:    resolver,
		}
		id := domain.FullID{SpaceID: spaceId, ObjectID: "object"}
		st := state.NewDoc(id.ObjectID, nil).NewState().SetObjectTypeKey(bundle.TypeKeyProject)

		// when
		err := h.injectLocalDetails(st, id, space)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, st.LocalDetails())
		assert.Equal(t, bundle.TypeKeyProject.URL(), st.LocalDetails().GetString(bundle.RelationKeyType))
		assert.Equal(t, spaceId, st.LocalDetails().GetString(bundle.RelationKeySpaceId))
		assert.Equal(t, int64(model.ObjectType_note), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
	})
}

func TestHistory_Show(t *testing.T) {
	t.Run("show history when parallel editing", func(t *testing.T) {
		objectId := "objectId"
		spaceID := "spaceID"

		accountKeys, _ := accountdata.NewRandom()
		account := accountKeys.SignKey.GetPublic()

		ch := &objecttree.Change{
			Id:          "id",
			PreviousIds: []string{objectId},
			Identity:    account,
			Model:       &pb.Change{},
		}

		ch1 := &objecttree.Change{
			Identity:    account,
			Id:          "id1",
			PreviousIds: []string{objectId},
			Model:       &pb.Change{},
		}

		root := &objecttree.Change{
			Id:       objectId,
			Identity: account,
			Model:    &pb.Change{},
		}

		changesMap := map[string][]*objecttree.Change{
			"id id1": {root, ch1},
			"id1 id": {root, ch},
			objectId: {root, ch, ch1},
		}

		h := newFixtureShow(t, changesMap, objectId, spaceID)

		// when
		fullId := domain.FullID{ObjectID: objectId, SpaceID: spaceID}
		resp, err := h.Versions(fullId, objectId, 10, false)
		require.Nil(t, err)
		require.Len(t, resp, 2)

		view, version, err := h.Show(fullId, resp[0].Id)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, view)
		assert.NotNil(t, version)
	})
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
		configureTreeBuilder(treeBuilder, objectId, versionId, spaceID, expectedChanges, space, spaceService)
	}
	history := &history{
		objectStore:  objectstore.NewStoreFixture(t),
		spaceService: spaceService,
		heads:        map[string]string{},
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
	resolver := mock_idresolver.NewMockResolver(t)
	resolver.EXPECT().ResolveSpaceID(mock.Anything).Return(spaceID, nil).Maybe()
	if len(currChanges) > 0 {
		configureTreeBuilder(treeBuilder, objectId, currVersionId, spaceID, currChanges, space, spaceService)
	}
	if len(prevChanges) > 0 {
		configureTreeBuilder(treeBuilder, objectId, prevVersionId, spaceID, prevChanges, space, spaceService)
	}
	history := &history{
		objectStore:  objectstore.NewStoreFixture(t),
		spaceService: spaceService,
		heads:        map[string]string{},
		resolver:     resolver,
	}
	return &historyFixture{
		history:     history,
		space:       space,
		treeBuilder: treeBuilder,
	}
}

func newFixtureShow(t *testing.T, changes map[string][]*objecttree.Change, objectId, spaceID string) *historyFixture {
	spaceService := mock_space.NewMockService(t)
	space := mock_clientspace.NewMockSpace(t)
	ctrl := gomock.NewController(t)
	treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)

	if len(changes) > 0 {
		treeBuilder.EXPECT().BuildHistoryTree(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, id string, opts objecttreebuilder.HistoryTreeOpts) (objecttree.HistoryTree, error) {
			assert.True(t, opts.Include)
			assert.Equal(t, objectId, id)
			versionId := strings.Join(opts.Heads, " ")

			chs, ok := changes[versionId]
			assert.True(t, ok)

			return &historyStub{
				objectId: objectId,
				changes:  chs,
				heads:    opts.Heads,
			}, nil
		}).AnyTimes()
		space.EXPECT().TreeBuilder().Return(treeBuilder)
		space.EXPECT().Id().Return(spaceID).Maybe()
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
	}

	space.EXPECT().GetTypeIdByKey(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, key domain.TypeKey) (string, error) {
		return key.URL(), nil
	})

	h := &history{
		objectStore:  objectstore.NewStoreFixture(t),
		spaceService: spaceService,
		heads:        map[string]string{},
	}
	return &historyFixture{
		history:     h,
		space:       space,
		treeBuilder: treeBuilder,
	}
}

func configureTreeBuilder(treeBuilder *mock_objecttreebuilder.MockTreeBuilder,
	objectId, currVersionId, spaceID string,
	expectedChanges []*objecttree.Change,
	space *mock_clientspace.MockSpace,
	spaceService *mock_space.MockService,
) {
	treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectId, objecttreebuilder.HistoryTreeOpts{
		Heads:   []string{currVersionId},
		Include: true,
	}).Return(&historyStub{
		objectId: objectId,
		changes:  expectedChanges,
		heads:    []string{currVersionId},
	}, nil)
	space.EXPECT().TreeBuilder().Return(treeBuilder)
	space.EXPECT().Id().Return(spaceID).Maybe()
	spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
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
											Id:    block.Id,
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
											Id:       block.Id,
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
											Id:   block.Id,
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
											Id:   block.Id,
											Name: &pb.EventBlockSetFileName{Value: "new name"},
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
											Id:  block.Id,
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
											Id:  block.Id,
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
											Id:            block.Id,
											VerticalAlign: model.Block_VerticalAlignBottom,
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
											Id:    block.Id,
											Align: model.Block_AlignCenter,
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
											Id:              block.Id,
											BackgroundColor: "pink",
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
											Id:     block.Id,
											Fields: &types.Struct{Fields: map[string]*types.Value{"key": pbtypes.String("value")}},
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

func provideNonBlockChange(account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfOriginalCreatedTimestampSet{
						OriginalCreatedTimestampSet: &pb.ChangeOriginalCreatedTimestampSet{Ts: 1},
					},
				},
			},
		},
	}
}

func provideDataviewBlock(viewId, viewName, relationKey, blDataviewId string) *model.Block {
	return &model.Block{
		Id: blDataviewId,
		Content: &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id:   viewId,
						Name: viewName,
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationKey: relationKey,
							},
						},
					},
				},
				RelationLinks: []*model.RelationLink{
					{
						Key: relationKey,
					},
				},
			},
		},
	}
}

func provideDetailsUnsetChange(account crypto.PubKey, relationKey string) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfDetailsUnset{
						DetailsUnset: &pb.ChangeDetailsUnset{
							Key: relationKey,
						},
					},
				},
			},
		},
	}
}

func provideRelationRemoveChange(account crypto.PubKey, relationKey string) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfRelationRemove{
						RelationRemove: &pb.ChangeRelationRemove{
							RelationKey: []string{relationKey},
						},
					},
				},
			},
		},
	}
}

func provideDetailsSetChange(account crypto.PubKey, relationKey string, value *types.Value) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfDetailsSet{
						DetailsSet: &pb.ChangeDetailsSet{
							Key:   relationKey,
							Value: value,
						},
					},
				},
			},
		},
	}
}

func provideRelationAddChange(account crypto.PubKey, relationLinks ...*model.RelationLink) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfRelationAdd{
						RelationAdd: &pb.ChangeRelationAdd{
							RelationLinks: relationLinks,
						},
					},
				},
			},
		},
	}
}

func provideBlockDataviewTargetObjectChange(blockId string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockDataviewTargetObjectIdSet{
										BlockDataviewTargetObjectIdSet: &pb.EventBlockDataviewTargetObjectIdSet{
											Id:             blockId,
											TargetObjectId: "targetObjectId",
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

func provideBlockDataviewViewUpdateChange(blockId string, viewId string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockDataviewViewUpdate{
										BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
											Id:     blockId,
											ViewId: viewId,
											Filter: provideFilterUpdate(),
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

func provideFilterUpdate() []*pb.EventBlockDataviewViewUpdateFilter {
	return []*pb.EventBlockDataviewViewUpdateFilter{
		{
			Operation: &pb.EventBlockDataviewViewUpdateFilterOperationOfAdd{
				Add: &pb.EventBlockDataviewViewUpdateFilterAdd{
					AfterId: "",
					Items: []*model.BlockContentDataviewFilter{
						{
							Id:          "filterId",
							Operator:    model.BlockContentDataviewFilter_Or,
							RelationKey: "key1",
							Value:       pbtypes.String("value"),
						},
					},
				},
			},
		},
	}
}

func provideBlockDataviewGroupOrderChange(blockId string, viewId string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockDataViewGroupOrderUpdate{
										BlockDataViewGroupOrderUpdate: &pb.EventBlockDataviewGroupOrderUpdate{
											Id: blockId,
											GroupOrder: &model.BlockContentDataviewGroupOrder{
												ViewId: viewId,
												ViewGroups: []*model.BlockContentDataviewViewGroup{
													{
														GroupId:         "group1",
														BackgroundColor: "pink",
													},
													{
														GroupId:         "group3",
														BackgroundColor: "blue",
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
		},
	}
}

func provideBlockDataviewObjectOrderChange(blockId string, viewId string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockDataViewObjectOrderUpdate{
										BlockDataViewObjectOrderUpdate: &pb.EventBlockDataviewObjectOrderUpdate{
											Id:     blockId,
											ViewId: viewId,
											SliceChanges: []*pb.EventBlockDataviewSliceChange{
												{
													Op:      pb.EventBlockDataview_SliceOperationNone,
													Ids:     []string{"objectId"},
													AfterId: "objectId1",
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
	}
}

func provideBlockDataviewRelationDeleteChange(blockId string, relationKey string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockDataviewRelationDelete{
										BlockDataviewRelationDelete: &pb.EventBlockDataviewRelationDelete{
											Id:           blockId,
											RelationKeys: []string{relationKey},
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

func provideBlockDataviewViewDeleteChange(blockId string, viewId2 string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockDataviewViewDelete{
										BlockDataviewViewDelete: &pb.EventBlockDataviewViewDelete{
											Id:     blockId,
											ViewId: viewId2,
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

func provideBlockDataviewViewOrderChange(blockId string, viewId string, viewId1 string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockDataviewViewOrder{
										BlockDataviewViewOrder: &pb.EventBlockDataviewViewOrder{
											Id:      blockId,
											ViewIds: []string{viewId, viewId1},
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

func provideBlockDataviewRelationSetChange(blockId string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockDataviewRelationSet{
										BlockDataviewRelationSet: &pb.EventBlockDataviewRelationSet{
											Id:            blockId,
											RelationLinks: []*model.RelationLink{{Key: "key1"}},
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

func provideBlockDataviewSourceSetChange(blockId string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockDataviewSourceSet{
										BlockDataviewSourceSet: &pb.EventBlockDataviewSourceSet{
											Id:     blockId,
											Source: []string{"source"},
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

func provideBlockDataviewViewSetChange(blDataviewId, viewId, view1Name, relationKey string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockDataviewViewSet{
										BlockDataviewViewSet: &pb.EventBlockDataviewViewSet{
											Id:     blDataviewId,
											ViewId: viewId,
											View: &model.BlockContentDataviewView{
												Id:   viewId,
												Name: view1Name,
												Sorts: []*model.BlockContentDataviewSort{
													{
														RelationKey: relationKey,
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
		},
	}
}

func provideBlockEmptyChange(objectId string, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Id:       objectId,
		Identity: account,
		Model:    &pb.Change{},
	}
}

func provideBlockSetTableRowChange(block *model.Block, account crypto.PubKey) *objecttree.Change {
	return &objecttree.Change{
		Identity: account,
		Model: &pb.Change{
			Content: []*pb.ChangeContent{
				{
					Value: &pb.ChangeContentValueOfBlockUpdate{
						BlockUpdate: &pb.ChangeBlockUpdate{
							Events: []*pb.EventMessage{
								{
									Value: &pb.EventMessageValueOfBlockSetTableRow{
										BlockSetTableRow: &pb.EventBlockSetTableRow{
											Id:       block.Id,
											IsHeader: &pb.EventBlockSetTableRowIsHeader{Value: true},
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
