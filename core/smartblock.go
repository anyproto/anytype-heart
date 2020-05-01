package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/anytypeio/go-anytype-library/util"
	"github.com/anytypeio/go-anytype-library/vclock"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	cbornode "github.com/ipfs/go-ipld-cbor"
	mh "github.com/multiformats/go-multihash"
	"github.com/textileio/go-threads/core/thread"
)

type SmartBlockType uint64

const (
	SmartBlockTypePage        SmartBlockType = 0x10
	SmartBlockTypeProfilePage SmartBlockType = 0x11
	SmartBlockTypeHome        SmartBlockType = 0x20
	SmartBlockTypeArchive     SmartBlockType = 0x30
	SmartBlockTypeDatabase    SmartBlockType = 0x40
	SmartBlockTypeSet         SmartBlockType = 0x41
)

const (
	snippetMinSize = 50
	snippetMaxSize = 300
)

type ProfileThreadEncryptionKeys struct {
	ServiceKey []byte
	ReadKey    []byte
}

func init() {
	cbornode.RegisterCborType(ProfileThreadEncryptionKeys{})
}

// ShouldCreateSnapshot informs if you need to make a snapshot based on deterministic alg
// temporally always returns true
func (block smartBlock) ShouldCreateSnapshot(state vclock.VClock) bool {
	if strings.HasSuffix(state.Hash(), "0") {
		return true
	}

	// return false
	// todo: return false when changes will be implemented
	return true
}

type SmartBlockContentChange struct {
	state vclock.VClock
	// to be discussed
}

type SmartBlockMeta struct {
	Details *types.Struct
}

type SmartBlockMetaChange struct {
	SmartBlockMeta
	state vclock.VClock
}

func (meta *SmartBlockMetaChange) State() vclock.VClock {
	return meta.state
}

func (meta *SmartBlockContentChange) State() vclock.VClock {
	return meta.state
}

type SmartBlockChange struct {
	State   vclock.VClock
	Content *SmartBlockContentChange
	Meta    *SmartBlockMetaChange
}

type SmartBlockVersion struct {
	State    vclock.VClock
	Snapshot SmartBlockSnapshot
	Changes  []SmartBlockChange
}

type SmartBlock interface {
	ID() string
	Type() SmartBlockType
	Creator() (string, error)
	GetLastSnapshot() (SmartBlockSnapshot, error)
	// GetLastDownloadedVersion returns tha last snapshot and all full-downloaded changes
	GetLastDownloadedVersion() (*SmartBlockVersion, error)
	GetSnapshotBefore(state vclock.VClock) (SmartBlockSnapshot, error)

	PushChanges(changes []*SmartBlockChange) (state vclock.VClock, err error)
	ShouldCreateSnapshot(state vclock.VClock) bool
	PushSnapshot(state vclock.VClock, meta *SmartBlockMeta, blocks []*model.Block) (SmartBlockSnapshot, error)
	GetChangesBetween(since vclock.VClock, until vclock.VClock) ([]SmartBlockChange, error)

	SubscribeForChanges(since vclock.VClock, ch chan SmartBlockChange) (cancel func(), err error)
	SubscribeForMetaChanges(since vclock.VClock, ch chan SmartBlockMetaChange) (cancel func(), err error)
	// SubscribeClientEvents provide a way to subscribe for the client-side events e.g. carriage position change
	SubscribeClientEvents(event chan<- proto.Message) (cancelFunc func(), err error)
	// PublishClientEvent gives a way to push the new client-side event e.g. carriage position change
	// notice that you will also get this event in SubscribeForEvents
	PublishClientEvent(event proto.Message) error
}

type smartBlock struct {
	thread thread.Info
	node   *Anytype
}

func (block *smartBlock) Creator() (string, error) {
	return "", fmt.Errorf("to be implemented")
}

func (block *smartBlock) GetLastDownloadedVersion() (*SmartBlockVersion, error) {
	snapshot, err := block.GetLastSnapshot()
	if err != nil {
		return nil, err
	}

	return &SmartBlockVersion{
		State:    snapshot.State(),
		Snapshot: snapshot,
		Changes:  []SmartBlockChange{},
	}, nil
}

func (block *smartBlock) PushChanges(changes []*SmartBlockChange) (state vclock.VClock, err error) {
	// todo: to be implemented
	return vclock.Undef, fmt.Errorf("to be implemented")
}

