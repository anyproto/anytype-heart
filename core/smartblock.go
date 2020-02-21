package core

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/anytypeio/go-anytype-library/schema"
	"github.com/anytypeio/go-anytype-library/util"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	uuid "github.com/satori/go.uuid"
	tcore "github.com/textileio/go-textile/core"
	"github.com/textileio/go-textile/mill"
	tpb "github.com/textileio/go-textile/pb"
)

type SmartBlock struct {
	thread *tcore.Thread
	node   *Anytype
}

func (smartBlock *SmartBlock) GetThread() *tcore.Thread {
	return smartBlock.thread
}

func (smartBlock *SmartBlock) GetId() string {
	return smartBlock.thread.Id
}

func (smartBlock *SmartBlock) GetCurrentVersion() (BlockVersion, error) {
	versions, err := smartBlock.GetVersions("", 1, false)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no block versions found")
	}

	return versions[0], nil
}

func (smartBlock *SmartBlock) GetCurrentVersionId() (string, error) {
	versions, err := smartBlock.GetVersions("", 1, true)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", ErrorNoBlockVersionsFound
	}

	return versions[0].VersionId(), nil
}

func (smartBlock *SmartBlock) GetVersion(id string) (BlockVersion, error) {
	fileMeta, err := smartBlock.node.textile().File(id)
	if err != nil {
		return nil, err
	}

	if len(fileMeta.Files) == 0 {
		return nil, fmt.Errorf("version block not found")
	}

	plaintext, err := readFile(smartBlock.node.textile(), fileMeta.Files[0].File)
	if err != nil {
		return nil, fmt.Errorf("readFile error: %s", err.Error())
	}

	var block *storage.BlockWithMeta
	err = proto.Unmarshal(plaintext, block)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %s", err.Error())
	}

	version := &SmartBlockVersion{model: block, versionId: fileMeta.Block, date: util.CastTimestampToGogo(fileMeta.Date), user: fileMeta.User.Address}
	err = version.addMissingFiles()
	if err != nil {
		return nil, err
	}

	return version, nil
}

func (smartBlock *SmartBlock) GetVersionMeta(id string) (BlockVersionMeta, error) {
	fileMeta, err := smartBlock.node.textile().File(id)
	if err != nil {
		return nil, err
	}

	if len(fileMeta.Files) == 0 {
		return nil, fmt.Errorf("version block not found")
	}

	plaintext, err := readFile(smartBlock.node.textile(), fileMeta.Files[0].File)
	if err != nil {
		return nil, fmt.Errorf("readFile error: %s", err.Error())
	}

	var block storage.BlockMetaOnly
	err = proto.Unmarshal(plaintext, &block)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %s", err.Error())
	}

	version := &SmartBlockVersionMeta{model: &block, versionId: fileMeta.Block, date: util.CastTimestampToGogo(fileMeta.Date), user: fileMeta.User.Address}

	return version, nil
}

func (smartBlock *SmartBlock) GetVersions(offset string, limit int, metaOnly bool) (versions []BlockVersion, err error) {
	files, err := smartBlock.node.textile().Files(offset, limit, smartBlock.thread.Id)
	if err != nil {
		return nil, err
	}

	for _, item := range files.Items {
		version := &SmartBlockVersion{versionId: item.Block, user: item.User.Address, date: util.CastTimestampToGogo(item.Date), node: smartBlock.node}
		if metaOnly {
			versions = append(versions, version)
			continue
		}

		block := &storage.BlockWithMeta{}

		plaintext, err := readFile(smartBlock.node.Textile.Node(), item.Files[0].File)
		if err != nil {
			// todo: decide if it will be ok to have more meta than blocks content itself
			// in case of error cut off filesMeta in order to have related indexes in both slices
			return versions, fmt.Errorf("readFile error: %s", err.Error())
		}

		err = proto.Unmarshal(plaintext, block)
		if err != nil {
			return versions, fmt.Errorf("page version proto unmarshal error: %s", err.Error())
		}

		version.model = block
		err = version.addMissingFiles()
		if err != nil {
			log.Errorf("addMissingFiles for version %s got error: %s", version.versionId, err.Error())
		}

		versions = append(versions, version)
	}

	if len(versions) > 0 && !metaOnly {
		db := versions[0].DependentBlocks()
		for _, child := range versions[0].Model().ChildrenIds {
			if _, exists := db[child]; !exists {
				log.Warningf("GetVersions: id=%d child %s is missing", versions[0].Model().Id, child)
			}
		}
	}
	return
}

