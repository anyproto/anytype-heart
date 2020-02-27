package core

import (
	"context"
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	mh "github.com/multiformats/go-multihash"
	uuid "github.com/satori/go.uuid"
	"github.com/textileio/go-threads/cbor"
	"github.com/textileio/go-threads/core/service"
	"github.com/textileio/go-threads/core/thread"
)

type SmartBlockType uint64

const (
	SmartBlockTypePage      SmartBlockType = 0x10
	SmartBlockTypeDashboard SmartBlockType = 0x20
)

type SmartBlock struct {
	thread thread.Info
	node   *Anytype
}

type threadVersionSnapshot struct {
	Data []byte
}

func init() {
	cbornode.RegisterCborType(threadVersionSnapshot{})
}

func (s *threadVersionSnapshot) BlockWithMeta() (*storage.BlockWithMeta, error) {
	var blockWithMeta storage.BlockWithMeta
	err := proto.Unmarshal(s.Data, &blockWithMeta)
	if err != nil {
		return nil, err
	}

	return &blockWithMeta, nil
}

func (s *threadVersionSnapshot) BlockMetaOnly() (*storage.BlockMetaOnly, error) {
	var blockWithMeta storage.BlockMetaOnly
	err := proto.Unmarshal(s.Data, &blockWithMeta)
	if err != nil {
		return nil, err
	}

	return &blockWithMeta, nil
}

func (smartBlock *SmartBlock) GetThread() thread.Info {
	return smartBlock.thread
}

func (smartBlock *SmartBlock) GetType() SmartBlockType {
	id := smartBlock.thread.ID.KeyString()
	v := smartBlock.thread.ID.Variant()
	fmt.Println(v)
	// skip version
	_, n := uvarint(id)
	// skip variant
	_, n2 := uvarint(id[n:])
	blockType, _ := uvarint(id[n+n2:])

	return SmartBlockType(blockType)
}

func (smartBlock *SmartBlock) GetId() string {
	return smartBlock.thread.ID.String()
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

func (smartBlock *SmartBlock) getVersionTime(event service.Event) (*types.Timestamp, error) {
	header, err := event.GetHeader(context.TODO(), smartBlock.node.ts, smartBlock.thread.ReadKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get headers: %w", err)
	}

	versionTime, err := header.Time()
	if err != nil {
		return nil, fmt.Errorf("failed to get record time from headers: %w", err)
	}

	versionTimePB, err := types.TimestampProto(*versionTime)
	if err != nil {
		return nil, err
	}

	return versionTimePB, nil
}

func (smartBlock *SmartBlock) getVersionSnapshotEvent(id string) (service.Event, error) {
	vid, err := cid.Parse(id)
	if err != nil {
		return nil, err
	}

	rec, err := smartBlock.node.ts.GetRecord(context.TODO(), smartBlock.thread.ID, vid)
	if err != nil {
		return nil, err
	}

	if smartBlock.thread.ReadKey == nil {
		return nil, fmt.Errorf("no read key")
	}
	event, err := cbor.EventFromRecord(context.TODO(), smartBlock.node.ts, rec)
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)

	}

	return event, nil
}

func (smartBlock *SmartBlock) GetVersion(id string) (BlockVersion, error) {
	event, err := smartBlock.getVersionSnapshotEvent(id)
	if err != nil {
		return nil, err
	}

	node, err := event.GetBody(context.TODO(), smartBlock.node.ts, smartBlock.thread.ReadKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get record body: %w", err)
	}
	m := new(threadVersionSnapshot)
	err = cbornode.DecodeInto(node.RawData(), m)
	if err != nil {
		return nil, fmt.Errorf("incorrect record type: %w", err)
	}

	model, err := m.BlockWithMeta()
	if err != nil {
		return nil, fmt.Errorf("failed to decode pb block version: %w", err)
	}

	time, err := smartBlock.getVersionTime(event)
	if err != nil {
		return nil, fmt.Errorf("failed to decode pb block version: %w", err)
	}
	// todo: how to get creator peer id?
	version := &SmartBlockVersion{model: model, versionId: id, date: time, user: "<todo>"}
	//err = version.addMissingFiles()
	//if err != nil {
	//	return nil, err
	//}

	return version, nil
}

func (smartBlock *SmartBlock) GetVersionMeta(id string) (BlockVersionMeta, error) {
	event, err := smartBlock.getVersionSnapshotEvent(id)
	if err != nil {
		return nil, err
	}

	node, err := event.GetBody(context.TODO(), smartBlock.node.ts, smartBlock.thread.ReadKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get record body: %w", err)
	}
	m := new(threadVersionSnapshot)
	err = cbornode.DecodeInto(node.RawData(), m)
	if err != nil {
		return nil, fmt.Errorf("incorrect record type: %w", err)
	}

	model, err := m.BlockMetaOnly()
	if err != nil {
		return nil, fmt.Errorf("failed to decode pb block version: %w", err)
	}

	time, err := smartBlock.getVersionTime(event)
	if err != nil {
		return nil, fmt.Errorf("failed to decode pb block version: %w", err)
	}

	// todo: how to get creator peer id?
	version := &SmartBlockVersionMeta{model: model, versionId: id, date: time, user: "<todo>"}

	return version, nil
}

