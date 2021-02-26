package wallet

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/strkey"
	"github.com/libp2p/go-libp2p-core/crypto"
)

const (
	accountAddressVersionByte strkey.VersionByte = 0x5b // Base58-encodes to 'A...'
	accountSeedVersionByte    strkey.VersionByte = 0xff // Base58-encodes to 'S...'
	deviceSeedVersionByte     strkey.VersionByte = 0x7d // Base58-encodes to 'D...'
)

type Keypair interface {
	Seed() string
	Address() string
	PeerId() (peer.ID, error)
	KeypairType() KeypairType
	MarshalBinary() ([]byte, error)

	crypto.PrivKey
}

type KeypairType uint64

const (
	KeypairTypeAccount KeypairType = iota
	KeypairTypeDevice
)

func (p KeypairType) String() string {
	switch p {
	case KeypairTypeAccount:
		return "Account"
	case KeypairTypeDevice:
		return "Device"
	}
	return fmt.Sprintf("KeypairType(%d)", p)
}

type keypair struct {
	keyType KeypairType
	crypto.PrivKey
}

func NewKeypairFromPrivKey(t KeypairType, privKey crypto.PrivKey) (Keypair, error) {
	if t != KeypairTypeAccount && t != KeypairTypeDevice {
		return nil, fmt.Errorf("incorrect KeypairType")
	}

	kp := keypair{
		keyType: t,
		PrivKey: privKey,
	}

	_, err := getAddress(t, privKey.GetPublic())
	if err != nil {
		return nil, err
	}

	return kp, nil
}

func NewRandomKeypair(t KeypairType) (Keypair, error) {
	privk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	return NewKeypairFromPrivKey(t, privk)
}

func (kp keypair) MarshalBinary() ([]byte, error) {
	var val = make([]byte, 1)
	binary.PutUvarint(val, uint64(kp.keyType))

	pk, err := crypto.MarshalPrivateKey(kp.PrivKey)
	if err != nil {
		return nil, err
	}

	return append(val, pk...), nil
}

func UnmarshalBinary(b []byte) (Keypair, error) {
	if len(b) < 2 {
		return nil, fmt.Errorf("bytes slice too small")
	}

	kt, n := binary.Uvarint(b[0:1])
	if n == 0 {
		return nil, fmt.Errorf("keypair type prefix not found")
	}

	switch KeypairType(kt) {
	case KeypairTypeAccount:
	case KeypairTypeDevice:
	default:
		return nil, fmt.Errorf("incorrect keypair type")
	}

	pk, err := crypto.UnmarshalPrivateKey(b[1:])
	if err != nil {
		return nil, err
	}

	return &keypair{
		keyType: KeypairType(kt),
		PrivKey: pk,
	}, nil
}

func (kp keypair) KeypairType() KeypairType {
	return kp.keyType
}

func (kp keypair) PeerId() (peer.ID, error) {
	return getPeer(kp.keyType, kp.GetPublic())
}

func (kp keypair) Address() string {
	address, err := getAddress(kp.keyType, kp.GetPublic())
	if err != nil {
		// shouldn't be a case because we check it on init
		log.Error(err)
	}

	return address
}

func (kp keypair) Seed() string {
	seed, err := kp.seed()
	if err != nil {
		// shouldn't be a case because we check it on init
		log.Error(err)
	}

	return seed
}

func (kp keypair) seed() (string, error) {
	var pkey = make([]byte, 32)
	b, err := kp.PrivKey.Raw()
	if err != nil {
		return "", err
	}
	copy(pkey, b[:32])

	if kp.keyType == KeypairTypeAccount {
		return strkey.Encode(accountSeedVersionByte, pkey)
	} else {
		return strkey.Encode(deviceSeedVersionByte, pkey)
	}
}
