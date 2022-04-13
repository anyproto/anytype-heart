package core

import (
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
	types2 "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
)

func getBlockById(id string, blocks []*model.Block) *model.Block {
	for _, block := range blocks {
		if block.Id == id {
			return block
		}
	}
	return nil
}

func getRelationByKey(relations []*model.Relation, key string) *model.Relation {
	for _, relation := range relations {
		if relation.Key == key {
			return relation
		}
	}
	return nil
}

func start(t *testing.T, eventSender event.Sender) (rootPath string, mw *Middleware, close func() error) {
	if debug, ok := os.LookupEnv("ANYPROF"); ok && debug != "" {
		go func() {
			http.ListenAndServe(debug, nil)
		}()
	}
	mw = New()
	rootPath, err := ioutil.TempDir(os.TempDir(), "anytype_*")
	require.NoError(t, err)
	close = func() error { return os.RemoveAll(rootPath) }

	if eventSender == nil {
		eventSender = event.NewCallbackSender(func(event *pb.Event) {
			log.Debugf("got event: %s", pbtypes.Sprint(event))
		})
	}

	mw.EventSender = eventSender

	respWalletCreate := mw.WalletCreate(&pb.RpcWalletCreateRequest{RootPath: rootPath})
	require.Equal(t, 0, int(respWalletCreate.Error.Code), respWalletCreate.Error.Description)

	respAccountCreate := mw.AccountCreate(&pb.RpcAccountCreateRequest{Name: "profile", AlphaInviteCode: "elbrus"})
	require.Equal(t, 0, int(respAccountCreate.Error.Code), respAccountCreate.Error.Description)

	return rootPath, mw, close
}

func addRelation(t *testing.T, contextId string, mw *Middleware) (key string, name string) {
	name = bson.NewObjectId().String()
	respDataviewRelationAdd := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
		ContextId: contextId,
		BlockId:   "dataview",
		Relation: &model.Relation{
			Key:      "",
			Format:   0,
			Name:     name,
			ReadOnly: false,
		},
	})

	require.Equal(t, 0, int(respDataviewRelationAdd.Error.Code), respDataviewRelationAdd.Error.Description)
	key = respDataviewRelationAdd.RelationKey
	return
}

func TestArchiveIndex(t *testing.T) {
	_, mw, close := start(t, nil)
	defer close()

	resp := mw.BlockCreatePage(&pb.RpcBlockCreatePageRequest{})
	require.Equal(t, int(resp.Error.Code), 0, resp.Error.Description)

	respArchive := mw.ObjectListSetIsArchived(&pb.RpcObjectListSetIsArchivedRequest{
		ObjectIds:  []string{resp.TargetId},
		IsArchived: true,
	})
	require.Equal(t, int(respArchive.Error.Code), 0, respArchive.Error.Description)
	time.Sleep(time.Millisecond * 500) // todo: remove after we have moved to the callbacks

	d, err := mw.GetAnytype().ObjectStore().GetDetails(resp.TargetId)
	require.NoError(t, err)
	require.True(t, pbtypes.Get(d.GetDetails(), bundle.RelationKeyIsArchived.String()).Equal(pbtypes.Bool(true)))

	respArchive = mw.ObjectListSetIsArchived(&pb.RpcObjectListSetIsArchivedRequest{
		ObjectIds:  []string{resp.TargetId},
		IsArchived: false,
	})
	require.Equal(t, int(respArchive.Error.Code), 0, respArchive.Error.Description)
	time.Sleep(time.Millisecond * 500) // todo: remove after we have moved to the callbacks
	d, err = mw.GetAnytype().ObjectStore().GetDetails(resp.TargetId)
	require.NoError(t, err)
	require.True(t, pbtypes.Get(d.GetDetails(), bundle.RelationKeyIsArchived.String()).Equal(pbtypes.Bool(false)))
}

func getEventObjectRelationAmend(msgs []*pb.EventMessage) *pb.EventObjectRelationsAmend {
	for _, msg := range msgs {
		if v, ok := msg.Value.(*pb.EventMessageValueOfObjectRelationsAmend); ok {
			return v.ObjectRelationsAmend
		}
	}
	return nil
}

func getEventObjectShow(msgs []*pb.EventMessage) *pb.EventObjectShow {
	for _, msg := range msgs {
		if v, ok := msg.Value.(*pb.EventMessageValueOfObjectShow); ok {
			return v.ObjectShow
		}
	}
	return nil
}

func getDetailsForContext(msgs []*pb.EventObjectDetailsSet, contextId string) *types2.Struct {
	for _, msg := range msgs {
		if msg.Id == contextId {
			return msg.Details
		}
	}
	return nil
}
