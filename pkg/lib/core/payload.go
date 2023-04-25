package core

import (
	"github.com/gogo/protobuf/proto"
	cbornode "github.com/ipfs/go-ipld-cbor"
)

// increasing this version allows old clients that trying to read the newer version of payload to force user to upgrade
const payloadVersion = 1

type SmartblockLog struct {
	ID          string
	Head        string
	HeadCounter int64
}

type SmartblockRecordEnvelope struct {
	SmartblockRecord
	AccountID string
	LogID     string
}

type SmartblockRecordWithThreadID struct {
	SmartblockRecordEnvelope
	ThreadID string
}

type ThreadRecordInfo struct {
	LogId    string
	ThreadID string
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

func (p *SignedPbPayload) Unmarshal(out proto.Message) error {
	return proto.Unmarshal(p.Data, out)
}

func (p *SmartblockRecord) Unmarshal(out proto.Message) error {
	return proto.Unmarshal(p.Payload, out)
}

func (p *SignedPbPayload) Verify() error {
	return nil
}
