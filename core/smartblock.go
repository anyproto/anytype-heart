package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
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

func (smartBlock *SmartBlock) GetVersionBlock(id string) (fileMeta *tpb.Files, block *pb.Block, err error) {
	fileMeta, err = smartBlock.node.textile().File(id)
	if err != nil {
		return nil, nil, err
	}

	if len(fileMeta.Files) == 0 {
		return nil, nil, fmt.Errorf("version block not found")
	}

	plaintext, err := readFile(smartBlock.node.textile(), fileMeta.Files[0].File)
	if err != nil {
		return nil, nil, fmt.Errorf("readFile error: %s", err.Error())
	}

	err = proto.Unmarshal(plaintext, block)
	if err != nil {
		return nil, nil, fmt.Errorf("unmarshal error: %s", err.Error())
	}

	return fileMeta, block, err
}

func (smartBlock *SmartBlock) GetVersionsFiles(offset string, limit int, metaOnly bool) (filesMeta []*tpb.Files, blocks []*pb.Block, err error) {
	files, err := smartBlock.node.textile().Files(offset, limit, smartBlock.thread.Id)
	if err != nil {
		return nil, nil, err
	}

	filesMeta = files.Items

	if metaOnly {
		return
	}

	for _, item := range files.Items {
		block := &pb.Block{}

		plaintext, err := readFile(smartBlock.node.Textile.Node(), item.Files[0].File)
		if err != nil {
			// todo: decide if it will be ok to have more meta than blocks content itself
			// in case of error cut off filesMeta in order to have related indexes in both slices
			return filesMeta[0:len(blocks)], blocks, fmt.Errorf("readFile error: %s", err.Error())
		}

		err = proto.Unmarshal(plaintext, block)
		if err != nil {
			return filesMeta, blocks, fmt.Errorf("page version proto unmarshal error: %s", err.Error())
		}

		blocks = append(blocks, block)
	}

	return
}

func (smartBlock *SmartBlock) AddVersion(newVersion *pb.Block) (versionId string, user string, date *timestamp.Timestamp, err error) {
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

	if name, exist := newVersion.GetFields().Fields["name"]; exist {
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
		date = newBlock.Date
	}

	return
}

func (smartBlock *SmartBlock) SubscribeClientEvents(events chan<- proto.Message) (cancelFunc func()) {
	//todo: to be implemented
	close(events)
	return func() {}
}

func (smartBlock *SmartBlock) PublishClientEvent(event proto.Message) {
	//todo: to be implemented
	return
}
