package core

import (
	"crypto/rand"
	"fmt"

	libp2pc "github.com/libp2p/go-libp2p-crypto"
	"github.com/segmentio/ksuid"
	tcore "github.com/textileio/go-textile/core"
	tpb "github.com/textileio/go-textile/pb"
)

func (a *Anytype) newBlockThread(schema string) (*tcore.Thread, error) {
	config := tpb.AddThreadConfig{
		Name: defaultDocName,
		Key:  ksuid.New().String(),
		Schema: &tpb.AddThreadConfig_Schema{
			Json: schema,
		},
		Sharing: tpb.Thread_SHARED,
		Type:    tpb.Thread_OPEN,
	}

	// make a new secret
	sk, _, err := libp2pc.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	return a.Textile.Node().AddThread(config, sk, a.Textile.Node().Account().Address(), true, true)
}

func (a *Anytype) SmartBlockGet(id string) (*SmartBlock, error) {
	thrd, _ := a.predefinedThreadByName(id)
	if thrd == nil {
		thrd = a.Textile.Node().Thread(id)
	}

	tv, err := a.Textile.Node().ThreadView(id)
	if err != nil {
		return nil, err
	}

	switch tv.SchemaNode.Name {
	case "dashboard", "page", "dataview":
		return &SmartBlock{thread: thrd, node: a}, nil
	default:
		return nil, fmt.Errorf("unknown schema name: %s", tv.SchemaNode.Name)
	}
}
