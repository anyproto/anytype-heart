package apicore

import (
	"context"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type AccountService interface {
	GetInfo(ctx context.Context) (*model.AccountInfo, error)
}

type ExportService interface {
	ExportSingleInMemory(ctx context.Context, spaceId string, objectId string, format model.ExportFormat) (res string, err error)
}

type ClientCommands interface {
	// Wallet
	AccountLocalLinkNewChallenge(context.Context, *pb.RpcAccountLocalLinkNewChallengeRequest) *pb.RpcAccountLocalLinkNewChallengeResponse
	AccountLocalLinkSolveChallenge(context.Context, *pb.RpcAccountLocalLinkSolveChallengeRequest) *pb.RpcAccountLocalLinkSolveChallengeResponse
	WalletCreateSession(context.Context, *pb.RpcWalletCreateSessionRequest) *pb.RpcWalletCreateSessionResponse

	// Space
	WorkspaceCreate(context.Context, *pb.RpcWorkspaceCreateRequest) *pb.RpcWorkspaceCreateResponse
	WorkspaceOpen(context.Context, *pb.RpcWorkspaceOpenRequest) *pb.RpcWorkspaceOpenResponse
	WorkspaceSetInfo(context.Context, *pb.RpcWorkspaceSetInfoRequest) *pb.RpcWorkspaceSetInfoResponse
	SpaceRequestApprove(context.Context, *pb.RpcSpaceRequestApproveRequest) *pb.RpcSpaceRequestApproveResponse
	SpaceRequestDecline(context.Context, *pb.RpcSpaceRequestDeclineRequest) *pb.RpcSpaceRequestDeclineResponse
	SpaceParticipantRemove(context.Context, *pb.RpcSpaceParticipantRemoveRequest) *pb.RpcSpaceParticipantRemoveResponse
	SpaceParticipantPermissionsChange(context.Context, *pb.RpcSpaceParticipantPermissionsChangeRequest) *pb.RpcSpaceParticipantPermissionsChangeResponse

	// Object
	ObjectShow(context.Context, *pb.RpcObjectShowRequest) *pb.RpcObjectShowResponse
	ObjectCreate(context.Context, *pb.RpcObjectCreateRequest) *pb.RpcObjectCreateResponse
	ObjectCreateBookmark(context.Context, *pb.RpcObjectCreateBookmarkRequest) *pb.RpcObjectCreateBookmarkResponse
	ObjectSearch(context.Context, *pb.RpcObjectSearchRequest) *pb.RpcObjectSearchResponse
	ObjectSearchSubscribe(context.Context, *pb.RpcObjectSearchSubscribeRequest) *pb.RpcObjectSearchSubscribeResponse
	ObjectSetDetails(context.Context, *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse
	ObjectSetIsArchived(context.Context, *pb.RpcObjectSetIsArchivedRequest) *pb.RpcObjectSetIsArchivedResponse
	ObjectListExport(context.Context, *pb.RpcObjectListExportRequest) *pb.RpcObjectListExportResponse

	// List
	ObjectCollectionAdd(context.Context, *pb.RpcObjectCollectionAddRequest) *pb.RpcObjectCollectionAddResponse
	ObjectCollectionRemove(context.Context, *pb.RpcObjectCollectionRemoveRequest) *pb.RpcObjectCollectionRemoveResponse

	// Property
	ObjectRelationAddFeatured(context.Context, *pb.RpcObjectRelationAddFeaturedRequest) *pb.RpcObjectRelationAddFeaturedResponse

	// Block
	BlockCreate(context.Context, *pb.RpcBlockCreateRequest) *pb.RpcBlockCreateResponse
	BlockPaste(context.Context, *pb.RpcBlockPasteRequest) *pb.RpcBlockPasteResponse
}
