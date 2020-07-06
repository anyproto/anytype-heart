package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/util"
	"github.com/anytypeio/go-anytype-library/vclock"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	mh "github.com/multiformats/go-multihash"
	"github.com/textileio/go-threads/cbor"
	"github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
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
	Type() smartblock.SmartBlockType
	Creator() (string, error)
	BaseSchema() SmartBlockSchema

	GetLogs() ([]SmartblockLog, error)
	GetRecord(ctx context.Context, recordID string) (*SmartblockRecord, error)
	PushRecord(payload proto.Message) (id string, err error)

	SubscribeForRecords(ch chan SmartblockRecordWithLogID) (cancel func(), err error)
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

func (block *smartBlock) Type() smartblock.SmartBlockType {
	t, err := smartblock.SmartBlockTypeFromThreadID(block.thread.ID)
	if err != nil {
		// shouldn't happen as we init the smartblock with an existing thread
		log.Errorf("smartblock has incorrect id(%s), failed to decode type: %s", block.thread.ID.String(), err.Error())
		return 0
	}

	return t
}

func (block *smartBlock) BaseSchema() SmartBlockSchema {
	return smartBlockBaseSchema(block.Type())
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

	for _, snapshot := range snapshotsPB {
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

func (block *smartBlock) PushRecord(payload proto.Message) (id string, err error) {
	payloadB, err := proto.Marshal(payload)
	if err != nil {
		return "", err
	}

	signedPayload, err := newSignedPayload(payloadB, block.node.opts.Account)
	if err != nil {
		return "", err
	}

	body, err := cbornode.WrapObject(signedPayload, mh.SHA2_256, -1)
	if err != nil {
		return "", err
	}

	rec, err := block.node.t.CreateRecord(context.TODO(), block.thread.ID, body)
	if err != nil {
		log.Errorf("failed to create record: %w", err)
		return "", err
	}

	log.Debugf("SmartBlock.PushRecord: blockId = %s", block.ID())
	return rec.Value().Cid().String(), nil
}

func (block *smartBlock) SubscribeForRecords(ch chan SmartblockRecordWithLogID) (cancel func(), err error) {
	doneCh := make(chan struct{})
	chCloseFn := func() {
		doneCh <- struct{}{}
	}

	ctx, ctxCancel := context.WithCancel(context.Background())
	// todo: this is not effective, need to make a single subscribe point for all subscribed threads
	threadsCh, err := block.node.t.Subscribe(ctx, net.WithSubFilter(block.thread.ID))
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %s", err.Error())
	}

	go func() {
		for {
			select {
			case <-doneCh:
				ctxCancel()
				return
			case val, ok := <-threadsCh:
				if !ok {
					return
				}

				rec, err := block.decodeRecord(ctx, val.Value())
				if err != nil {
					log.Errorf("failed to decode thread record: %s", err.Error())
					continue
				}

				ch <- SmartblockRecordWithLogID{
					SmartblockRecord: *rec,
					LogID:            val.LogID().String(),
				}

			}
		}
	}()

	return chCloseFn, nil
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

func (block *smartBlock) GetLogs() ([]SmartblockLog, error) {
	thrd, err := block.node.t.GetThread(context.Background(), block.thread.ID)
	if err != nil {
		return nil, err
	}

	var logs []SmartblockLog
	for _, l := range thrd.Logs {
		var head string
		if l.Head.Defined() {
			head = l.Head.String()
		}

		logs = append(logs, SmartblockLog{
			ID:   l.ID.String(),
			Head: head,
		})
	}

	return logs, nil
}

func (block *smartBlock) decodeRecord(ctx context.Context, rec net.Record) (*SmartblockRecord, error) {
	event, err := cbor.EventFromRecord(ctx, block.node.t, rec)
	if err != nil {
		return nil, err
	}

	node, err := event.GetBody(context.TODO(), block.node.t, block.thread.Key.Read())
	if err != nil {
		return nil, fmt.Errorf("failed to get record body: %w", err)
	}
	m := new(SignedPbPayload)
	err = cbornode.DecodeInto(node.RawData(), m)
	if err != nil {
		return nil, fmt.Errorf("incorrect record type: %w", err)
	}

	err = m.Verify()
	if err != nil {
		return nil, err
	}

	var prevID string
	if rec.PrevID().Defined() {
		prevID = rec.PrevID().String()
	}

	return &SmartblockRecord{
		ID:      rec.Cid().String(),
		PrevID:  prevID,
		Payload: m.Data,
	}, nil
}

func (block *smartBlock) GetRecord(ctx context.Context, recordID string) (*SmartblockRecord, error) {
	rid, err := cid.Decode(recordID)
	if err != nil {
		return nil, err
	}

	rec, err := block.node.t.GetRecord(ctx, block.thread.ID, rid)
	if err != nil {
		return nil, err
	}

	return block.decodeRecord(ctx, rec)
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
	if block.Type() == smartblock.SmartBlockTypeArchive {
		return nil
	}

	storeOutgoingLinks := func(snap *smartBlockSnapshot, linksMap map[string]struct{}) {
		for _, block := range snap.blocks {
			if link := block.GetLink(); link != nil {
				linksMap[link.TargetBlockId] = struct{}{}
			}

			if text := block.GetText(); text != nil && text.Marks != nil {
				for _, m := range text.Marks.Marks {
					if m.Type == model.BlockContentTextMark_Mention {
						linksMap[m.Param] = struct{}{}
					}
				}
			}
		}
	}

	newOutgoingLinks := make(map[string]struct{})
	newSnippet := getSnippet(snap)

	var newOutgoingLinksIds = []string{}
	storeOutgoingLinks(snap, newOutgoingLinks)
	for linkId := range newOutgoingLinks {
		newOutgoingLinksIds = append(newOutgoingLinksIds, linkId)
	}

	return block.node.localStore.Pages.Update(block.ID(), snap.details, newOutgoingLinksIds, &newSnippet)
}

func (block *smartBlock) index() error {
	versions, err := block.GetSnapshots(vclock.Undef, 1, false)
	if err != nil {
		return err
	}

	if len(versions) == 0 {
		return block.indexSnapshot(&smartBlockSnapshot{
			state:    vclock.New(),
			threadID: block.thread.ID,
		})
	}

	lastVersion := versions[0]
	return block.indexSnapshot(&lastVersion)
}
