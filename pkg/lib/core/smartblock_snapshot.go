package core

import (
	"fmt"
	"time"

	"github.com/gogo/protobuf/types"
	cid "github.com/ipfs/go-cid"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/crypto"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/vclock"
)

type SmartBlockSnapshot interface {
	State() vclock.VClock
	Creator() (string, error)
	CreatedDate() *time.Time
	ReceivedDate() *time.Time
	Blocks() ([]*model.Block, error)
	Meta() (*SmartBlockMeta, error)
	PublicWebURL() (string, error)
}

var ErrFailedToDecodeSnapshot = fmt.Errorf("failed to decode pb block snapshot")

type smartBlockSnapshot struct {
	blocks  []*model.Block
	details *types.Struct
	state   vclock.VClock

	threadID thread.ID
	recordID cid.Cid
	eventID  cid.Cid
	key      crypto.DecryptionKey
	creator  string
	date     *types.Timestamp
	node     *Anytype
}

func (snapshot smartBlockSnapshot) State() vclock.VClock {
	return snapshot.state
}

func (snapshot smartBlockSnapshot) Creator() (string, error) {
	return snapshot.creator, nil
}

func (snapshot smartBlockSnapshot) CreatedDate() *time.Time {
	return nil
}

func (snapshot smartBlockSnapshot) ReceivedDate() *time.Time {
	return nil
}

func (snapshot smartBlockSnapshot) Blocks() ([]*model.Block, error) {
	// todo: blocks lazy loading
	return snapshot.blocks, nil
}

func (snapshot smartBlockSnapshot) Meta() (*SmartBlockMeta, error) {
	return &SmartBlockMeta{Details: snapshot.details}, nil
}

func (snapshot smartBlockSnapshot) PublicWebURL() (string, error) {
	return "", fmt.Errorf("not implemented")
	/*ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	ipfs := snapshot.node.threadService.Threads()
	if snapshot.eventID == cid.Undef {
		// todo: extract from recordID?
		return "", fmt.Errorf("eventID is empty")
	}

	event, err := cbor.GetEvent(ctx, ipfs, snapshot.eventID)
	if err != nil {
		return "", fmt.Errorf("failed to get snapshot event: %w", err)
	}

	header, err := event.GetHeader(ctx, ipfs, snapshot.key)
	if err != nil {
		return "", fmt.Errorf("failed to get snapshot event header: %w", err)
	}

	bodyKey, err := header.Key()
	if err != nil {
		return "", fmt.Errorf("failed to get body decryption key: %w", err)
	}

	bodyKeyBin, err := bodyKey.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to get marshal decryption key: %w", err)
	}

	return fmt.Sprintf(
		snapshot.node.opts.WebGatewayBaseUrl+snapshot.node.opts.WebGatewaySnapshotUri,
		snapshot.threadID.String(),
		event.BodyID().String(),
		base64.RawURLEncoding.EncodeToString(bodyKeyBin),
	), nil*/
}

type SnapshotWithMetadata struct {
	storage.SmartBlockSnapshot
	Creator  string
	RecordID cid.Cid
	EventID  cid.Cid
}
