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

func (smartBlock *SmartBlock) GetVersionFile(id string) (*tpb.Files, []byte, error) {
	files, err := smartBlock.node.textile().File(id)
	if err != nil {
		return nil, nil, err
	}

	if len(files.Files) == 0 {
		return nil, nil, fmt.Errorf("version block not found")
	}

	plaintext, err := readFile(smartBlock.node.textile(), files.Files[0].File)
	if err != nil {
		return nil, nil, fmt.Errorf("readFile error: %s", err.Error())
	}

	return files, plaintext, err
}

func (smartBlock *SmartBlock) GetVersionsFiles(offset string, limit int, metaOnly bool) ([]*tpb.Files, error) {
	files, err := smartBlock.node.textile().Files(offset, limit, smartBlock.thread.Id)
	if err != nil {
		return nil, err
	}

	return files.Items, nil
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
