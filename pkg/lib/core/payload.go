package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"github.com/gogo/protobuf/proto"
	cbornode "github.com/ipfs/go-ipld-cbor"
)

// increasing this version allows old clients that trying to read the newer version of payload to force user to upgrade
const payloadVersion = 1

type SmartblockLog struct {
	ID   string
	Head string
}

type SmartblockRecordEnvelope struct {
	SmartblockRecord
	AccountID string
	LogID     string
}

type SmartblockRecord struct {
	ID      string
	PrevID  string
	Payload []byte
}

type SignedPbPayload struct {
	DeviceSig []byte // deprecated
	AccSig    []byte
	AccAddr   string
	Data      []byte
	Ver       uint16
}

func init() {
	cbornode.RegisterCborType(SignedPbPayload{})
}

func newSignedPayload(payload []byte, accountKey wallet.Keypair) (*SignedPbPayload, error) {
	accSig, err := accountKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	return &SignedPbPayload{AccAddr: accountKey.Address(), AccSig: accSig, Data: payload, Ver: payloadVersion}, nil
}

func (p *SignedPbPayload) Unmarshal(out proto.Message) error {
	return proto.Unmarshal(p.Data, out)
}

func (p *SmartblockRecord) Unmarshal(out proto.Message) error {
	return proto.Unmarshal(p.Payload, out)
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
