package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
)

func start(t *testing.T, eventSender event.Sender) (setId string, rootPath string, mw *Middleware, close func() error) {
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

	resp := mw.ObjectCreateSet(&pb.RpcObjectCreateSetRequest{
		Source: []string{bundle.TypeKeyNote.URL()},
	})
	return resp.Id, rootPath, mw, close
}

func TestRelations_New_Account(t *testing.T) {
	if os.Getenv("ANYTYPE_TEST_INTEGRATION") != "1" {
		t.Skip("ANYTYPE_TEST_INTEGRATION not set")
		return
	}
	eventHandler := func(event *pb.Event) {
		return
	}

	setId, _, mw, appClose := start(t, event.NewCallbackSender(func(event *pb.Event) {
		eventHandler(event)
	}))
	defer appClose()

	respOpenNewPage := mw.ObjectOpen(&pb.RpcObjectOpenRequest{ObjectId: setId})
	require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)

	respRelationCreate := mw.RelationCreate(&pb.RpcRelationCreateRequest{
		Relation: &model.Relation{
			Format:      model.RelationFormat_longtext,
			Name:        "test_str",
			Description: "test_str_desc",
		},
	})
	require.Equal(t, 0, int(respRelationCreate.Error.Code), respRelationCreate.Error.Description)
	require.True(t, respRelationCreate.Key != "")
	require.True(t, respRelationCreate.Id != "")

	respObjectRelationAdd := mw.ObjectRelationAdd(&pb.RpcObjectRelationAddRequest{
		ContextId:   setId,
		RelationIds: []string{respRelationCreate.Id},
	})
	require.Equal(t, 0, int(respObjectRelationAdd.Error.Code), respObjectRelationAdd.Error.Description)

	respObjectSetDetails := mw.ObjectSetDetails(&pb.RpcObjectSetDetailsRequest{
		ContextId: setId,
		Details: []*pb.RpcObjectSetDetailsDetail{
			{
				Key:   respRelationCreate.Key,
				Value: nil,
			},
		},
	})
	require.Equal(t, 0, int(respObjectSetDetails.Error.Code), respObjectSetDetails.Error.Description)

	respBlockDataviewRelationAdd := mw.BlockDataviewRelationAdd(&pb.RpcBlockDataviewRelationAddRequest{
		ContextId:  setId,
		BlockId:    "dataview",
		RelationId: respRelationCreate.Id,
	})

	require.Equal(t, 0, int(respBlockDataviewRelationAdd.Error.Code), respBlockDataviewRelationAdd.Error.Description)

	respObjectShow := mw.ObjectShow(&pb.RpcObjectShowRequest{ObjectId: setId})
	require.Equal(t, 0, int(respObjectShow.Error.Code), respObjectShow.Error.Description)

	var found bool
	for _, rel := range respObjectShow.Event.Messages[0].GetObjectShow().Relations {
		if rel.Id == respRelationCreate.Id && rel.Key == respRelationCreate.Key {
			found = true
			break
		}
	}
	require.True(t, found)

	var dataviewBlock *model.Block
	for _, block := range respObjectShow.Event.Messages[0].GetObjectShow().Blocks {
		if block.Id == "dataview" {
			dataviewBlock = block
			break
		}
	}
	require.NotNil(t, dataviewBlock)

	found = false
	for _, rel := range dataviewBlock.GetDataview().RelationLinks {
		if rel.Id == respRelationCreate.Id && rel.Key == respRelationCreate.Key {
			found = true
			break
		}
	}
	require.True(t, found)
}
