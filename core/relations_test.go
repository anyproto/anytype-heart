package core

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	types "google.golang.org/protobuf/types/known/structpb"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

	mw.SetEventSender(eventSender)

	respWalletCreate := mw.WalletCreate(context.Background(), &pb.RpcWalletCreateRequest{RootPath: rootPath})
	require.Equal(t, 0, int(respWalletCreate.Error.Code), respWalletCreate.Error.Description)

	respAccountCreate := mw.AccountCreate(context.Background(), &pb.RpcAccountCreateRequest{Name: "profile"})
	require.Equal(t, 0, int(respAccountCreate.Error.Code), respAccountCreate.Error.Description)

	resp := mw.ObjectCreateSet(context.Background(), &pb.RpcObjectCreateSetRequest{
		Source: []string{bundle.TypeKeyNote.URL()},
	})
	return resp.ObjectId, rootPath, mw, close
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

	respOpenNewPage := mw.ObjectOpen(context.Background(), &pb.RpcObjectOpenRequest{ObjectId: setId})
	require.Equal(t, 0, int(respOpenNewPage.Error.Code), respOpenNewPage.Error.Description)

	relName := "test_str"
	relDesc := "test_str_desc"
	relFormat := model.RelationFormat_tag
	respRelationCreate := mw.ObjectCreateRelation(context.Background(), &pb.RpcObjectCreateRelationRequest{
		Details: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRelationFormat.String(): pbtypes.Float64(float64(relFormat)),
			bundle.RelationKeyName.String():           pbtypes.String(relName),
			bundle.RelationKeyDescription.String():    pbtypes.String(relDesc),
			bundle.RelationKeyType.String():           pbtypes.String(bundle.TypeKeyRelation.URL()),
		}},
	})
	require.Equal(t, 0, int(respRelationCreate.Error.Code), respRelationCreate.Error.Description)
	require.True(t, respRelationCreate.Key != "")
	require.True(t, respRelationCreate.ObjectId != "")

	respObjectRelationAdd := mw.ObjectRelationAdd(context.Background(), &pb.RpcObjectRelationAddRequest{
		ContextId:    setId,
		RelationKeys: []string{respRelationCreate.Key},
	})
	require.Equal(t, 0, int(respObjectRelationAdd.Error.Code), respObjectRelationAdd.Error.Description)

	respObjectSetDetails := mw.ObjectSetDetails(context.Background(), &pb.RpcObjectSetDetailsRequest{
		ContextId: setId,
		Details: []*model.Detail{
			{
				Key:   respRelationCreate.Key,
				Value: nil,
			},
		},
	})
	require.Equal(t, 0, int(respObjectSetDetails.Error.Code), respObjectSetDetails.Error.Description)

	respBlockDataviewRelationAdd := mw.BlockDataviewRelationAdd(context.Background(), &pb.RpcBlockDataviewRelationAddRequest{
		ContextId:    setId,
		BlockId:      "dataview",
		RelationKeys: []string{respRelationCreate.Key},
	})

	require.Equal(t, 0, int(respBlockDataviewRelationAdd.Error.Code), respBlockDataviewRelationAdd.Error.Description)

	respObjectShow := mw.ObjectShow(context.Background(), &pb.RpcObjectShowRequest{ObjectId: setId})
	require.Equal(t, 0, int(respObjectShow.Error.Code), respObjectShow.Error.Description)

	var found bool
	for _, rel := range respObjectShow.ObjectView.RelationLinks {
		if rel.Key == respRelationCreate.Key && rel.Format == relFormat {
			found = true
			break
		}
	}
	require.True(t, found)

	var details *types.Struct
	for _, detEvent := range respObjectShow.ObjectView.Details {
		if detEvent.Id == respRelationCreate.ObjectId {
			details = detEvent.Details
			break
		}
	}
	require.NotNil(t, details, "we should receive details for the relation object")
	require.Equal(t, relName, pbtypes.GetString(details, bundle.RelationKeyName.String()), "we should receive the correct name for the relation object")

	var dataviewBlock *model.Block
	for _, block := range respObjectShow.ObjectView.Blocks {
		if block.Id == "dataview" {
			dataviewBlock = block
			break
		}
	}
	require.NotNil(t, dataviewBlock)

	found = false
	for _, rel := range dataviewBlock.GetDataview().RelationLinks {
		if rel.Format == relFormat && rel.Key == respRelationCreate.Key {
			found = true
			break
		}
	}

	require.True(t, found)

	respRelationCreateOption := mw.ObjectCreateRelationOption(context.Background(), &pb.RpcObjectCreateRelationOptionRequest{
		Details: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRelationKey.String():         pbtypes.String(respRelationCreate.Key),
			bundle.RelationKeyName.String():                pbtypes.String("test_option_text"),
			bundle.RelationKeyRelationOptionColor.String(): pbtypes.String("red"),
		},
		}})

	require.Equal(t, 0, int(respRelationCreateOption.Error.Code), respRelationCreateOption.Error.Description)
	require.NotEmpty(t, respRelationCreateOption.ObjectId)

	respOptionShow := mw.ObjectShow(context.Background(), &pb.RpcObjectShowRequest{ObjectId: respRelationCreateOption.ObjectId})
	require.Equal(t, 0, int(respOptionShow.Error.Code), respOptionShow.Error.Description)

	respObjectSearch := mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Operator:         0,
				RelationKey:      bundle.RelationKeyType.String(),
				RelationProperty: "",
				Condition:        model.BlockContentDataviewFilter_Equal,
				Value:            pbtypes.String(bundle.TypeKeyRelationOption.URL()),
				QuickOption:      0,
			},
			{
				Operator:         0,
				RelationKey:      bundle.RelationKeyRelationKey.String(),
				RelationProperty: "",
				Condition:        model.BlockContentDataviewFilter_Equal,
				Value:            pbtypes.String(respRelationCreate.Key),
				QuickOption:      0,
			},
		},
	})
	require.Equal(t, 0, int(respObjectSearch.Error.Code), respObjectSearch.Error.Description)
	require.Len(t, respObjectSearch.Records, 1)
	require.Equal(t, respRelationCreateOption.ObjectId, respObjectSearch.Records[0].Fields[bundle.RelationKeyId.String()].GetStringValue())

	// add option to relation
	relationSetOptionResponse := mw.ObjectSetDetails(context.Background(), &pb.RpcObjectSetDetailsRequest{
		ContextId: setId,
		Details: []*model.Detail{
			{
				Key:   respRelationCreate.Key,
				Value: pbtypes.StringList([]string{respRelationCreateOption.ObjectId}),
			},
		},
	})
	require.Equal(t, 0, int(relationSetOptionResponse.Error.Code), relationSetOptionResponse.Error.Description)

	mw.RelationListRemoveOption(context.Background(), &pb.RpcRelationListRemoveOptionRequest{
		OptionIds: []string{respRelationCreateOption.ObjectId},
	})

	// check if option has been deleted
	respOptionDeletedSearch := mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Operator:         0,
				RelationKey:      bundle.RelationKeyType.String(),
				RelationProperty: "",
				Condition:        model.BlockContentDataviewFilter_Equal,
				Value:            pbtypes.String(bundle.TypeKeyRelationOption.URL()),
				QuickOption:      0,
			},
			{
				Operator:         0,
				RelationKey:      bundle.RelationKeyRelationKey.String(),
				RelationProperty: "",
				Condition:        model.BlockContentDataviewFilter_Equal,
				Value:            pbtypes.String(respRelationCreate.Key),
				QuickOption:      0,
			},
		},
	})
	require.Equal(t, 0, int(respOptionDeletedSearch.Error.Code), respOptionDeletedSearch.Error.Description)
	require.Len(t, respOptionDeletedSearch.Records, 0)

}