func (smartBlock *SmartBlock) mergeWithLastVersion(newVersion *SmartBlockVersion) *SmartBlockVersion {
	lastVersion, _ := smartBlock.GetCurrentVersion()
	if lastVersion == nil {
		lastVersion = smartBlock.EmptyVersion()
	}

	var dependentBlocks = lastVersion.DependentBlocks()
	if newVersion.model.BlockById == nil {
		newVersion.model.BlockById = make(map[string]*model.Block, len(dependentBlocks))
	}
	for id, dependentBlock := range dependentBlocks {
		newVersion.model.BlockById[id] = dependentBlock.Model()
	}

	if newVersion.model.KeysByHash == nil {
		newVersion.model.KeysByHash = lastVersion.(*SmartBlockVersion).model.KeysByHash
	} else {
		for id, file := range lastVersion.(*SmartBlockVersion).model.KeysByHash {
			newVersion.model.KeysByHash[id] = file
		}
	}

	if newVersion.model.Block.Fields == nil || newVersion.model.Block.Fields.Fields == nil {
		newVersion.model.Block.Fields = lastVersion.Model().Fields
	}

	if newVersion.model.Block.Content == nil {
		newVersion.model.Block.Content = lastVersion.Model().Content
	}

	if newVersion.model.Block.ChildrenIds == nil {
		newVersion.model.Block.ChildrenIds = lastVersion.Model().ChildrenIds
	}

	if newVersion.model.Block.Restrictions == nil {
		newVersion.model.Block.Restrictions = lastVersion.Model().Restrictions
	}

	newVersion.model.Block.BackgroundColor = lastVersion.Model().BackgroundColor
	newVersion.model.Block.Align = lastVersion.Model().Align

	lastVersionB, _ := proto.Marshal(lastVersion.Model())
	newVersionB, _ := proto.Marshal(newVersion.Model())
	if string(lastVersionB) == string(newVersionB) {
		log.Debugf("[MERGE] new version has the same blocks as the last version - ignore it")
		// do not insert the new version if no blocks have changed
		newVersion.versionId = lastVersion.VersionId()
		newVersion.user = lastVersion.User()
		newVersion.date = lastVersion.Date()
		return newVersion
	}
	return newVersion
}

func (smartBlock *SmartBlock) AddVersion(block *model.Block) (BlockVersion, error) {
	if block.Id == "" {
		return nil, fmt.Errorf("block has empty id")
	}
	log.Debugf("AddVersion(%s): %d children=%+v", smartBlock.GetId(), len(block.ChildrenIds), block.ChildrenIds)

	newVersion := &SmartBlockVersion{model: &storage.BlockWithMeta{Block: block}}

	if block.Content != nil {
		switch strings.ToLower(smartBlock.thread.Schema.Name) {
		case "dashboard":
			if _, ok := block.Content.(*model.BlockContentOfDashboard); !ok {
				return nil, fmt.Errorf("unxpected smartblock type")
			}
		case "page":
			if _, ok := block.Content.(*model.BlockContentOfPage); !ok {
				return nil, fmt.Errorf("unxpected smartblock type")
			}
		default:
			return nil, fmt.Errorf("for now only smartblocks are queriable")
		}

		newVersion.model.Block.Content = block.Content
	}

	newVersion = smartBlock.mergeWithLastVersion(newVersion)
	if newVersion.versionId != "" {
		// nothing changes
		// todo: should we return error here to handle this specific case?
		return newVersion, nil
	}

	if block.Content == nil {
		block.Content = &model.BlockContentOfDashboard{Dashboard: &model.BlockContentDashboard{}}
	}

	var err error
	newVersion.versionId, newVersion.user, newVersion.date, err = smartBlock.addVersion(newVersion.model)
	if err != nil {
		return nil, err
	}

	return newVersion, nil
}

