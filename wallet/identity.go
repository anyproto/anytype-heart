package wallet

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/textileio/go-textile/strkey"
)

const (
	accountAddressVersionByte strkey.VersionByte = 0x5b // Base58-encodes to 'A...'
	accountSeedVersionByte strkey.VersionByte = 0xfb // Base58-encodes to 'S...'
)

type AccountKeypair struct {
	crypto.PrivKey
}

type DeviceKeypair struct {
	crypto.PrivKey
}

func AccountKeypairFromPrivKey(privKey crypto.PrivKey) (*AccountKeypair, error){
	identity := AccountKeypair{PrivKey: privKey}
	_, err := identity.address()
	if err != nil {
		return nil, err
	}

	return &identity, nil
}

func PrivateKeyFromRandom() (crypto.PrivKey, error){
	privk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	return privk, nil
}

func PrivateKeyFromFile(filepath string) (crypto.PrivKey, error){
	_, err := os.Stat(filepath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("key file not exists")
	} else if err != nil {
		return nil, err
	}

	b, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	return crypto.UnmarshalPrivateKey(b)
}

func DeviceKeypairFromPrivKey(privKey crypto.PrivKey) (*DeviceKeypair, error) {
	identity := DeviceKeypair{PrivKey: privKey}
	_, err := identity.address()
	if err != nil {
		return nil, err
	}

	return &identity, nil
}

func (kp DeviceKeypair) Address() string {
	address, err := kp.address()
	if err != nil {
		// shouldn't be a case because we check it on init
		log.Error(err)
	}

	return address
}

func (kp AccountKeypair) Address() string {
	address, err := kp.address()
	if err != nil {
		// shouldn't be a case because we check it on init
		log.Error(err)
	}

	return address
}

func (kp AccountKeypair) Seed() []byte {
	b, err := kp.Raw()
	if err != nil {
		// shouldn't be a case because we check it on init
		log.Error(err)
	}

	return b
}

func (kp AccountKeypair) MarshalBinary() ([]byte, error) {
	return crypto.MarshalPrivateKey(kp.PrivKey)
}

func (kp AccountKeypair) address() (string, error) {
	b, err := kp.GetPublic().Raw()
	if err != nil {
		return "", err
	}

	return strkey.Encode(accountAddressVersionByte, b)
}

func (kp AccountKeypair) seed() (string, error) {
	b, err := kp.GetPublic().Raw()
	if err != nil {
		return "", err
	}

	return strkey.Encode(accountSeedVersionByte, b)
}

func (kp DeviceKeypair) address() (string, error) {
	id, err := peer.IDFromPublicKey(kp.PrivKey.GetPublic())
	if err != nil {
		return "", err
	}

	return id.String(), nil
}

func (kp DeviceKeypair) MarshalBinary() ([]byte, error) {
	return crypto.MarshalPrivateKey(kp.PrivKey)
}
