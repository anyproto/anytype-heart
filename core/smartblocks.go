package core

import (
	"crypto/rand"
	"fmt"

	"github.com/anytypeio/go-anytype-library/schema"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/timestamp"
	libp2pc "github.com/libp2p/go-libp2p-crypto"
	"github.com/segmentio/ksuid"
	tcore "github.com/textileio/go-textile/core"
	tpb "github.com/textileio/go-textile/pb"
)

type SmartBlock interface {
	GetId() string
	GetType() BlockType
	GetThread() *tcore.Thread

	GetVersion(id string) (SmartBlockVersion, error)
	GetVersions(offset string, limit int, metaOnly bool) ([]SmartBlockVersion, error)
	GetLastVersion() (SmartBlockVersion, error)

	AddVersion(SmartBlockVersion) error

	GetExternalFields() *structpb.Struct

	// instead call via GetLastVersion
	//
	//GetBlocks() ([]*Block, error)
	//GetSmartBlocksTree() ([]SmartBlock, error)
	//GetFields() *structpb.Struct

}

type SmartBlockVersion interface {
	GetBlockId() string
	GetVersionId() string
	GetUser() string
	GetDate() *timestamp.Timestamp

	GetFields() *structpb.Struct
	GetExternalFields() *structpb.Struct

	GetBlocks() map[string]*Block
	GetSmartBlocksTree(anytype *Anytype) ([]SmartBlock, error)
	// GetBlocksTree() ([]Block, error)

	//GetSmartBlocksMap() (map[string]SmartBlock, error)
}

func (a *Anytype) SmartBlockCreate(blockType BlockType) (SmartBlock, error) {
	//@todo: cache mill id
	var millJson string
	switch blockType {
	case BlockType_DASHBOARD:
		millJson = schema.Dashboard
	case BlockType_PAGE:
		millJson = schema.Page
	default:
		return nil, fmt.Errorf("can't find schema for this block type: %s", blockType.String())
	}

	config := tpb.AddThreadConfig{
		Name: defaultDocName,
		Key:  ksuid.New().String(),
		Schema: &tpb.AddThreadConfig_Schema{
			Json: millJson,
		},
		Sharing: tpb.Thread_SHARED,
		Type:    tpb.Thread_OPEN,
	}

	// make a new secret
	sk, _, err := libp2pc.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	var thrd *tcore.Thread

	thrd, err = a.Textile.Node().AddThread(config, sk, a.Textile.Node().Account().Address(), true, true)
	if err != nil {
		return nil, err
	}

	switch blockType {
	case BlockType_DASHBOARD:
		return &Dashboard{thread: thrd, node: a}, nil
	case BlockType_PAGE:
		return &Page{thread: thrd, node: a}, nil
	default:
		return nil, fmt.Errorf("can't create smart block for this block type: %s", blockType.String())
	}
}

func (a *Anytype) SmartBlockGet(id string) (SmartBlock, error) {
	thrd, _ := a.predefinedThreadByName(id)
	if thrd == nil {
		thrd = a.Textile.Node().Thread(id)
	}

	tv, err := a.Textile.Node().ThreadView(id)
	if err != nil {
		return nil, err
	}

	switch tv.SchemaNode.Name {
	case BlockType_DASHBOARD.String():
		return &Dashboard{thread: thrd, node: a}, nil
	case BlockType_PAGE.String():
		return &Page{thread: thrd, node: a}, nil
	default:
		return nil, fmt.Errorf("unknown schema name: %s", tv.SchemaNode.Name)
	}
}
