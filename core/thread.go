package core

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-library/wallet"
	"github.com/anytypeio/go-slip21"
	"github.com/libp2p/go-libp2p-core/crypto"
	db2 "github.com/textileio/go-threads/core/db"
	corenet "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/crypto/symmetric"
	"github.com/textileio/go-threads/db"
	"github.com/textileio/go-threads/util"
)

type threadDerivedIndex uint32

type threadInfo struct {
	ID    db2.InstanceID `json:"_id"`
	Key   string
	Addrs []string
}

var (
	threadInfoCollectionName = "threads"

	threadInfoCollection = db.CollectionConfig{
		Name:   threadInfoCollectionName,
		Schema: util.SchemaFromInstance(threadInfo{}, false),
	}
)

const (
	waitInitialSyncMaxAttempts = 3
)

const (
	// profile page is publicly accessible as service/read keys derived from account public key
	threadDerivedIndexProfilePage threadDerivedIndex = 0
	threadDerivedIndexHome        threadDerivedIndex = 1
	threadDerivedIndexArchive     threadDerivedIndex = 2
	threadDerivedIndexAccount     threadDerivedIndex = 3

	threadDerivedIndexSetPages threadDerivedIndex = 20

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
var threadDerivedIndexToSmartblockType = map[threadDerivedIndex]smartblock.SmartBlockType{
	threadDerivedIndexProfilePage: smartblock.SmartBlockTypeProfilePage,
	threadDerivedIndexHome:        smartblock.SmartBlockTypeHome,
	threadDerivedIndexArchive:     smartblock.SmartBlockTypeArchive,
	threadDerivedIndexSetPages:    smartblock.SmartBlockTypeSet,
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
	masterKey, err := pubk.Raw()
	if err != nil {
		return
	}

	return threadDeriveKeys(threadDerivedIndexProfilePage, masterKey)
}

func ProfileThreadKeysFromAccountAddress(address string) (service *symmetric.Key, read *symmetric.Key, err error) {
	pubk, err := wallet.NewPubKeyFromAddress(wallet.KeypairTypeAccount, address)
	if err != nil {
		return
	}

	return ProfileThreadKeysFromAccountPublicKey(pubk)
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
		masterKey, err2 := a.opts.Account.GetPublic().Raw()
		if err2 != nil {
			err = err2
			return
		}
		return threadDeriveKeys(index, masterKey)
	}

	var masterKey = make([]byte, 32)
	pkey, err2 := a.opts.Account.Raw()
	if err2 != nil {
		err = err2
		return
	}
	copy(masterKey, pkey[:32])

	return threadDeriveKeys(index, masterKey)
}