func (block *smartBlock) GetThread() thread.Info {
	return block.thread
}

func (block *smartBlock) Type() SmartBlockType {
	t, err := SmartBlockTypeFromThreadID(block.thread.ID)
	if err != nil {
		// shouldn't happen as we init the smartblock with an existing thread
		log.Errorf("smartblock has incorrect id(%s), failed to decode type: %s", block.thread.ID.String(), err.Error())
		return 0
	}

	return t
}

func (block *smartBlock) ID() string {
	return block.thread.ID.String()
}

func (block *smartBlock) GetLastSnapshot() (SmartBlockSnapshot, error) {
	versions, err := block.GetSnapshots(vclock.Undef, 1, false)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, ErrBlockSnapshotNotFound
	}

	return versions[0], nil
}

func (block *smartBlock) GetChangesBetween(since vclock.VClock, until vclock.VClock) ([]SmartBlockChange, error) {
	return nil, fmt.Errorf("not implemented")
}

func (block *smartBlock) GetSnapshotBefore(state vclock.VClock) (SmartBlockSnapshot, error) {
	versions, err := block.GetSnapshots(state, 1, false)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, ErrBlockSnapshotNotFound
	}

	return versions[0], nil
}

/*func (block *smartBlock) GetSnapshotMeta(id string) (Sm, error) {
	event, err := block.getSnapshotSnapshotEvent(id)
	if err != nil {
		return nil, err
	}

	service, err := event.GetBody(context.TODO(), block.service.t, block.thread.ReadKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get record body: %w", err)
	}
	m := new(threadSnapshot)
	err = cbornode.DecodeInto(service.RawData(), m)
	if err != nil {
		return nil, fmt.Errorf("incorrect record type: %w", err)
	}

	model, err := m.()
	if err != nil {
		return nil, fmt.Errorf("failed to decode pb block version: %w", err)
	}

	time, err := block.getSnapshotTime(event)
	if err != nil {
		return nil, fmt.Errorf("failed to decode pb block version: %w", err)
	}

	// todo: how to get creator peer id?
	version := &smartBlockSnapshotMeta{model: model, date: time, creator: "<todo>"}

	return version, nil
}*/

func (block *smartBlock) GetSnapshots(offset vclock.VClock, limit int, metaOnly bool) (snapshots []smartBlockSnapshot, err error) {
	snapshotsPB, err := block.node.snapshotTraverseLogs(context.TODO(), block.thread.ID, offset, limit)
	if err != nil {
		return
	}
	block.node.files.KeysCacheMutex.Lock()
	defer block.node.files.KeysCacheMutex.Unlock()
	for _, snapshot := range snapshotsPB {
		for k, v := range snapshot.KeysByHash {
			block.node.files.KeysCache[k] = v.KeysByPath
		}

		snapshots = append(snapshots, smartBlockSnapshot{

			blocks:  snapshot.Blocks,
			details: snapshot.Details,
			state:   vclock.NewFromMap(snapshot.State),
			creator: snapshot.Creator,

			threadID: block.thread.ID,
			recordID: snapshot.RecordID,
			eventID:  snapshot.EventID,
			key:      block.thread.Key.Read(),

			node: block.node,
		})
	}

	return
}

func (block *smartBlock) getAllFileKeys(blocks []*model.Block) map[string]*storage.FileKeys {
	fileKeys := make(map[string]*storage.FileKeys)
	block.node.files.KeysCacheMutex.RLock()
	defer block.node.files.KeysCacheMutex.RUnlock()

	for _, b := range blocks {
		if file, ok := b.Content.(*model.BlockContentOfFile); ok {
			if file.File.Hash == "" {
				continue
			}

			if keys, exists := block.node.files.KeysCache[file.File.Hash]; exists {
				fileKeys[file.File.Hash] = &storage.FileKeys{keys}
			} else {
				// in case we don't have keys cached fot this file
				fileKeysRestored, err := block.node.files.FileRestoreKeys(context.TODO(), file.File.Hash)
				if err != nil {
					log.Errorf("failed to restore file keys: %w", err)
				} else {
					fileKeys[file.File.Hash] = &storage.FileKeys{fileKeysRestored}
				}
			}
		}
	}

	return fileKeys
}

