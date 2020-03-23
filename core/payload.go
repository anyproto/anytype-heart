package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/wallet"
	"github.com/gogo/protobuf/proto"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/crypto"
)

type signedPbPayload struct {
	DeviceSig []byte
	AccSig    []byte
	Data      []byte
}

func init() {
	cbornode.RegisterCborType(signedPbPayload{})
}

func newSignedPayload(payload []byte, deviceKey wallet.Keypair, accountKey wallet.Keypair) (*signedPbPayload, error) {
	accSig, err := accountKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	deviceSig, err := deviceKey.Sign(append(payload, accSig...))
	if err != nil {
		return nil, err
	}

	return &signedPbPayload{DeviceSig: deviceSig, AccSig: accSig, Data: payload}, nil
}

func (p *signedPbPayload) Unmarshal(out proto.Message) error {
	return proto.Unmarshal(p.Data, out)
}

func (p *signedPbPayload) Verify(device crypto.PubKey, account crypto.PubKey) error {
	ok, err := device.Verify(append(p.Data, p.AccSig...), p.DeviceSig)
	if !ok || err != nil {
		return fmt.Errorf("bad device signature")
	}

	ok, err = account.Verify(append(p.Data), p.AccSig)
	if !ok || err != nil {
		return fmt.Errorf("bad account signature")
	}
	return nil
}
