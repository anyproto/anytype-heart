package core

import (
	"errors"
	"fmt"

	"github.com/anytypeio/go-anytype-library/schema"
	logging "github.com/ipfs/go-log"
	"github.com/mr-tron/base58"
	tcore "github.com/textileio/go-textile/core"
	"github.com/textileio/go-textile/crypto"
	"github.com/textileio/go-textile/keypair"
	tpb "github.com/textileio/go-textile/pb"
	"github.com/textileio/go-textile/wallet"
)

var log = logging.Logger("tex-core")

type derivedThreadIndex uint32

const (
	threadDerivedIndexHomeDashboard    derivedThreadIndex = 0
	threadDerivedIndexArchiveDashboard derivedThreadIndex = 1
)

var threadDerivedIndexToThreadName = map[derivedThreadIndex]string{
	threadDerivedIndexHomeDashboard:    "home",
	threadDerivedIndexArchiveDashboard: "archive",
}

var threadDerivedIndexToSchema = map[derivedThreadIndex]string{
	threadDerivedIndexHomeDashboard:    schema.Dashboard,
	threadDerivedIndexArchiveDashboard: schema.Dashboard,
}

func (a *Anytype) createPredefinedThreads() error {
	for index, _ := range threadDerivedIndexToThreadName {
		err := a.predefinedThreadAdd(index)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *Anytype) deriveKey(index derivedThreadIndex) (*keypair.Full, error) {
	key, err := wallet.NewMasterKey([]byte(a.Textile.Node().Account().Seed()))
	if err != nil {
		return nil, err
	}

	key, err = key.Derive(uint32(index) + wallet.FirstHardenedIndex)
	if err != nil {
		return nil, err
	}

	return keypair.FromRawSeed(key.RawSeed())
}

func (a *Anytype) predefinedThreadByName(name string) (*tcore.Thread, error) {
	for index, tname := range threadDerivedIndexToThreadName {
		if name == tname {
			return a.predefinedThread(index)
		}
	}

	return nil, fmt.Errorf("thread not found")
}

func (a *Anytype) predefinedThread(index derivedThreadIndex) (*tcore.Thread, error) {
	key, err := a.deriveKey(index)
	if err != nil {
		return nil, err
	}

	if t := a.Textile.Node().ThreadByKey(key.Address()); t != nil {
		return t, nil
	}
	return nil, fmt.Errorf("thread not found")
}

func (a *Anytype) predefinedThreadAdd(index derivedThreadIndex) error {
	key, err := a.deriveKey(index)
	if err != nil {
		return err
	}

	if a.Textile.Node().ThreadByKey(key.Address()) != nil {
		return nil
	}

	sf, err := a.Textile.Node().AddSchema(threadDerivedIndexToSchema[index], "dashboard")
	if err != nil {
		return err
	}

	config := tpb.AddThreadConfig{
		Key:  key.Address(),
		Name: threadDerivedIndexToThreadName[index],
		Schema: &tpb.AddThreadConfig_Schema{
			Id: sf.Hash,
		},
		Type:    tpb.Thread_PRIVATE,
		Sharing: tpb.Thread_NOT_SHARED,
	}

	sk, err := key.LibP2PPrivKey()
	if err != nil {
		return err
	}

	thread, err := a.Textile.Node().AddThread(config, sk, a.Textile.Address(), true, false)
	if err != nil {
		return err
	}

	// add existing contacts
	for _, p := range a.Textile.Node().Datastore().Peers().List(fmt.Sprintf("address!='%s'", a.Textile.Address())) {
		_, err = thread.Annouce(&tpb.ThreadAnnounce{Peer: p})
		if err != nil {
			return err
		}
	}

	return nil
}

func readFile(t *tcore.Textile, file *tpb.FileIndex) ([]byte, error) {
	if file == nil {
		return nil, errors.New("fileIndex is nil")
	}

	data, err := t.DataAtPath(file.Hash)
	if err != nil {
		return nil, fmt.Errorf("DataAtPath error: %s", err.Error())
	}

	if file.Key == "" {
		return data, nil
	}

	keyb, err := base58.Decode(file.Key)
	if err != nil {
		return nil, fmt.Errorf("key decode error: %s", err.Error())
	}

	plain, err := crypto.DecryptAES(data, keyb)
	if err != nil {
		return nil, fmt.Errorf("decryption error: %s", err.Error())
	}

	return plain, nil
}