func (block *smartBlock) PushSnapshot(state vclock.VClock, meta *SmartBlockMeta, blocks []*model.Block) (SmartBlockSnapshot, error) {
	// todo: we don't need to increment here
	// temporally increment the vclock until we don't have changes implemented
	state.Increment(block.thread.GetOwnLog().ID.String())

	model := &storage.SmartBlockSnapshot{
		State:      state.Map(),
		ClientTime: time.Now().Unix(),
		KeysByHash: block.getAllFileKeys(blocks),
	}

	if meta != nil && meta.Details != nil {
		model.Details = meta.Details
	}

	if blocks != nil {
		model.Blocks = blocks
	}

	var err error
	recID, user, date, err := block.pushSnapshot(model)
	if err != nil {
		return nil, err
	}

	snapshot := &smartBlockSnapshot{
		blocks:  model.Blocks,
		details: model.Details,

		state:    state,
		threadID: block.thread.ID,
		recordID: recID,

		eventID: cid.Cid{}, // todo: extract eventId
		key:     block.thread.Key.Read(),
		creator: user,
		date:    date,
		node:    block.node,
	}

	err = block.indexSnapshot(snapshot)
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

func hasCafeLog(logsinfo []thread.LogInfo) bool {
	/*for _, li := range logsinfo {
		//if li.
	}*/
	return false
}

func (block *smartBlock) pushSnapshot(newSnapshot *storage.SmartBlockSnapshot) (recID cid.Cid, user string, date *types.Timestamp, err error) {
	var newSnapshotB []byte
	newSnapshotB, err = proto.Marshal(newSnapshot)
	if err != nil {
		return
	}

	payload, err2 := newSignedPayload(newSnapshotB, block.node.opts.Device, block.node.opts.Account)
	if err2 != nil {
		err = err2
		return
	}

	body, err2 := cbornode.WrapObject(payload, mh.SHA2_256, -1)
	if err2 != nil {
		err = err2
		return
	}

	_, err = block.node.t.CreateRecord(context.TODO(), block.thread.ID, body)
	if err != nil {
		log.Errorf("failed to create record: %w", err)
		return
	}

	log.Debugf("SmartBlock.addSnapshot: blockId = %s", block.ID())
	return
}

func (block *smartBlock) EmptySnapshot() SmartBlockSnapshot {
	return &smartBlockSnapshot{
		blocks: []*model.Block{},

		threadID: block.thread.ID,
		node:     block.node,
	}
}

func (block *smartBlock) SubscribeForChanges(since vclock.VClock, ch chan SmartBlockChange) (cancel func(), err error) {
	chCloseFn := func() { close(ch) }

	//todo: to be implemented
	return chCloseFn, nil
}

func (block *smartBlock) SubscribeForMetaChanges(since vclock.VClock, ch chan SmartBlockMetaChange) (cancel func(), err error) {
	doneCh := make(chan struct{})
	chCloseFn := func() {
		doneCh <- struct{}{}
	}

	log.Infof("SubscribeForMetaChanges %s", block.ID())

	go func() {
		listener := block.node.smartBlockChanges.Listen()

		var lastDetails *types.Struct
		lastSnap, _ := block.GetLastSnapshot()
		if lastSnap != nil {
			lastMeta, _ := lastSnap.Meta()
			if lastMeta != nil {
				lastDetails = lastMeta.Details
				if lastSnap.State().Compare(since, vclock.Ancestor) {
					ch <- SmartBlockMetaChange{
						SmartBlockMeta: *lastMeta,
						state:          lastSnap.State()}
				}
			}
		}

		for {
			select {
			case <-doneCh:
				listener.Discard()
				close(ch)
				return
			case val, ok := <-listener.Channel():
				if !ok {
					close(ch)
					return
				}

				if tid, ok := val.(thread.ID); ok {
					if tid != block.thread.ID {
						continue
					}
					log.Infof("got thread update... %s", tid.String())

					newSnap, _ := block.GetLastSnapshot()
					if newSnap != nil {
						newMeta, _ := newSnap.Meta()
						if newMeta != nil && newMeta.Details != nil {
							if newMeta.Details.Compare(lastDetails) != 0 {
								log.Infof("details changed! %s", tid.String())
								ch <- SmartBlockMetaChange{
									SmartBlockMeta: *newMeta,
									state:          newSnap.State()}

								lastDetails = newMeta.Details
							}
						}
					}
				}
			}
		}
	}()

	return chCloseFn, nil
}

func (block *smartBlock) SubscribeClientEvents(events chan<- proto.Message) (cancelFunc func(), err error) {
	//todo: to be implemented
	return func() { close(events) }, nil
}

func (block *smartBlock) PublishClientEvent(event proto.Message) error {
	//todo: to be implemented
	return fmt.Errorf("not implemented")
}

func getSnippet(snap *smartBlockSnapshot) string {
	var s string
	for _, block := range snap.blocks {
		if text := block.GetText(); text != nil {
			if s != "" {
				s += " "
			}
			s += text.Text
			if len(s) >= snippetMinSize {
				break
			}
		}
	}

	return util.TruncateText(s, snippetMaxSize)
}

func (block *smartBlock) indexSnapshot(snap *smartBlockSnapshot) error {
	if block.Type() != SmartBlockTypePage {
		return nil
	}

	fromStateM, err := block.node.localStore.Pages.GetStateByID(block.ID())
	if err != nil && err != ds.ErrNotFound {
		return err
	}

	var fromState vclock.VClock
	if fromStateM != nil && fromStateM.State != nil {
		fromState = vclock.NewFromMap(fromStateM.State)
		if !snap.State().Compare(fromState, vclock.Ancestor) {
			return nil
		}
	}

	storeOutgoingLinks := func(snap *smartBlockSnapshot, linksMap map[string]struct{}) {
		for _, block := range snap.blocks {
			if link := block.GetLink(); link != nil {
				linksMap[link.TargetBlockId] = struct{}{}
			}
		}
	}

	prevOutgoingLinks := make(map[string]struct{})
	newOutgoingLinks := make(map[string]struct{})
	var oldSnippet string
	var oldDetails *types.Struct
	newSnippet := getSnippet(snap)

	if !fromState.IsNil() {
		prevSnaps, err := block.GetSnapshots(fromState, 1, false)
		if err != nil && err != ErrBlockSnapshotNotFound {
			return err
		} else if prevSnaps != nil && len(prevSnaps) > 0 {
			prevSnap := prevSnaps[0]
			storeOutgoingLinks(&prevSnap, prevOutgoingLinks)
			oldSnippet = getSnippet(&prevSnap)
			oldDetails = prevSnaps[0].details
		}
	}

	storeOutgoingLinks(snap, newOutgoingLinks)

	var linksToRemove []string
	var linksToAdd []string
	for link, _ := range newOutgoingLinks {
		if _, exists := prevOutgoingLinks[link]; !exists {
			linksToAdd = append(linksToAdd, link)
		}
	}

	for link, _ := range prevOutgoingLinks {
		if _, exists := newOutgoingLinks[link]; !exists {
			linksToRemove = append(linksToRemove, link)
		}
	}

	var changeSnippet string
	if oldSnippet != newSnippet {
		if newSnippet == "" {
			// workaround to send non-empty string
			newSnippet = " "
		}
		changeSnippet = newSnippet
	}

	var changedDetails *model.PageDetails
	if oldDetails == nil || oldDetails.Compare(snap.details) != 0 {
		changedDetails = &model.PageDetails{snap.details}
	}

	return block.node.localStore.Pages.Update(&model.State{snap.State().Map()}, block.ID(), linksToAdd, linksToRemove, changeSnippet, changedDetails)
}

func (block *smartBlock) index() error {
	versions, err := block.GetSnapshots(vclock.Undef, 1, false)
	if err != nil {
		return err
	}

	if len(versions) == 0 {
		block.indexSnapshot(&smartBlockSnapshot{
			state:    vclock.New(),
			threadID: block.thread.ID,
		})
		return nil
	}

	lastVersion := versions[0]
	return block.indexSnapshot(&lastVersion)
}

// Snapshot of varint function that work with a string rather than
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

func SmartBlockTypeFromID(id string) (SmartBlockType, error) {
	tid, err := thread.Decode(id)
	if err != nil {
		return 0, err
	}

	return SmartBlockTypeFromThreadID(tid)
}

func SmartBlockTypeFromThreadID(tid thread.ID) (SmartBlockType, error) {
	rawid := tid.KeyString()
	// skip version
	_, n := uvarint(rawid)
	// skip variant
	_, n2 := uvarint(rawid[n:])
	blockType, _ := uvarint(rawid[n+n2:])

	return SmartBlockType(blockType), nil
}
