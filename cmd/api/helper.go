package api

import (
	"context"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// func (a *ApiServer) setInitialParameters(ctx context.Context) {
// 	resp := a.mw.InitialSetParameters(ctx, &pb.RpcInitialSetParametersRequest{
// 		DoNotSaveLogs:      false,
// 		DoNotSendLogs:      false,
// 		DoNotSendTelemetry: false,
// 		LogLevel:           "",
// 		Platform:           "",
// 		Version:            "0.0.1",
// 		Workdir:            "",
// 	})
//
// 	if resp.Error.Code != pb.RpcInitialSetParametersResponseError_NULL {
// 		fmt.Printf("failed to set initial parameters: %v\n", resp.Error.Description)
// 		return
// 	}
// }
//
// func (a *ApiServer) getAccountInfo(ctx context.Context, accountId string) {
// 	resp := a.mw.AccountSelect(ctx, &pb.RpcAccountSelectRequest{
// 		Id: accountId,
// 	})
//
// 	if resp.Error.Code != pb.RpcAccountSelectResponseError_NULL {
// 		fmt.Printf("failed to get account info: %v\n", resp.Error.Description)
// 		return
// 	}
//
// 	a.accountInfo = resp.Account.Info
// }

func (a *ApiServer) resolveTypeToName(spaceId string, typeId string) (string, *pb.RpcObjectSearchResponseError) {
	// Call ObjectSearch for object of specified type and return the name
	resp := a.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(typeId),
			},
		},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return "", resp.Error
	}

	if len(resp.Records) == 0 {
		return "", &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_BAD_INPUT, Description: "Type not found"}
	}

	return resp.Records[0].Fields["name"].GetStringValue(), nil
}
