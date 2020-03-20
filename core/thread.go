package core

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sort"
	"time"

	"github.com/anytypeio/go-anytype-library/wallet"
	"github.com/anytypeio/go-slip21"
	"github.com/ipfs/go-cid"
	"github.com/textileio/go-textile/keypair"
	twallet "github.com/textileio/go-textile/wallet"
	"github.com/textileio/go-threads/cbor"
	corenet "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/crypto/symmetric"

	"github.com/textileio/go-threads/core/thread"
)

type threadDerivedIndex uint32

const (
	threadDerivedIndexProfilePage      threadDerivedIndex = 0
	threadDerivedIndexHomeDashboard    threadDerivedIndex = 1
	threadDerivedIndexArchiveDashboard threadDerivedIndex = 2
	threadDerivedIndexAccount      	   threadDerivedIndex = 3


	// AnytypeThreadPathLogKeyFormat is a path format used for Anytype predefined thread log keypair
	// Use with `fmt.Sprintf` and `DeriveForPath`.
	// m/44'/607'/<predefined_thread_index>'/%d'
	AnytypeThreadPathLogKeyFormat = wallet.AnytypeAccountPrefix + "/%d'/%d'/1"

	anytypeThreadSymmetricKeyPathPrefix = "m/SLIP-0021/anytype"
	// TextileAccountPathFormat is a path format used for Anytype keypair
	// derivation as described in SEP-00XX. Use with `fmt.Sprintf` and `DeriveForPath`.
	// m/SLIP-0021/anytype/<predefined_thread_index>/%d/<label>
	anytypeThreadPathFormat = anytypeThreadSymmetricKeyPathPrefix + `/%d/%s`

	anytypeThreadFollowKeySuffix = `follow`
	anytypeThreadReadKeySuffix   = `read`
	anytypeThreadIdKeySuffix     = `id`
	anytypeThreadNonceSuffix     = `nonce`
)

var threadDerivedIndexToThreadName = map[threadDerivedIndex]string{
	threadDerivedIndexProfilePage:      "profile",
	threadDerivedIndexHomeDashboard:    "home",
	threadDerivedIndexArchiveDashboard: "archive",
}

var threadDerivedIndexToSmartblockType = map[threadDerivedIndex]SmartBlockType{
	threadDerivedIndexProfilePage:      SmartBlockTypePage,
	threadDerivedIndexHomeDashboard:    SmartBlockTypeDashboard,
	threadDerivedIndexArchiveDashboard: SmartBlockTypeDashboard,
}

func (a *Anytype) deriveKeys(index threadDerivedIndex) (follow *symmetric.Key, read *symmetric.Key, log *keypair.Full, err error) {
	accountSeed, err2 := a.account.Raw()
	if err2 != nil {
		err = err2
		return
	}

	master, err2 := twallet.NewMasterKey(accountSeed)
	if err2 != nil {
		err = err2
		return
	}

	logKey, err2 := master.Derive(uint32(index) + twallet.FirstHardenedIndex)
	if err2 != nil {
		err = err2
		return
	}
	log, err = keypair.FromRawSeed(logKey.RawSeed())
	if err != nil {
		return
	}

	nodeKey, err2 := slip21.DeriveForPath(fmt.Sprintf(anytypeThreadPathFormat, index, anytypeThreadFollowKeySuffix), accountSeed)
	if err2 != nil {
		err = err2
		return
	}

	nodeNonce, err2 := slip21.DeriveForPath(fmt.Sprintf(anytypeThreadPathFormat, index, anytypeThreadFollowKeySuffix)+"/"+anytypeThreadNonceSuffix, accountSeed)
	if err2 != nil {
		err = err2
		return
	}

	follow, err = symmetric.FromBytes(append(nodeKey.SymmetricKey(), nodeNonce.SymmetricKey()[0:12]...))
	if err != nil {
		return
	}

	nodeKey, err = slip21.DeriveForPath(fmt.Sprintf(anytypeThreadPathFormat, index, anytypeThreadReadKeySuffix), accountSeed)
	if err != nil {
		return
	}

	nodeNonce, err = slip21.DeriveForPath(fmt.Sprintf(anytypeThreadPathFormat, index, anytypeThreadReadKeySuffix)+"/"+anytypeThreadNonceSuffix, accountSeed)
	if err != nil {
		return
	}

	read, err = symmetric.FromBytes(append(nodeKey.SymmetricKey(), nodeNonce.SymmetricKey()[0:12]...))
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

	return a.ts.GetThread(context.TODO(), id)
}

func (a *Anytype) predefinedThreadAdd(index threadDerivedIndex, mustSyncSnapshotIfNotExist bool) (thread.Info, error) {
	id, err := a.deriveThreadId(index)
	if err != nil {
		return thread.Info{}, err
	}

	thrd, err := a.ts.GetThread(context.TODO(), id)
	if err == nil && thrd.FollowKey != nil {
		return thrd, nil
	}

	readKey, followKey, logKeypair, err := a.deriveKeys(index)
	if err != nil {
		return thread.Info{}, err
	}

	logKey, err := logKeypair.LibP2PPrivKey()
	if err != nil {
		return thread.Info{}, err
	}

	thrd, err = a.ts.CreateThread(context.TODO(),
		id,
		corenet.FollowKey(followKey),
		corenet.ReadKey(readKey),
		corenet.LogKey(logKey))
	if err != nil {
		return thread.Info{}, err
	}

	if mustSyncSnapshotIfNotExist {
		fmt.Printf("pull thread %s %p\n", id, a.ts)
		err := a.ts.PullThread(context.TODO(), id)
		if err != nil {
			return thread.Info{}, fmt.Errorf("failed to pull thread: %w", err)
		}
	}

	return thrd, nil
}

type RecordWithMetadata struct {
	corenet.Record
	Date time.Time
}

func (a *Anytype) traverseFromCid(ctx context.Context, thrd thread.Info, heads []cid.Cid, before *time.Time, limit int) ([]RecordWithMetadata, error) {
	var records []RecordWithMetadata
	// todo: filter by record type
	var m = make(map[cid.Cid]struct{})

	for _, head := range heads {
		var recordsPerHead []RecordWithMetadata

		rid := head
		for {
			if _, exists := m[rid]; exists {
				break
			}
			m[rid] = struct{}{}
			rec, err := a.ts.GetRecord(ctx, thrd.ID, rid)
			if err != nil {
				return nil, err
			}

			event, err := cbor.EventFromRecord(ctx, a.ts, rec)
			if err != nil {
				return nil, err
			}

			header, err := event.GetHeader(ctx, a.ts, thrd.ReadKey)
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

			recordsPerHead = append(recordsPerHead, RecordWithMetadata{rec, *recordTime})
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
	thrd, err := a.ts.GetThread(context.Background(), thrdId)
	if err != nil {
		return nil, err
	}

	for _, log := range thrd.Logs {
		records, err := a.traverseFromCid(ctx, thrd, log.Heads, before, limit)
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