func (smartBlock *SmartBlock) GetVersions(offset string, limit int, metaOnly bool) (versions []BlockVersion, err error) {
	var head cid.Cid

	var offsetTime *time.Time
	if offset != "" {
		head, err = cid.Decode(offset)
		if err != nil {
			return nil, err
		}
		rec, err2 := smartBlock.node.ts.GetRecord(context.TODO(), smartBlock.thread.ID, head)
		if err2 != nil {
			err = err2
			return nil, err
		}
		event, err2 := cbor.EventFromRecord(context.TODO(), smartBlock.node.ts, rec)
		if err2 != nil {
			err = err2
			return
		}

		header, err2 := event.GetHeader(context.TODO(), smartBlock.node.ts, smartBlock.thread.ReadKey)
		if err2 != nil {
			err = err2
			return
		}

		offsetTime, err = header.Time()
		if err != nil {
			return
		}
	}

	records, err := smartBlock.node.traverseLogs(context.TODO(), smartBlock.thread, offsetTime, limit)
	if err != nil {
		return
	}

	for _, rec := range records {
		event, err := cbor.EventFromRecord(context.TODO(), smartBlock.node.ts, rec)
		if err != nil {
			return nil, fmt.Errorf("failed to get event: %w", err)
		}

		node, err := event.GetBody(context.TODO(), smartBlock.node.ts, smartBlock.thread.ReadKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get record body: %w", err)
		}
		m := new(threadVersionSnapshot)
		err = cbornode.DecodeInto(node.RawData(), m)
		if err != nil {
			return nil, fmt.Errorf("incorrect record type: %w", err)
		}

		model, err := m.BlockWithMeta()
		if err != nil {
			return nil, fmt.Errorf("failed to decode pb block version: %w", err)
		}

		t, err := types.TimestampProto(rec.Date)
		if err != nil {
			return nil, fmt.Errorf("can't convert tme to pb: %w", err)
		}

		versions = append(versions, &SmartBlockVersion{model: model, versionId: rec.Cid().String(), date: t, user: "<todo>"})
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
		switch smartBlock.GetType() {
		case SmartBlockTypeDashboard:
			if _, ok := block.Content.(*model.BlockContentOfDashboard); !ok {
				return nil, fmt.Errorf("unxpected smartblock type")
			}
		case SmartBlockTypePage:
			if _, ok := block.Content.(*model.BlockContentOfPage); !ok {
				return nil, fmt.Errorf("unxpected smartblock type")
			}
		default:
			return nil, fmt.Errorf("for now you can only add smartblocks")
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

	body, err2 := cbornode.WrapObject(&threadVersionSnapshot{Data: newVersionB}, mh.SHA2_256, -1)
	if err2 != nil {
		err = err2
		return
	}

	rec, err2 := smartBlock.node.ts.CreateRecord(context.TODO(), smartBlock.thread.ID, body)
	if err2 != nil {
		err = err2
		return
	}

	event, err2 := cbor.EventFromRecord(context.TODO(), smartBlock.node.ts, rec.Value())
	if err2 != nil {
		err = err2
		return
	}

	header, err2 := event.GetHeader(context.TODO(), smartBlock.node.ts, smartBlock.thread.ReadKey)
	if err2 != nil {
		err = err2
		return
	}

	msgTime, err2 := header.Time()
	if err2 != nil {
		err = err2
		return
	}

	versionId = rec.LogID().String()
	log.Debugf("SmartBlock.addVersion: blockId = %s newVersionId = %s", smartBlock.GetId(), versionId)
	user = smartBlock.node.account.Address()
	date, err = types.TimestampProto(*msgTime)
	if err != nil {
		return
	}

	return
}

// NewBlock should be used as constructor for the new block
func (smartBlock *SmartBlock) NewBlock(block model.Block) (Block, error) {
	if block.Content == nil {
		return nil, fmt.Errorf("content not set")
	}

	var smartBlockType SmartBlockType
	switch block.Content.(type) {
	case *model.BlockContentOfPage:
		smartBlockType = SmartBlockTypePage

	case *model.BlockContentOfDashboard:
		smartBlockType = SmartBlockTypeDashboard

	}
	if smartBlockType != 0 {
		thrd, err := smartBlock.node.newBlockThread(smartBlockType)
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
	switch smartBlock.GetType() {
	case SmartBlockTypeDashboard:
		content = &model.BlockContentOfDashboard{Dashboard: &model.BlockContentDashboard{}}
	case SmartBlockTypePage:
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

// Version of varint function that work with a string rather than
// []byte to avoid unnecessary allocation

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license as given at https://golang.org/LICENSE

// uvarint decodes a uint64 from buf and returns that value and the
// number of characters read (> 0). If an error occurred, the value is 0
// and the number of bytes n is <= 0 meaning:
//
// 	n == 0: buf too small
// 	n  < 0: value larger than 64 bits (overflow)
// 	        and -n is the number of bytes read
//
func uvarint(buf string) (uint64, int) {
	var x uint64
	var s uint
	// we have a binary string so we can't use a range loope
	for i := 0; i < len(buf); i++ {
		b := buf[i]
		if b < 0x80 {
			if i > 9 || i == 9 && b > 1 {
				return 0, -(i + 1) // overflow
			}
			return x | uint64(b)<<s, i + 1
		}
		x |= uint64(b&0x7f) << s
		s += 7
	}
	return 0, 0
}
