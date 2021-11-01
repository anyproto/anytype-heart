package threads

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	threadsDb "github.com/textileio/go-threads/db"
	threadsUtil "github.com/textileio/go-threads/util"

	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"github.com/anytypeio/go-slip21"
	"github.com/libp2p/go-libp2p-core/crypto"

	corenet "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/crypto/symmetric"
)

type threadDerivedIndex uint32

const (
	// profile page is publicly accessible as service/read keys derived from account public key
	threadDerivedIndexProfilePage threadDerivedIndex = 0
	threadDerivedIndexHome        threadDerivedIndex = 1
	threadDerivedIndexArchive     threadDerivedIndex = 2
	threadDerivedIndexAccountOld  threadDerivedIndex = 3
	threadDerivedIndexAccount     threadDerivedIndex = 4

	threadDerivedIndexSetPages threadDerivedIndex = 20

	threadDerivedIndexMarketplaceType     threadDerivedIndex = 30
	threadDerivedIndexMarketplaceRelation threadDerivedIndex = 31
	threadDerivedIndexMarketplaceTemplate threadDerivedIndex = 32

	anytypeThreadSymmetricKeyPathPrefix = "m/SLIP-0021/anytype"
	// TextileAccountPathFormat is a path format used for Anytype keypair
	// derivation as described in SEP-00XX. Use with `fmt.Sprintf` and `DeriveForPath`.
	// m/SLIP-0021/anytype/<predefined_thread_index>/%d/<label>
	anytypeThreadPathFormat = anytypeThreadSymmetricKeyPathPrefix + `/%d/%s`

	anytypeThreadServiceKeySuffix = `service`
	anytypeThreadReadKeySuffix    = `read`
	anytypeThreadIdKeySuffix      = `id`
)

type DerivedSmartblockIds struct {
	AccountOld          string
	Account             string
	Profile             string
	Home                string
	Archive             string
	SetPages            string
	MarketplaceType     string
	MarketplaceRelation string
	MarketplaceTemplate string
}

func (d DerivedSmartblockIds) IsAccount(id string) bool {
	return id == d.Account || id == d.AccountOld
}

var threadDerivedIndexToSmartblockType = map[threadDerivedIndex]smartblock.SmartBlockType{
	threadDerivedIndexAccount:             smartblock.SmartBlockTypeWorkspace,
	threadDerivedIndexAccountOld:          smartblock.SmartBlockTypeAccountOld,
	threadDerivedIndexProfilePage:         smartblock.SmartBlockTypeProfilePage,
	threadDerivedIndexHome:                smartblock.SmartBlockTypeHome,
	threadDerivedIndexArchive:             smartblock.SmartBlockTypeArchive,
	threadDerivedIndexSetPages:            smartblock.SmartBlockTypeSet,
	threadDerivedIndexMarketplaceType:     smartblock.SmartblockTypeMarketplaceType,
	threadDerivedIndexMarketplaceRelation: smartblock.SmartblockTypeMarketplaceRelation,
	threadDerivedIndexMarketplaceTemplate: smartblock.SmartblockTypeMarketplaceTemplate,
}
var ErrAddReplicatorsAttemptsExceeded = fmt.Errorf("add replicatorAddr attempts exceeded")

func (s *service) EnsurePredefinedThreads(ctx context.Context, newAccount bool) (DerivedSmartblockIds, error) {
	s.Lock()
	defer s.Unlock()

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-s.ctx.Done():
			cancel()
		case <-cctx.Done():
			return
		}
	}()

	// we actually need to set up new one and old one and check if we need to start old one at all
	accountIds := DerivedSmartblockIds{}
	// account old
	accountOld, justCreated, err := s.derivedThreadEnsure(cctx, threadDerivedIndexAccountOld, newAccount, false)
	if err != nil {
		return accountIds, err
	}
	accountIds.AccountOld = accountOld.ID.String()

	// we still need to do this at threads level for old account
	err = s.SetupThreadsDB(accountOld.ID)
	if err != nil {
		return accountIds, err
	}

	if !newAccount {
		if justCreated {
			// old account thread: sync pull case
			err = s.t.PullThread(cctx, accountOld.ID)
			if ctx.Err() != context.Canceled && err != nil {
				return accountIds, err
			}
		} else {
			s.handleMissingReplicators()
			// old account thread: async pull case
			s.handleMissingOldDbRecords(accountOld.ID.String())
		}
	}

	// account new
	account, _, err := s.derivedThreadEnsure(cctx, threadDerivedIndexAccount, newAccount, true)
	if err != nil {
		return accountIds, err
	}
	accountIds.Account = account.ID.String()

	// profile
	profile, _, err := s.derivedThreadEnsure(cctx, threadDerivedIndexProfilePage, newAccount, true)
	if err != nil {
		return accountIds, err
	}
	accountIds.Profile = profile.ID.String()

	// home
	home, _, err := s.derivedThreadEnsure(cctx, threadDerivedIndexHome, newAccount, true)
	if err != nil {
		return accountIds, err
	}
	accountIds.Home = home.ID.String()

	// archive
	archive, _, err := s.derivedThreadEnsure(cctx, threadDerivedIndexArchive, newAccount, true)
	if err != nil {
		return accountIds, err
	}
	accountIds.Archive = archive.ID.String()

	// set pages
	setPages, _, err := s.derivedThreadEnsure(cctx, threadDerivedIndexSetPages, newAccount, true)
	if err != nil {
		return accountIds, err
	}
	accountIds.SetPages = setPages.ID.String()

	// marketplace
	marketplace, _, err := s.derivedThreadEnsure(cctx, threadDerivedIndexMarketplaceType, newAccount, true)
	if err != nil {
		return accountIds, err
	}
	accountIds.MarketplaceType = marketplace.ID.String()

	// marketplace library
	marketplaceLib, _, err := s.derivedThreadEnsure(cctx, threadDerivedIndexMarketplaceRelation, newAccount, true)
	if err != nil {
		return accountIds, err
	}
	accountIds.MarketplaceRelation = marketplaceLib.ID.String()

	// marketplace template
	marketplaceTemplate, _, err := s.derivedThreadEnsure(cctx, threadDerivedIndexMarketplaceTemplate, newAccount, true)
	if err != nil {
		return accountIds, err
	}
	accountIds.MarketplaceTemplate = marketplaceTemplate.ID.String()

	return accountIds, nil
}

