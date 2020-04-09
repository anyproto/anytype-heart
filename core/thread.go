package core

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/anytypeio/go-anytype-library/vclock"
	"github.com/anytypeio/go-slip21"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
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
		corenet.WithThreadKey(thread.NewKey(followKey, readKey)),
		corenet.WithLogKey(a.device))
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

type SnapshotWithMetadata struct {
	storage.SmartBlockSnapshot
	Creator string
}

func (a *Anytype) traverseFromCid(ctx context.Context, thrd thread.Info, li thread.LogInfo, before vclock.VClock, limit int) ([]SnapshotWithMetadata, error) {
	var snapshots []SnapshotWithMetadata
	// todo: filter by record type
	var m = make(map[cid.Cid]struct{})

	pubKey, err := li.ID.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	rid := li.Head
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

		node, err := event.GetBody(context.TODO(), a.t, thrd.Key.Read())
		if err != nil {
			return nil, fmt.Errorf("failed to get record body: %w", err)
		}
		m := new(signedPbPayload)
		err = cbornode.DecodeInto(node.RawData(), m)
		if err != nil {
			return nil, fmt.Errorf("incorrect record type: %w", err)
		}

		err = m.Verify(pubKey)
		if err != nil {
			return nil, err
		}

		var snapshot = storage.SmartBlockSnapshot{}
		err = m.Unmarshal(&snapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to decode pb block snapshot: %w", err)
		}

		if !before.IsNil() && vclock.NewFromMap(snapshot.State).Compare(before, vclock.Descendant) {
			log.Debugf("traverseFromCid skip Descendant: %+v > %+v", snapshot.State, before)
			continue
		}

		snapshots = append(snapshots, SnapshotWithMetadata{snapshot, m.AccAddr})
		if len(snapshots) == limit {
			break
		}

		if !rec.PrevID().Defined() {
			break
		}

		rid = rec.PrevID()
	}

	return snapshots, nil
}

func (a *Anytype) traverseLogs(ctx context.Context, thrdId thread.ID, before vclock.VClock, limit int) ([]SnapshotWithMetadata, error) {
	var allSnapshots []SnapshotWithMetadata
	thrd, err := a.t.GetThread(context.Background(), thrdId)
	if err != nil {
		return nil, err
	}

	for _, log := range thrd.Logs {
		snapshots, err := a.traverseFromCid(ctx, thrd, log, before, limit)
		if err != nil {
			continue
		}

		allSnapshots = append(allSnapshots, snapshots...)
	}

	sort.Slice(allSnapshots, func(i, j int) bool {
		// sort from the newest to the oldest snapshot
		stateI := vclock.NewFromMap(allSnapshots[i].State)
		stateJ := vclock.NewFromMap(allSnapshots[j].State)
		anc := stateI.Compare(stateJ, vclock.Ancestor)
		if anc {
			return true
		}

		if stateI.Compare(stateJ, vclock.Descendant) {
			return false
		}

		// in case of concurrent changes choose the hash with greater hash first
		return stateI.Hash() > stateJ.Hash()
	})

	if len(allSnapshots) < limit {
		limit = len(allSnapshots)
	}

	return allSnapshots[0:limit], nil
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