func (smartBlock *SmartBlock) AddVersions(blocks []*model.Block) ([]BlockVersion, error) {
	if len(blocks) == 0 {
		return nil, ErrorNoBlockVersionsFound
	}

	blockVersion := &SmartBlockVersion{model: &storage.BlockWithMeta{}}
	lastVersion, _ := smartBlock.GetCurrentVersion()
	fileKeysInLastVersion := make(map[string]*storage.FileKeys)
	if lastVersion != nil {
		var dependentBlocks = lastVersion.DependentBlocks()
		blockVersion.model.BlockById = make(map[string]*model.Block, len(dependentBlocks))
		for id, dependentBlock := range dependentBlocks {
			blockVersion.model.BlockById[id] = dependentBlock.Model()
		}
		blockVersion.model.Block = lastVersion.Model()
		fileKeysInLastVersion = lastVersion.(*SmartBlockVersion).model.KeysByHash
	} else {
		blockVersion.model.Block = &model.Block{Id: smartBlock.GetId()}
	}

	if blockVersion.model.BlockById == nil {
		blockVersion.model.BlockById = make(map[string]*model.Block, len(blocks))
	}

	if blockVersion.model.KeysByHash == nil {
		blockVersion.model.KeysByHash = make(map[string]*storage.FileKeys)
	}

	blockVersions := make([]BlockVersion, 0, len(blocks))

	for _, block := range blocks {
		if block.Id == "" {
			return nil, fmt.Errorf("block has empty id")
		}

		if block.Id == smartBlock.GetId() {
			if block.ChildrenIds != nil {
				blockVersion.model.Block.ChildrenIds = block.ChildrenIds
			}

			if block.Content != nil {
				blockVersion.model.Block.Content = block.Content
			}

			if block.Fields != nil {
				blockVersion.model.Block.Fields = block.Fields
			}

			if block.Restrictions != nil {
				blockVersion.model.Block.Restrictions = block.Restrictions
			}

			blockVersion.model.Block.Align = block.Align
			blockVersion.model.Block.BackgroundColor = block.BackgroundColor

			// only add dashboardVersion in case it was intentionally passed to AddVersions blocks
			blockVersions = append(blockVersions, blockVersion)
		} else {
			if isSmartBlock(block) {
				// todo: should we create an empty version?
				childSmartBlock, err := smartBlock.node.GetSmartBlock(block.Id)
				if err != nil {
					return nil, err
				}
				blockVersion, err := childSmartBlock.AddVersion(block)
				if err != nil {
					return nil, err
				}

				blockVersions = append(blockVersions, blockVersion)

				// no need to add smart block to dependencies blocks, so we can skip
				continue
			}

			if _, exists := blockVersion.model.BlockById[block.Id]; !exists {
				blockVersion.model.BlockById[block.Id] = block
			} else {
				if block.ChildrenIds != nil {
					blockVersion.model.BlockById[block.Id].ChildrenIds = block.ChildrenIds
				}

				if block.Restrictions != nil {
					blockVersion.model.BlockById[block.Id].Restrictions = block.Restrictions
				}

				if block.Fields != nil {
					blockVersion.model.BlockById[block.Id].Fields = block.Fields
				}

				if block.Content != nil {
					blockVersion.model.BlockById[block.Id].Content = block.Content
				}

				blockVersion.model.BlockById[block.Id].BackgroundColor = block.BackgroundColor
				blockVersion.model.BlockById[block.Id].Align = block.Align
			}

			if file, ok := block.Content.(*model.BlockContentOfFile); ok {
				if _, exists := fileKeysInLastVersion[file.File.Hash]; exists {
					blockVersion.model.KeysByHash[file.File.Hash] = fileKeysInLastVersion[file.File.Hash]
				} else {
					filesKeysCacheMutex.RLock()
					defer filesKeysCacheMutex.RUnlock()
					if keys, exists := filesKeysCache[file.File.Hash]; exists {
						blockVersion.model.KeysByHash[file.File.Hash] = &storage.FileKeys{keys}
					} //else if efile := smartBlock.thread.Datastore().Files().Get(file.File.Hash); efile != nil {
					// todo: extract keys from 'files' table in sqlite
					//  to provide a shutdown protection
					//}
				}
			}

			blockVersions = append(blockVersions, smartBlock.node.blockToVersion(block, blockVersion, "", "", nil))
		}
	}

	var err error
	blockVersion.versionId, blockVersion.user, blockVersion.date, err = smartBlock.addVersion(blockVersion.model)
	if err != nil {
		return nil, err
	}

	return blockVersions, nil
}

