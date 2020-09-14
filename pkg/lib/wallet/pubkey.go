package wallet

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/strkey"
	"github.com/libp2p/go-libp2p-core/crypto"
	crypto_pb "github.com/libp2p/go-libp2p-core/crypto/pb"
	"github.com/libp2p/go-libp2p-core/peer"
)

type PubKey interface {
	Address() string
	KeypairType() KeypairType

	crypto.PubKey
}

type pubKey struct {
	keyType KeypairType
	crypto.PubKey
}

func NewPubKey(t KeypairType, pk crypto.PubKey) (PubKey, error) {
	if t != KeypairTypeAccount && t != KeypairTypeDevice {
		return nil, fmt.Errorf("incorrect KeypairType")
	}

	pubk := pubKey{
		keyType: t,
		PubKey:  pk,
	}

	_, err := getAddress(t, pk)
	if err != nil {
		return nil, err
	}

	return pubk, nil
}

func NewPubKeyFromAddress(t KeypairType, address string) (PubKey, error) {
	if t != KeypairTypeAccount && t != KeypairTypeDevice {
		return nil, fmt.Errorf("incorrect KeypairType")
	}

	if t == KeypairTypeAccount {
		pubKeyRaw, err := strkey.Decode(accountAddressVersionByte, address)
		if err != nil {
			return nil, err
		}

		unmarshal := crypto.PubKeyUnmarshallers[crypto_pb.KeyType_Ed25519]
		pk, err := unmarshal(pubKeyRaw)
		if err != nil {
			return nil, err
		}

		return &pubKey{
			keyType: t,
			PubKey:  pk,
		}, nil
	} else {
		peerID, err := peer.Decode(address)
		if err != nil {
			return nil, err
		}

		pk, err := peerID.ExtractPublicKey()
		if err != nil {
			return nil, err
		}

		return &pubKey{
			keyType: t,
			PubKey:  pk,
		}, nil
	}
}

func (pk pubKey) Address() string {
	address, err := getAddress(pk.keyType, pk.PubKey)
	if err != nil {
		// shouldn't be a case because we check it on init
		log.Error(err)
	}

	return address
}

func (pk pubKey) KeypairType() KeypairType {
	return pk.keyType
}

func getAddress(keyType KeypairType, key crypto.PubKey) (string, error) {
	b, err := key.Raw()
	if err != nil {
		return "", err
	}

	if keyType == KeypairTypeAccount {
		return strkey.Encode(accountAddressVersionByte, b)
	} else {
		peerId, err := peer.IDFromPublicKey(key)
		if err != nil {
			return "", err
		}

		return peerId.String(), nil
	}
}