func (a *Anytype) threadDeriveID(index threadDerivedIndex) (thread.ID, error) {
	if a.opts.Account == nil {
		return thread.Undef, fmt.Errorf("account key not set")
	}

	if index == threadDerivedIndexProfilePage {
		accountKey, err := a.opts.Account.GetPublic().Raw()
		if err != nil {
			return thread.Undef, err
		}

		return threadDeriveId(index, accountKey)
	}

	var masterKey = make([]byte, 32)
	pkey, err := a.opts.Account.Raw()
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

func (a *Anytype) syncThread(thrd thread.Info, mustConnectToCafe bool, pullAfterConnect bool, waitForPull bool) error {
	var err error
	if a.opts.CafeP2PAddr == nil {
		return nil
	}

	select {
	case <-a.onlineCh:
		break
	case <-a.shutdownStartsCh:
		return fmt.Errorf("node was stopped")
	}

	ctx, cancel := context.WithCancel(context.Background())
	var syncDone = make(chan struct{})
	go func() {
		select {
		case <-a.shutdownStartsCh:
			cancel()
			return
		case <-syncDone:
			cancel()
			return
		case <-ctx.Done():
			return
		}
	}()

	replicatorAddFinished := make(chan struct{})
	go func() {
		attempts := 0
		defer close(replicatorAddFinished)
		for _, addr := range thrd.Addrs {
			if addr.Equal(a.opts.CafeP2PAddr) {
				log.Warnf("syncThread %s already has replicator")
				return
			}
		}

		for {
			start := time.Now()

			select {
			case <-a.shutdownStartsCh:
				err = fmt.Errorf("failed to add replicator to %s: node was stopped", thrd.ID.String())
				return
			default:
			}

			_, err = a.t.AddReplicator(ctx, thrd.ID, a.opts.CafeP2PAddr)
			if err != nil {
				attempts++
				if mustConnectToCafe {
					log.Errorf("syncThread failed to add replicator for %s after %.2fs %d/%d attempt: %s", thrd.ID.String(), time.Since(start).Seconds(), attempts, waitInitialSyncMaxAttempts, err.Error())

					if attempts >= waitInitialSyncMaxAttempts {
						break
					}
					// we do not need sleep here because we
					continue
				}

				log.Errorf("syncThread failed to add replicator for %s after %.2fs (%d attempt): %s", thrd.ID.String(), time.Since(start).Seconds(), attempts, err.Error())

				select {
				case <-time.After(time.Second * time.Duration(10*attempts)):
					continue
				case <-a.shutdownStartsCh:
					err = fmt.Errorf("failed to add replicator to %s: node was stopped", thrd.ID.String())
					return
				}
			}

			break
		}
	}()

	pullDone := make(chan struct{})
	if mustConnectToCafe {
		select {
		case <-replicatorAddFinished:
		case <-a.shutdownStartsCh:
			return fmt.Errorf("node stopped")
		}
		if err != nil {
			close(syncDone)
			return err
		}

		if !pullAfterConnect {
			close(syncDone)
			return nil
		}

		go func() {
			defer close(pullDone)
			defer close(syncDone)
			err = a.pullThread(ctx, thrd.ID)
			if err != nil {
				log.Errorf("syncThread failed to pullThread: %s", err.Error())
				return
			}
		}()
	} else {
		go func() {
			defer close(pullDone)
			defer close(syncDone)
			select {
			case <-replicatorAddFinished:
			case <-a.shutdownStartsCh:
				log.Errorf("syncThread failed to pullThread: node stopped")
				return
			}

			if err != nil {
				log.Errorf(err.Error())
				return
			}

			if !pullAfterConnect {
				return
			}

			err = a.pullThread(ctx, thrd.ID)
			if err != nil {
				log.Errorf("syncThread failed to pullThread: %s", err.Error())
				return
			}
		}()
	}

	if mustConnectToCafe && pullAfterConnect && waitForPull {
		log.Debugf("syncThread wait for pull")
		<-pullDone
		log.Debugf("syncThread pull done")
	}

	return nil
}

func (a *Anytype) predefinedThreadAdd(index threadDerivedIndex, mustSyncSnapshotIfNotExist bool, pullAfterConnect bool, waitForPull bool) (info thread.Info, justCreated bool, err error) {
	id, err := a.threadDeriveID(index)
	if err != nil {
		return thread.Info{}, false, err
	}

	thrd, err := a.t.GetThread(context.TODO(), id)
	if err == nil && thrd.Key.Service() != nil {
		err = a.syncThread(thrd, false, pullAfterConnect, waitForPull)
		if err != nil {
			return thread.Info{}, false, err
		}
		return thrd, false, nil
	}

	serviceKey, readKey, err := a.threadDeriveKeys(index)
	if err != nil {
		return thread.Info{}, false, err
	}

	thrd, err = a.t.CreateThread(context.TODO(),
		id,
		corenet.WithThreadKey(thread.NewKey(serviceKey, readKey)),
		corenet.WithLogKey(a.opts.Device))
	if err != nil {
		return thread.Info{}, false, err
	}

	err = a.syncThread(thrd, mustSyncSnapshotIfNotExist, pullAfterConnect, waitForPull)
	if err != nil {
		return thread.Info{}, true, err
	}

	return thrd, true, nil
}

func threadIDFromBytes(variant thread.Variant, blockType smartblock.SmartBlockType, b []byte) (thread.ID, error) {
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

func threadCreateID(variant thread.Variant, blockType smartblock.SmartBlockType) (thread.ID, error) {
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
