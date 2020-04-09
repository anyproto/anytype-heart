package core

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"

	"github.com/anytypeio/go-anytype-library/wallet"
	"github.com/anytypeio/go-slip21"
	"github.com/libp2p/go-libp2p-core/crypto"
	corenet "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/crypto/symmetric"
)

type threadDerivedIndex uint32

const (
	// profile page is publicly accessible as service/read keys derived from account public key
	threadDerivedIndexProfilePage   threadDerivedIndex = 0
	threadDerivedIndexHomeDashboard threadDerivedIndex = 1
	threadDerivedIndexArchive       threadDerivedIndex = 2
	threadDerivedIndexAccount       threadDerivedIndex = 3

	anytypeThreadSymmetricKeyPathPrefix = "m/SLIP-0021/anytype"
	// TextileAccountPathFormat is a path format used for Anytype keypair
	// derivation as described in SEP-00XX. Use with `fmt.Sprintf` and `DeriveForPath`.
	// m/SLIP-0021/anytype/<predefined_thread_index>/%d/<label>
	anytypeThreadPathFormat = anytypeThreadSymmetricKeyPathPrefix + `/%d/%s`

	anytypeThreadServiceKeySuffix = `service`
	anytypeThreadReadKeySuffix    = `read`
	anytypeThreadIdKeySuffix      = `id`
)

var threadDerivedIndexToThreadName = map[threadDerivedIndex]string{
	threadDerivedIndexProfilePage: "profile",
	threadDerivedIndexHome:        "home",
	threadDerivedIndexArchive:     "archive",
}
var threadDerivedIndexToSmartblockType = map[threadDerivedIndex]SmartBlockType{
	threadDerivedIndexProfilePage: SmartBlockTypeProfilePage,
	threadDerivedIndexHome:        SmartBlockTypeHome,
	threadDerivedIndexArchive:     SmartBlockTypeArchive,
}

func ProfileThreadIDFromAccountPublicKey(pubk crypto.PubKey) (thread.ID, error) {
	accountPub, err := pubk.Raw()
	if err != nil {
		return thread.Undef, err
	}

	node, err := slip21.DeriveForPath(fmt.Sprintf(anytypeThreadPathFormat, threadDerivedIndexProfilePage, anytypeThreadIdKeySuffix), accountPub)
	if err != nil {
		return thread.Undef, err
	}

	// we use symmetric key because it is also has the size of 32 bytes
	return threadIDFromBytes(thread.Raw, threadDerivedIndexToSmartblockType[threadDerivedIndexProfilePage], node.SymmetricKey())
}

func ProfileThreadKeysFromAccountPublicKey(pubk crypto.PubKey) (service *symmetric.Key, read *symmetric.Key, err error) {
	masterKey, err2 := pubk.Raw()
	if err2 != nil {
		err = err2
		return
	}

	return threadDeriveKeys(threadDerivedIndexProfilePage, masterKey)
}

func ProfileThreadIDFromAccountAddress(address string) (thread.ID, error) {
	pubk, err := wallet.NewPubKeyFromAddress(wallet.KeypairTypeAccount, address)
	if err != nil {
		return thread.Undef, err
	}
	return ProfileThreadIDFromAccountPublicKey(pubk)
}

// threadDeriveKeys derive service and read encryption keys derived from key
func threadDeriveKeys(index threadDerivedIndex, masterKey []byte) (service *symmetric.Key, read *symmetric.Key, err error) {
	if len(masterKey) != 32 {
		err = fmt.Errorf("masterKey length should be 32 bytes, got %d instead", len(masterKey))
		return
	}

	nodeKey, err2 := slip21.DeriveForPath(fmt.Sprintf(anytypeThreadPathFormat, index, anytypeThreadServiceKeySuffix), masterKey)
	if err2 != nil {
		err = err2
		return
	}

	service, err = symmetric.FromBytes(append(nodeKey.SymmetricKey()))
	if err != nil {
		return
	}

	nodeKey, err = slip21.DeriveForPath(fmt.Sprintf(anytypeThreadPathFormat, index, anytypeThreadReadKeySuffix), masterKey)
	if err != nil {
		return
	}

	read, err = symmetric.FromBytes(nodeKey.SymmetricKey())
	if err != nil {
		return
	}

	return
}

func threadDeriveId(index threadDerivedIndex, accountKey []byte) (thread.ID, error) {
	node, err := slip21.DeriveForPath(fmt.Sprintf(anytypeThreadPathFormat, index, anytypeThreadIdKeySuffix), accountKey)
	if err != nil {
		return thread.Undef, err
	}

	// we use symmetric key because it is also has the size of 32 bytes
	return threadIDFromBytes(thread.Raw, threadDerivedIndexToSmartblockType[index], node.SymmetricKey())
}

func (a *Anytype) threadDeriveKeys(index threadDerivedIndex) (service *symmetric.Key, read *symmetric.Key, err error) {
	if index == threadDerivedIndexProfilePage {
		// anyone should be able to read profile
		// so lets derive its encryption keys from the account public key instead
		masterKey, err2 := a.account.GetPublic().Raw()
		if err2 != nil {
			err = err2
			return
		}
		return threadDeriveKeys(index, masterKey)
	}

	var masterKey = make([]byte, 32)
	pkey, err2 := a.account.Raw()
	if err2 != nil {
		err = err2
		return
	}
	copy(masterKey, pkey[:32])

	return threadDeriveKeys(index, masterKey)
}

func (a *Anytype) threadDeriveID(index threadDerivedIndex) (thread.ID, error) {
	if index == threadDerivedIndexProfilePage {
		accountKey, err := a.account.GetPublic().Bytes()
		if err != nil {
			return thread.Undef, err
		}

		return threadDeriveId(index, accountKey)
	}

	var masterKey = make([]byte, 32)
	pkey, err := a.account.Raw()
	if err != nil {
		return thread.Undef, err
	}
	copy(masterKey, pkey[:32])

	return threadDeriveId(index, masterKey)
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
	id, err := a.threadDeriveID(index)
	if err != nil {
		return thread.Info{}, err
	}

	return a.t.GetThread(context.TODO(), id)
}

func (a *Anytype) predefinedThreadAdd(index threadDerivedIndex, mustSyncSnapshotIfNotExist bool) (thread.Info, error) {
	id, err := a.threadDeriveID(index)
	if err != nil {
		return thread.Info{}, err
	}

	thrd, err := a.t.GetThread(context.TODO(), id)
	if err == nil && thrd.Key.Service() != nil {
		return thrd, nil
	}

	serviceKey, readKey, err := a.threadDeriveKeys(index)
	if err != nil {
		return thread.Info{}, err
	}

	thrd, err = a.t.CreateThread(context.TODO(),
		id,
		corenet.WithThreadKey(thread.NewKey(serviceKey, readKey)),
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

func threadIDFromBytes(variant thread.Variant, blockType SmartBlockType, b []byte) (thread.ID, error) {
	blen := len(b)
	// two 8 bytes (max) numbers plus num
	buf := make([]byte, 2*binary.MaxVarintLen64+blen)
	n := binary.PutUvarint(buf, thread.V1)
	n += binary.PutUvarint(buf[n:], uint64(variant))
	n += binary.PutUvarint(buf[n:], uint64(blockType))

	cn := copy(buf[n:], b)
	if cn != blen {
		return thread.Undef, fmt.Errorf("copy length is inconsistent")
	}

	return thread.Cast(buf[:n+blen])
}

func threadCreateID(variant thread.Variant, blockType SmartBlockType) (thread.ID, error) {
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
