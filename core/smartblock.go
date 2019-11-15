package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/anytypeio/go-anytype-library/schema"
	"github.com/anytypeio/go-anytype-library/util"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	uuid "github.com/satori/go.uuid"
	tcore "github.com/textileio/go-textile/core"
	mill2 "github.com/textileio/go-textile/mill"
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

func (smartBlock *SmartBlock) GetVersion(id string) (smartBlockVersion *SmartBlockVersion, err error) {
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

	var block *storage.BlockWithDependentBlocks
	err = proto.Unmarshal(plaintext, block)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %s", err.Error())
	}

	version := &SmartBlockVersion{pb: block, versionId: fileMeta.Block, date: util.CastTimestampToGogo(fileMeta.Date), user: fileMeta.User.Address}
	return version, nil
}

func (smartBlock *SmartBlock) GetVersions(offset string, limit int, metaOnly bool) (versions []*SmartBlockVersion, err error) {
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

		block := &storage.BlockWithDependentBlocks{}

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

		version.pb = block
		versions = append(versions, version)
	}

	return
}

func (smartBlock *SmartBlock) AddVersion(newVersion *storage.BlockWithDependentBlocks) (versionId string, user string, date *types.Timestamp, err error) {
	var newVersionB []byte
	newVersionB, err = proto.Marshal(newVersion)
	if err != nil {
		return
	}

	mill := &mill2.Json{}

	conf := tcore.AddFileConfig{
		Media:     "application/json",
		Plaintext: false,
		Input:     newVersionB,
	}

	var newBlockVersionFile *tpb.FileIndex
	newBlockVersionFile, err = smartBlock.node.textile().AddFileIndex(mill, conf)
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
		caption = name.String()
	}

	block, err := smartBlock.thread.AddFiles(node, "version", caption, keys.Files)
	if err != nil {
		err = fmt.Errorf("thread.AddFiles error: %s", err.Error())
		return
	}

	versionId = block.B58String()
	user = smartBlock.node.textile().Account().Address()
	newBlock, err := smartBlock.node.textile().Block(block.B58String())
	if err != nil {
		log.Errorf("failed to get the block %s: %s", newBlock.Id, err.Error())
	}

	if newBlock != nil {
		date = util.CastTimestampToGogo(newBlock.Date)
	}

	return
}

// NewBlock should be used as constructor for the new block
func (smartBlock *SmartBlock) newBlock(block model.Block, smartBlockWrapper Block) (Block, error) {
	switch block.Content.(type) {
	case *model.BlockContentOfPage:
		thrd, err := smartBlock.node.newBlockThread(schema.Page)
		if err != nil {
			return nil, err
		}
		return &Page{&SmartBlock{thread: thrd, node: smartBlock.node}}, nil
	case *model.BlockContentOfDashboard:
		thrd, err := smartBlock.node.newBlockThread(schema.Dashboard)
		if err != nil {
			return nil, err
		}

		return &Dashboard{&SmartBlock{thread: thrd, node: smartBlock.node}}, nil
	default:
		return &SimpleBlock{
			parentSmartBlock: smartBlockWrapper,
			id:               uuid.NewV4().String(),
			node:             smartBlock.node,
		}, nil
	}
}

func (smartBlock *SmartBlock) SubscribeNewVersionsOfBlocks(sinceVersionId string, blocks chan<- []BlockVersion) (cancelFunc func(), err error) {
	// todo: to be implemented
	close(blocks)
	return func() {}, fmt.Errorf("not implemented")
}

func (smartBlock *SmartBlock) SubscribeClientEvents(events chan<- proto.Message) (cancelFunc func(), err error) {
	//todo: to be implemented
	close(events)
	return func() {}, fmt.Errorf("not implemented")
}

func (smartBlock *SmartBlock) PublishClientEvent(event proto.Message) error {
	//todo: to be implemented
	return fmt.Errorf("not implemented")
}