func (smartBlock *SmartBlock) addVersion(newVersion *storage.BlockWithMeta) (versionId string, user string, date *types.Timestamp, err error) {
	var newVersionB []byte
	newVersionB, err = proto.Marshal(newVersion)
	if err != nil {
		return
	}

	millBlob := &mill.Blob{}

	conf := tcore.AddFileConfig{
		Media:     "application/json",
		Plaintext: false,
		Input:     newVersionB,
	}

	var newBlockVersionFile *tpb.FileIndex
	newBlockVersionFile, err = smartBlock.node.textile().AddFileIndex(millBlob, conf)
	if err != nil {
		err = fmt.Errorf("AddFileIndex error: %s", err.Error())
		return
	}

	node, keys, err := smartBlock.node.textile().AddNodeFromFiles([]*tpb.FileIndex{newBlockVersionFile})
	if err != nil {
		err = fmt.Errorf("AddNodeFromFiles error: %s", err.Error())
		return
	}

	var caption string

	if name, exist := newVersion.Block.GetFields().Fields["name"]; exist {
		caption = name.GetStringValue()
	}

	block, err := smartBlock.thread.AddFiles(node, "", caption, keys.Files)
	if err != nil {
		err = fmt.Errorf("thread.AddFiles error: %s", err.Error())
		return
	}

	versionId = block.B58String()
	log.Debugf("SmartBlock.addVersion: blockId = %s newVersionId = %s", smartBlock.GetId(), versionId)
	user = smartBlock.node.textile().Account().Address()
	newBlock, err := smartBlock.node.textile().Block(block.B58String())
	if err != nil {
		log.Errorf("failed to get the block %s: %s", block.B58String(), err.Error())
	}

	if newBlock != nil {
		date = util.CastTimestampToGogo(newBlock.Date)
	}

	return
}

// NewBlock should be used as constructor for the new block
func (smartBlock *SmartBlock) NewBlock(block model.Block) (Block, error) {
	if block.Content == nil {
		return nil, fmt.Errorf("content not set")
	}

	var smartBlockSchemaBlob string
	switch block.Content.(type) {
	case *model.BlockContentOfPage:
		smartBlockSchemaBlob = schema.Page

	case *model.BlockContentOfDashboard:
		smartBlockSchemaBlob = schema.Dashboard

	}
	if smartBlockSchemaBlob != "" {
		thrd, err := smartBlock.node.newBlockThread(schema.Page)
		if err != nil {
			return nil, err
		}
		return &SmartBlock{thread: thrd, node: smartBlock.node}, nil
	}

	return &SimpleBlock{
		parentSmartBlock: smartBlock,
		id:               uuid.NewV4().String(),
		node:             smartBlock.node,
	}, nil
}

func (smartBlock *SmartBlock) EmptyVersion() BlockVersion {
	var content model.IsBlockContent
	switch strings.ToLower(smartBlock.thread.Schema.Name) {
	case "dashboard":
		content = &model.BlockContentOfDashboard{Dashboard: &model.BlockContentDashboard{}}
	case "page":
		content = &model.BlockContentOfPage{Page: &model.BlockContentPage{}}
	default:
		// shouldn't happen as checks for the schema performed before
		return nil
	}

	restr := blockRestrictionsEmpty()
	return &SmartBlockVersion{
		node: smartBlock.node,
		model: &storage.BlockWithMeta{
			Block: &model.Block{
				Id: smartBlock.GetId(),
				Fields: &types.Struct{Fields: map[string]*types.Value{
					"name": {Kind: &types.Value_StringValue{StringValue: ""}},
					"icon": {Kind: &types.Value_StringValue{StringValue: ""}},
				}},
				Restrictions: &restr,
				Content:      content,
			}},
	}
}

func (smartBlock *SmartBlock) SubscribeNewVersionsOfBlocks(sinceVersionId string, includeSinceVersion bool, blocks chan<- []BlockVersion) (cancelFunc func(), err error) {
	chCloseFn := func() { close(blocks) }

	if sinceVersionId == "" {
		// it must be set to ensure no versions were skipped in between
		return chCloseFn, fmt.Errorf("sinceVersionId must be set")
	}
	// todo: to be implemented
	return chCloseFn, nil
}

func (smartBlock *SmartBlock) SubscribeMetaOfNewVersionsOfBlock(sinceVersionId string, includeSinceVersion bool, blockMeta chan<- BlockVersionMeta) (cancelFunc func(), err error) {
	// temporary just sent the last version
	if sinceVersionId == "" {
		// it must be set to ensure no versions were skipped in between
		return nil, fmt.Errorf("sinceVersionId must be set")
	}
	var closeChan = make(chan struct{})
	chCloseFn := func() {
		close(closeChan)
	}

	// todo: implement with chan from textile events feed
	if includeSinceVersion {
		versionMeta, err := smartBlock.GetVersionMeta(sinceVersionId)
		if err != nil {
			return chCloseFn, err
		}
		go func() {
			select {
			case blockMeta <- versionMeta:
			case <-closeChan:
			}
			close(blockMeta)
		}()
	}

	return chCloseFn, nil
}

func (smartBlock *SmartBlock) SubscribeClientEvents(events chan<- proto.Message) (cancelFunc func(), err error) {
	//todo: to be implemented
	return func() { close(events) }, nil
}

func (smartBlock *SmartBlock) PublishClientEvent(event proto.Message) error {
	//todo: to be implemented
	return fmt.Errorf("not implemented")
}