func (s *service) SetupThreadsDB(id thread.ID) error {
	if s.db != nil {
		return nil
	}
	if id == thread.Undef {
		return fmt.Errorf("cannot start setup threads db with undefined thread")
	}
	tInfo, err := s.t.GetThread(context.Background(), id)
	if err != nil {
		return fmt.Errorf("cannot start threads db, because thread is not downloaded: %w", err)
	}

	s.db, err = threadsDb.NewDB(
		context.Background(),
		s.threadsDbDS,
		s.t,
		id,
		// We need to provide the key beforehand
		// otherwise there can be problems if the log is not created (and therefore the keys are not matched)
		// this happens with workspaces, because we are adding threads but not creating them
		threadsDb.WithNewKey(tInfo.Key),
		threadsDb.WithNewCollections())
	if err != nil {
		return err
	}

	threadsCollectionName := ThreadInfoCollectionName
	s.threadsCollection = s.db.GetCollection(threadsCollectionName)

	if s.threadsCollection != nil {
		return nil
	}

	collectionConfig := threadsDb.CollectionConfig{
		Name:   threadsCollectionName,
		Schema: threadsUtil.SchemaFromInstance(ThreadDBInfo{}, false),
	}
	s.threadsCollection, err = s.db.NewCollection(collectionConfig)
	return err
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

func (s *service) derivedThreadKeyByIndex(index threadDerivedIndex) (service *symmetric.Key, read *symmetric.Key, err error) {
	if index == threadDerivedIndexProfilePage {
		// anyone should be able to read profile
		// so lets derive its encryption keys from the account public key instead
		masterKey, err2 := s.account.GetPublic().Raw()
		if err2 != nil {
			err = err2
			return
		}
		return threadDeriveKeys(index, masterKey)
	}

	var masterKey = make([]byte, 32)
	pkey, err2 := s.account.Raw()
	if err2 != nil {
		err = err2
		return
	}
	copy(masterKey, pkey[:32])

	return threadDeriveKeys(index, masterKey)
}

func (s *service) derivedThreadIdByIndex(index threadDerivedIndex) (thread.ID, error) {
	if s.account == nil {
		return thread.Undef, fmt.Errorf("account key not set")
	}

	if index == threadDerivedIndexProfilePage {
		accountKey, err := s.account.GetPublic().Raw()
		if err != nil {
			return thread.Undef, err
		}

		return threadDeriveId(index, accountKey)
	}

	var masterKey = make([]byte, 32)
	pkey, err := s.account.Raw()
	if err != nil {
		return thread.Undef, err
	}
	copy(masterKey, pkey[:32])

	return threadDeriveId(index, masterKey)
}

func (s *service) derivedThreadEnsure(ctx context.Context, index threadDerivedIndex, newAccount bool, pull bool) (thrd thread.Info, justCreated bool, err error) {
	if newAccount {
		thrd, err := s.derivedThreadCreate(index)
		return thrd, true, err
	}

	return s.derivedThreadAddExistingFromLocalOrRemote(ctx, index, pull)
}

func (s *service) derivedThreadCreate(index threadDerivedIndex) (thread.Info, error) {
	id, err := s.derivedThreadIdByIndex(index)
	if err != nil {
		return thread.Info{}, err
	}
	serviceKey, readKey, err := s.derivedThreadKeyByIndex(index)
	if err != nil {
		return thread.Info{}, err
	}

	return s.threadCreate(id, thread.NewKey(serviceKey, readKey))
}

func (s *service) threadCreate(threadId thread.ID, key thread.Key) (thread.Info, error) {
	thrd, err := s.t.GetThread(context.Background(), threadId)
	if err == nil && thrd.Key.Service() != nil {
		return thrd, nil
	}

	thrd, err = s.t.CreateThread(context.Background(),
		threadId,
		corenet.WithThreadKey(key),
		corenet.WithLogKey(s.device))
	if err != nil {
		return thread.Info{}, err
	}

	metrics.ServedThreads.Inc()
	metrics.ThreadAdded.Inc()
	// because this thread just have been created locally we can safely put all networking in the background
	go func() {
		if s.replicatorAddr == nil {
			return
		}

		err = s.addReplicatorWithAttempts(s.ctx, thrd, s.replicatorAddr, 0)
		if err != nil {
			log.Warnf("derivedThreadCreate failed to add replicatorAddr: %s", err.Error())
		}
	}()

	return thrd, nil
}

func (s *service) derivedThreadAddExistingFromLocalOrRemote(ctx context.Context, index threadDerivedIndex, pull bool) (thrd thread.Info, justCreated bool, err error) {
	id, err := s.derivedThreadIdByIndex(index)
	if err != nil {
		return thread.Info{}, false, err
	}

	addReplicatorAnPullAfter := func(thrd thread.Info) {
		var err error
		if s.replicatorAddr != nil {
			// if thread doesn't yet have s replicatorAddr this function will continuously try to add it in the background
			err := s.addReplicatorWithAttempts(s.ctx, thrd, s.replicatorAddr, 0)
			if err != nil {
				log.Errorf("existing thread failed to add replicatorAddr: %v", err)
				return
			}
		}

		if !pull {
			return
		}

		// lets try to pull it once the replicatorAddr have been added
		// in case it fails this thread will be still pulled every PullInterval
		err = s.t.PullThread(ctx, thrd.ID)
		if err != nil {
			log.Errorf("failed to pull existing thread: %s", err.Error())
			return
		}
	}

	thrd, err = s.t.GetThread(ctx, id)
	if err == nil && thrd.Key.Service() != nil {
		// we already have the thread locally, we can safely pull it in background
		go addReplicatorAnPullAfter(thrd)
		return thrd, false, nil
	}

	serviceKey, readKey, err := s.derivedThreadKeyByIndex(index)
	if err != nil {
		return thrd, false, err
	}

	// we must recover it from
	// intentionally do not pass the original ctx, because we don't want to stuck in the middle of thread creation
	thrd, err = s.t.CreateThread(context.Background(),
		id,
		corenet.WithThreadKey(thread.NewKey(serviceKey, readKey)),
		corenet.WithLogKey(s.device))
	if err != nil {
		return
	}

	metrics.ServedThreads.Inc()
	metrics.ThreadAdded.Inc()

	justCreated = true

	if s.replicatorAddr != nil {
		err = s.addReplicatorWithAttempts(ctx, thrd, s.replicatorAddr, 3)
		if err != nil {
			// remove the thread we have just created because we've supposed to successfully pull it from the replicatorAddr
			err2 := s.t.DeleteThread(context.Background(), id)
			if err2 != nil {
				log.Errorf("failed to delete thread: %s", err2.Error())
			}
			return
		}
	}

	if !pull {
		return
	}

	err = s.t.PullThread(ctx, thrd.ID)
	if err != nil {
		log.Errorf("failed to pull new thread: %s", err.Error())

		// remove the thread we have just created because we've supposed to successfully pull it from the replicatorAddr
		err2 := s.t.DeleteThread(context.Background(), id)
		if err2 != nil {
			log.Errorf("failed to delete thread: %s", err2.Error())
		}
		return
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

func ThreadCreateID(variant thread.Variant, blockType smartblock.SmartBlockType) (thread.ID, error) {
	rndlen := 32
	buf := make([]byte, 3*binary.MaxVarintLen64+rndlen)
	n := binary.PutUvarint(buf, thread.V1)
	n += binary.PutUvarint(buf[n:], uint64(variant))
	n += binary.PutUvarint(buf[n:], uint64(blockType))

	_, err := rand.Read(buf[n : n+rndlen])
	if err != nil {
		panic("random read failed")
	}
	return thread.Cast(buf[:n+rndlen])
}

func PatchSmartBlockType(id string, sbt smartblock.SmartBlockType) (string, error) {
	tid, err := thread.Decode(id)
	if err != nil {
		return id, err
	}
	rawid := []byte(tid.KeyString())
	ver, n := binary.Uvarint(rawid)
	variant, n2 := binary.Uvarint(rawid[n:])
	_, n3 := binary.Uvarint(rawid[n+n2:])
	finalN := n + n2 + n3
	buf := make([]byte, 3*binary.MaxVarintLen64+len(rawid)-finalN)
	n = binary.PutUvarint(buf, ver)
	n += binary.PutUvarint(buf[n:], variant)
	n += binary.PutUvarint(buf[n:], uint64(sbt))
	copy(buf[n:], rawid[finalN:])
	if tid, err = thread.Cast(buf[:n+len(rawid)-finalN]); err != nil {
		return id, err
	}
	return tid.String(), nil
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
