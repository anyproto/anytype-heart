package apicore

import (
	"context"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ChatSubscriptionService interface {
	SubscribeLastMessages(ctx context.Context, chatObjectId string, limit int, subId string, sink chan<- *pb.Event) ([]*model.ChatMessage, error)
	Unsubscribe(chatObjectId string, subId string) error
}

type AccountService interface {
	GetInfo(ctx context.Context) (*model.AccountInfo, error)
}

type EventService interface {
	Broadcast(event *pb.Event)
}

type CrossSpaceSubscriptionService interface {
	Subscribe(req subscription.SubscribeRequest, predicate crossspacesub.Predicate) (*subscription.SubscribeResponse, error)
	Unsubscribe(subId string) error
}

// FileObjectService exposes the subset of fileobject.Service that the API
// needs to stream file/image content directly to clients.
type FileObjectService interface {
	GetFileData(ctx context.Context, objectId string) (files.File, error)
	GetImageData(ctx context.Context, objectId string) (files.Image, error)
	GetImageDataFromRawId(ctx context.Context, fileId domain.FileId) (files.Image, error)
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
	ObjectSearchUnsubscribe(context.Context, *pb.RpcObjectSearchUnsubscribeRequest) *pb.RpcObjectSearchUnsubscribeResponse
	ObjectSetDetails(context.Context, *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse
	ObjectSetIsArchived(context.Context, *pb.RpcObjectSetIsArchivedRequest) *pb.RpcObjectSetIsArchivedResponse
	ObjectListDelete(context.Context, *pb.RpcObjectListDeleteRequest) *pb.RpcObjectListDeleteResponse
	ObjectExport(context.Context, *pb.RpcObjectExportRequest) *pb.RpcObjectExportResponse
	ObjectSetObjectType(context.Context, *pb.RpcObjectSetObjectTypeRequest) *pb.RpcObjectSetObjectTypeResponse

	// Type
	ObjectCreateObjectType(context.Context, *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse

	// List
	ObjectCollectionAdd(context.Context, *pb.RpcObjectCollectionAddRequest) *pb.RpcObjectCollectionAddResponse
	ObjectCollectionRemove(context.Context, *pb.RpcObjectCollectionRemoveRequest) *pb.RpcObjectCollectionRemoveResponse

	// Property
	ObjectRelationAddFeatured(context.Context, *pb.RpcObjectRelationAddFeaturedRequest) *pb.RpcObjectRelationAddFeaturedResponse
	ObjectCreateRelation(context.Context, *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse

	// Tags
	ObjectCreateRelationOption(context.Context, *pb.RpcObjectCreateRelationOptionRequest) *pb.RpcObjectCreateRelationOptionResponse
	RelationListRemoveOption(context.Context, *pb.RpcRelationListRemoveOptionRequest) *pb.RpcRelationListRemoveOptionResponse
	RelationOptions(context.Context, *pb.RpcRelationOptionsRequest) *pb.RpcRelationOptionsResponse

	// File
	FileUpload(context.Context, *pb.RpcFileUploadRequest) *pb.RpcFileUploadResponse

	// Block
	BlockCreate(context.Context, *pb.RpcBlockCreateRequest) *pb.RpcBlockCreateResponse
	BlockPaste(context.Context, *pb.RpcBlockPasteRequest) *pb.RpcBlockPasteResponse
	BlockReplace(context.Context, *pb.RpcBlockReplaceRequest) *pb.RpcBlockReplaceResponse
	BlockListDelete(context.Context, *pb.RpcBlockListDeleteRequest) *pb.RpcBlockListDeleteResponse

	// Chat
	ChatAddMessage(context.Context, *pb.RpcChatAddMessageRequest) *pb.RpcChatAddMessageResponse
	ChatEditMessageContent(context.Context, *pb.RpcChatEditMessageContentRequest) *pb.RpcChatEditMessageContentResponse
	ChatToggleMessageReaction(context.Context, *pb.RpcChatToggleMessageReactionRequest) *pb.RpcChatToggleMessageReactionResponse
	ChatDeleteMessage(context.Context, *pb.RpcChatDeleteMessageRequest) *pb.RpcChatDeleteMessageResponse
	ChatGetMessages(context.Context, *pb.RpcChatGetMessagesRequest) *pb.RpcChatGetMessagesResponse
	ChatGetMessagesByIds(context.Context, *pb.RpcChatGetMessagesByIdsRequest) *pb.RpcChatGetMessagesByIdsResponse
	ChatReadMessages(context.Context, *pb.RpcChatReadMessagesRequest) *pb.RpcChatReadMessagesResponse
	ChatReadReactions(context.Context, *pb.RpcChatReadReactionsRequest) *pb.RpcChatReadReactionsResponse
	ChatSearch(context.Context, *pb.RpcChatSearchRequest) *pb.RpcChatSearchResponse
}
