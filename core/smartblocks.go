package core

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/types"
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

func (a *Anytype) GetSmartBlock(id string) (*SmartBlock, error) {
	thrd, _ := a.predefinedThreadByName(id)
	if thrd == nil {
		thrd = a.Textile.Node().Thread(id)
	}

	tv, err := a.Textile.Node().ThreadView(id)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(tv.SchemaNode.Name) {
	case "dashboard", "page", "dataview":
		return &SmartBlock{thread: thrd, node: a}, nil
	default:
		return nil, fmt.Errorf("unknown schema name: %s", tv.SchemaNode.Name)
	}
}

func (a *Anytype) smartBlockVersionWithoutPermissions(id string) *SmartBlockVersion {
	return &SmartBlockVersion{
		node: a,
		model: &storage.BlockWithDependentBlocks{
			Block: &model.Block{
				Id: id,
				Fields: &types.Struct{Fields: map[string]*types.Value{
					"name": {Kind: &types.Value_StringValue{StringValue: "Inaccessible block"}},
					"icon": {Kind: &types.Value_StringValue{StringValue: ":no_entry_sign:"}},
				}},
				Permissions: &model.BlockPermissions{
					// all permissions are false by default
				},
				// we don't know the block type for sure, lets set a page
				Content: &model.BlockCore{&model.BlockCoreContentOfPage{Page: &model.BlockContentPage{}}},
			}},
	}
}
