package core

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sort"
	"time"

	"github.com/anytypeio/go-slip21"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/textileio/go-threads/cbor"
	corenet "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/crypto/symmetric"
)

type threadDerivedIndex uint32

const (
	threadDerivedIndexProfilePage   threadDerivedIndex = 0
	threadDerivedIndexHomeDashboard threadDerivedIndex = 1
	threadDerivedIndexArchive       threadDerivedIndex = 2
	threadDerivedIndexAccount       threadDerivedIndex = 3

	anytypeThreadSymmetricKeyPathPrefix = "m/SLIP-0021/anytype"
	// TextileAccountPathFormat is a path format used for Anytype keypair
	// derivation as described in SEP-00XX. Use with `fmt.Sprintf` and `DeriveForPath`.
	// m/SLIP-0021/anytype/<predefined_thread_index>/%d/<label>
	anytypeThreadPathFormat = anytypeThreadSymmetricKeyPathPrefix + `/%d/%s`

	anytypeThreadFollowKeySuffix = `follow`
	anytypeThreadReadKeySuffix   = `read`
	anytypeThreadIdKeySuffix     = `id`
)

var threadDerivedIndexToThreadName = map[threadDerivedIndex]string{
	threadDerivedIndexProfilePage:   "profile",
	threadDerivedIndexHomeDashboard: "home",
	threadDerivedIndexArchive:       "archive",
}

var threadDerivedIndexToSmartblockType = map[threadDerivedIndex]SmartBlockType{
	threadDerivedIndexProfilePage:   SmartBlockTypePage,
	threadDerivedIndexHomeDashboard: SmartBlockTypeDashboard,
	threadDerivedIndexArchive:       SmartBlockTypeArchive,
}

func (a *Anytype) deriveKeys(index threadDerivedIndex) (follow *symmetric.Key, read *symmetric.Key, err error) {
	accountSeed, err2 := a.account.Raw()
	if err2 != nil {
		err = err2
		return
	}

	nodeKey, err2 := slip21.DeriveForPath(fmt.Sprintf(anytypeThreadPathFormat, index, anytypeThreadFollowKeySuffix), accountSeed)
	if err2 != nil {
		err = err2
		return
	}

	follow, err = symmetric.FromBytes(append(nodeKey.SymmetricKey()))
	if err != nil {
		return
	}

	nodeKey, err = slip21.DeriveForPath(fmt.Sprintf(anytypeThreadPathFormat, index, anytypeThreadReadKeySuffix), accountSeed)
	if err != nil {
		return
	}

	read, err = symmetric.FromBytes(nodeKey.SymmetricKey())
	if err != nil {
		return
	}

	return
}

func (a *Anytype) deriveThreadId(index threadDerivedIndex) (thread.ID, error) {
	accountSeed, err := a.account.Raw()
	if err != nil {
		return thread.Undef, err
	}

	node, err := slip21.DeriveForPath(fmt.Sprintf(anytypeThreadPathFormat, index, anytypeThreadIdKeySuffix), accountSeed)
	if err != nil {
		return thread.Undef, err
	}

	// we use symmetric key because it is also has the size of 32 bytes
	return threadIDFromRandom(thread.Raw, threadDerivedIndexToSmartblockType[index], node.SymmetricKey())
}

func (a *Anytype) predefinedThreadByName(name string) (thread.Info, error) {
	for index, tname := range threadDerivedIndexToThreadName {
		if name == tname {
			return a.predefinedThreadWithIndex(index)
		}
	}

	return thread.Info{}, fmt.Errorf("thread not found")
}

func (a *Anytype) predefinedThreadWithIndex(index threadDerivedIndex) (thread.Info, error) {
	id, err := a.deriveThreadId(index)
	if err != nil {
		return thread.Info{}, err
	}

	return a.t.GetThread(context.TODO(), id)
}

func (a *Anytype) predefinedThreadAdd(index threadDerivedIndex, mustSyncSnapshotIfNotExist bool) (thread.Info, error) {
	id, err := a.deriveThreadId(index)
	if err != nil {
		return thread.Info{}, err
	}

	thrd, err := a.t.GetThread(context.TODO(), id)
	if err == nil && thrd.Key.Service() != nil {
		return thrd, nil
	}

	readKey, followKey, err := a.deriveKeys(index)
	if err != nil {
		return thread.Info{}, err
	}

	thrd, err = a.t.CreateThread(context.TODO(),
		id,
		corenet.ThreadKey(thread.NewKey(followKey, readKey)),
		corenet.LogKey(a.device))
	if err != nil {
		return thread.Info{}, err
	}

	if mustSyncSnapshotIfNotExist {
		fmt.Printf("pull thread %s %p\n", id, a.t)
		err := a.t.PullThread(context.TODO(), id)
		if err != nil {
			return thread.Info{}, fmt.Errorf("failed to pull thread: %w", err)
		}
	}

	return thrd, nil
}

