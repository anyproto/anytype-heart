package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/wallet"
	"github.com/gogo/protobuf/proto"
	cbornode "github.com/ipfs/go-ipld-cbor"
)

// increasing this version allows old clients that trying to read the newer version of payload to force user to upgrade
const payloadVersion = 0

type SignedPbPayload struct {
	DeviceSig []byte // deprecated
	AccSig    []byte
	AccAddr   string
	Data      []byte
	Ver       uint16
}

// deprecated, to be removed in the next version
//
// SignedPbPayloadWithoutVersion used in this transition for the new records in order to not break the prev versions without the Ver field
type SignedPbPayloadWithoutVersion struct {
	DeviceSig []byte // deprecated
	AccSig    []byte
	AccAddr   string
	Data      []byte
}

func init() {
	cbornode.RegisterCborType(SignedPbPayload{})
	cbornode.RegisterCborType(SignedPbPayloadWithoutVersion{})
}

func newSignedPayload(payload []byte, accountKey wallet.Keypair) (*SignedPbPayloadWithoutVersion, error) {
	accSig, err := accountKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	return &SignedPbPayloadWithoutVersion{AccAddr: accountKey.Address(), AccSig: accSig, Data: payload}, nil
}

func (p *SignedPbPayload) Unmarshal(out proto.Message) error {
	return proto.Unmarshal(p.Data, out)
}

func (p *SignedPbPayload) Verify() error {
	account, err := wallet.NewPubKeyFromAddress(wallet.KeypairTypeAccount, p.AccAddr)
	if err != nil {
		return fmt.Errorf("incorrect account addr: %w", err)
	}

	ok, err := account.Verify(append(p.Data), p.AccSig)
	if !ok || err != nil {
		return fmt.Errorf("bad account signature: %w", err)
	}
	return nil
}