type RecordWithMetadata struct {
	corenet.Record
	Date   time.Time
	PubKey crypto.PubKey
}

func (a *Anytype) traverseFromCid(ctx context.Context, thrd thread.Info, li thread.LogInfo, before *time.Time, limit int) ([]RecordWithMetadata, error) {
	var records []RecordWithMetadata
	// todo: filter by record type
	var m = make(map[cid.Cid]struct{})

	pubKey, err := li.ID.ExtractPublicKey()
	if err != nil {
		return nil, err
	}

	for _, head := range li.Heads {
		var recordsPerHead []RecordWithMetadata

		rid := head
		for {
			if _, exists := m[rid]; exists {
				break
			}
			m[rid] = struct{}{}
			rec, err := a.t.GetRecord(ctx, thrd.ID, rid)
			if err != nil {
				return nil, err
			}

			event, err := cbor.EventFromRecord(ctx, a.t, rec)
			if err != nil {
				return nil, err
			}

			header, err := event.GetHeader(ctx, a.t, thrd.Key.Read())
			if err != nil {
				return nil, err
			}

			recordTime, err := header.Time()
			if err != nil {
				return nil, err
			}

			if before != nil && recordTime.After(*before) {
				continue
			}

			recordsPerHead = append(recordsPerHead, RecordWithMetadata{rec, *recordTime, pubKey})
			if len(recordsPerHead) == limit {
				break
			}

			if !rec.PrevID().Defined() {
				break
			}

			rid = rec.PrevID()
		}

		records = append(records, recordsPerHead...)
	}
	return records, nil
}

func (a *Anytype) traverseLogs(ctx context.Context, thrdId thread.ID, before *time.Time, limit int) ([]RecordWithMetadata, error) {
	var allRecords []RecordWithMetadata
	thrd, err := a.t.GetThread(context.Background(), thrdId)
	if err != nil {
		return nil, err
	}

	for _, log := range thrd.Logs {
		records, err := a.traverseFromCid(ctx, thrd, log, before, limit)
		if err != nil {
			continue
		}

		allRecords = append(allRecords, records...)
	}

	sort.Slice(allRecords, func(i, j int) bool {
		return allRecords[i].Date.After(allRecords[j].Date)
	})

	if len(allRecords) < limit {
		limit = len(allRecords)
	}

	return allRecords[0:limit], nil
}

func threadIDFromRandom(variant thread.Variant, blockType SmartBlockType, rnd []byte) (thread.ID, error) {
	rndlen := len(rnd)
	// two 8 bytes (max) numbers plus num
	buf := make([]byte, 2*binary.MaxVarintLen64+rndlen)
	n := binary.PutUvarint(buf, thread.V1)
	n += binary.PutUvarint(buf[n:], uint64(variant))
	n += binary.PutUvarint(buf[n:], uint64(blockType))

	cn := copy(buf[n:], rnd)
	if cn != rndlen {
		return thread.Undef, fmt.Errorf("copy length is inconsistent")
	}

	return thread.Cast(buf[:n+rndlen])
}

func newThreadID(variant thread.Variant, blockType SmartBlockType) (thread.ID, error) {
	rnd := make([]byte, 32)
	_, err := rand.Read(rnd)
	if err != nil {
		panic("random read failed")
	}

	rndlen := len(rnd)

	// two 8 bytes (max) numbers plus rnd
	buf := make([]byte, 2*binary.MaxVarintLen64+rndlen)
	n := binary.PutUvarint(buf, thread.V1)
	n += binary.PutUvarint(buf[n:], uint64(variant))
	n += binary.PutUvarint(buf[n:], uint64(blockType))

	cn := copy(buf[n:], rnd)
	if cn != rndlen {
		return thread.Undef, fmt.Errorf("copy length is inconsistent")
	}

	return thread.Cast(buf[:n+rndlen])
}
