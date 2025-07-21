package core

import (
	"context"

	"github.com/anyproto/any-sync/net"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/application"
	"github.com/anyproto/anytype-heart/core/session"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/migrator"
	"github.com/anyproto/anytype-heart/util/grpcprocess"
)

func (mw *Middleware) AccountCreate(cctx context.Context, req *pb.RpcAccountCreateRequest) *pb.RpcAccountCreateResponse {
	newAccount, err := mw.applicationService.AccountCreate(cctx, req)
	code := mapErrorCode(err,
		errToCode(config.ErrNetworkFileFailedToRead, pb.RpcAccountCreateResponseError_CONFIG_FILE_INVALID),
		errToCode(config.ErrNetworkFileNotFound, pb.RpcAccountCreateResponseError_CONFIG_FILE_NOT_FOUND),
		errToCode(config.ErrNetworkIdMismatch, pb.RpcAccountCreateResponseError_CONFIG_FILE_NETWORK_ID_MISMATCH),
		errToCode(application.ErrFailedToStopApplication, pb.RpcAccountCreateResponseError_FAILED_TO_STOP_RUNNING_NODE),
		errToCode(application.ErrFailedToStartApplication, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE),
		errToCode(application.ErrFailedToCreateLocalRepo, pb.RpcAccountCreateResponseError_FAILED_TO_CREATE_LOCAL_REPO),
		errToCode(application.ErrFailedToWriteConfig, pb.RpcAccountCreateResponseError_FAILED_TO_WRITE_CONFIG),
		errToCode(application.ErrSetDetails, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME),
		errToCode(context.Canceled, pb.RpcAccountCreateResponseError_ACCOUNT_CREATION_IS_CANCELED),
	)
	return &pb.RpcAccountCreateResponse{
		Config:  nil,
		Account: newAccount,
		Error: &pb.RpcAccountCreateResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountRecover(cctx context.Context, _ *pb.RpcAccountRecoverRequest) *pb.RpcAccountRecoverResponse {
	err := mw.applicationService.AccountRecover()
	code := mapErrorCode(err,
		errToCode(application.ErrNoMnemonicProvided, pb.RpcAccountRecoverResponseError_NEED_TO_RECOVER_WALLET_FIRST),
		errToCode(application.ErrBadInput, pb.RpcAccountRecoverResponseError_BAD_INPUT),
	)
	return &pb.RpcAccountRecoverResponse{
		Error: &pb.RpcAccountRecoverResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountMigrate(cctx context.Context, req *pb.RpcAccountMigrateRequest) *pb.RpcAccountMigrateResponse {
	freeSpaceErr := &migrator.NotEnoughFreeSpaceError{}

	err := mw.applicationService.AccountMigrate(cctx, req)
	code := mapErrorCode(err,
		errToCode(application.ErrBadInput, pb.RpcAccountMigrateResponseError_BAD_INPUT),
		errToCode(application.ErrAccountNotFound, pb.RpcAccountMigrateResponseError_ACCOUNT_NOT_FOUND),
		errToCode(context.Canceled, pb.RpcAccountMigrateResponseError_CANCELED),
		errTypeToCode(&freeSpaceErr, pb.RpcAccountMigrateResponseError_NOT_ENOUGH_FREE_SPACE),
	)

	return &pb.RpcAccountMigrateResponse{
		Error: &pb.RpcAccountMigrateResponseError{
			Code:          code,
			Description:   getErrorDescription(err),
			RequiredSpace: int64(freeSpaceErr.Required),
		},
	}
}

func (mw *Middleware) AccountMigrateCancel(cctx context.Context, req *pb.RpcAccountMigrateCancelRequest) *pb.RpcAccountMigrateCancelResponse {
	err := mw.applicationService.AccountMigrateCancel(cctx, req)
	code := mapErrorCode(err,
		errToCode(application.ErrBadInput, pb.RpcAccountMigrateCancelResponseError_BAD_INPUT),
	)
	return &pb.RpcAccountMigrateCancelResponse{
		Error: &pb.RpcAccountMigrateCancelResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountSelect(cctx context.Context, req *pb.RpcAccountSelectRequest) *pb.RpcAccountSelectResponse {
	account, err := mw.applicationService.AccountSelect(cctx, req)
	code := mapErrorCode(err,
		errToCode(config.ErrNetworkFileFailedToRead, pb.RpcAccountSelectResponseError_CONFIG_FILE_INVALID),
		errToCode(config.ErrNetworkFileNotFound, pb.RpcAccountSelectResponseError_CONFIG_FILE_NOT_FOUND),
		errToCode(config.ErrNetworkIdMismatch, pb.RpcAccountSelectResponseError_CONFIG_FILE_NETWORK_ID_MISMATCH),
		errToCode(application.ErrEmptyAccountID, pb.RpcAccountSelectResponseError_BAD_INPUT),
		errToCode(application.ErrFailedToStopApplication, pb.RpcAccountSelectResponseError_FAILED_TO_STOP_SEARCHER_NODE),
		errToCode(application.ErrNoMnemonicProvided, pb.RpcAccountSelectResponseError_LOCAL_REPO_NOT_EXISTS_AND_MNEMONIC_NOT_SET),
		errToCode(application.ErrFailedToCreateLocalRepo, pb.RpcAccountSelectResponseError_FAILED_TO_CREATE_LOCAL_REPO),
		errToCode(application.ErrFailedToFindAccountInfo, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO),
		errToCode(context.Canceled, pb.RpcAccountSelectResponseError_ACCOUNT_LOAD_IS_CANCELED),
		errToCode(application.ErrAnotherProcessIsRunning, pb.RpcAccountSelectResponseError_ANOTHER_ANYTYPE_PROCESS_IS_RUNNING),
		errToCode(application.ErrIncompatibleVersion, pb.RpcAccountSelectResponseError_FAILED_TO_FETCH_REMOTE_NODE_HAS_INCOMPATIBLE_PROTO_VERSION),
		errToCode(application.ErrFailedToStartApplication, pb.RpcAccountSelectResponseError_FAILED_TO_RUN_NODE),
		errToCode(application.ErrAccountStoreIsNotMigrated, pb.RpcAccountSelectResponseError_ACCOUNT_STORE_NOT_MIGRATED),
	)
	return &pb.RpcAccountSelectResponse{
		Config:  nil,
		Account: account,
		Error: &pb.RpcAccountSelectResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountStop(_ context.Context, req *pb.RpcAccountStopRequest) *pb.RpcAccountStopResponse {
	err := mw.applicationService.AccountStop(req)
	code := mapErrorCode(err,
		errToCode(application.ErrApplicationIsNotRunning, pb.RpcAccountStopResponseError_ACCOUNT_IS_NOT_RUNNING),
		errToCode(application.ErrFailedToRemoveAccountData, pb.RpcAccountStopResponseError_FAILED_TO_REMOVE_ACCOUNT_DATA),
		errToCode(application.ErrFailedToStopApplication, pb.RpcAccountStopResponseError_FAILED_TO_STOP_NODE),
	)
	return &pb.RpcAccountStopResponse{
		Error: &pb.RpcAccountStopResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountChangeNetworkConfigAndRestart(ctx context.Context, req *pb.RpcAccountChangeNetworkConfigAndRestartRequest) *pb.RpcAccountChangeNetworkConfigAndRestartResponse {
	err := mw.applicationService.AccountChangeNetworkConfigAndRestart(ctx, req)
	code := mapErrorCode(err,
		errToCode(application.ErrApplicationIsNotRunning, pb.RpcAccountChangeNetworkConfigAndRestartResponseError_ACCOUNT_IS_NOT_RUNNING),
		errToCode(application.ErrFailedToStopApplication, pb.RpcAccountChangeNetworkConfigAndRestartResponseError_ACCOUNT_FAILED_TO_STOP),
		errToCode(config.ErrNetworkFileFailedToRead, pb.RpcAccountChangeNetworkConfigAndRestartResponseError_CONFIG_FILE_INVALID),
		errToCode(config.ErrNetworkFileNotFound, pb.RpcAccountChangeNetworkConfigAndRestartResponseError_CONFIG_FILE_NOT_FOUND),
		errToCode(config.ErrNetworkIdMismatch, pb.RpcAccountChangeNetworkConfigAndRestartResponseError_CONFIG_FILE_NETWORK_ID_MISMATCH),
	)
	return &pb.RpcAccountChangeNetworkConfigAndRestartResponse{
		Error: &pb.RpcAccountChangeNetworkConfigAndRestartResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountMove(cctx context.Context, req *pb.RpcAccountMoveRequest) *pb.RpcAccountMoveResponse {
	err := mw.applicationService.AccountMove(req)
	code := mapErrorCode(err,
		errToCode(application.ErrFailedToGetConfig, pb.RpcAccountMoveResponseError_FAILED_TO_GET_CONFIG),
		errToCode(application.ErrFailedToIdentifyAccountDir, pb.RpcAccountMoveResponseError_FAILED_TO_IDENTIFY_ACCOUNT_DIR),
		errToCode(application.ErrFailedToCreateLocalRepo, pb.RpcAccountMoveResponseError_FAILED_TO_CREATE_LOCAL_REPO),
		errToCode(application.ErrFailedToRemoveAccountData, pb.RpcAccountMoveResponseError_FAILED_TO_REMOVE_ACCOUNT_DATA),
		errToCode(application.ErrFailedToStopApplication, pb.RpcAccountMoveResponseError_FAILED_TO_STOP_NODE),
		errToCode(application.ErrFailedToWriteConfig, pb.RpcAccountMoveResponseError_FAILED_TO_WRITE_CONFIG),
	)
	return &pb.RpcAccountMoveResponse{
		Error: &pb.RpcAccountMoveResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountDelete(cctx context.Context, _ *pb.RpcAccountDeleteRequest) *pb.RpcAccountDeleteResponse {
	status, err := mw.applicationService.AccountDelete(cctx)
	code := mapErrorCode(err,
		errToCode(application.ErrAccountIsAlreadyDeleted, pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ALREADY_DELETED),
		errToCode(net.ErrUnableToConnect, pb.RpcAccountDeleteResponseError_UNABLE_TO_CONNECT),
	)
	return &pb.RpcAccountDeleteResponse{
		Status: status,
		Error: &pb.RpcAccountDeleteResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountRevertDeletion(cctx context.Context, req *pb.RpcAccountRevertDeletionRequest) *pb.RpcAccountRevertDeletionResponse {
	status, err := mw.applicationService.AccountRevertDeletion(cctx)
	code := mapErrorCode(err,
		errToCode(application.ErrAccountIsActive, pb.RpcAccountRevertDeletionResponseError_ACCOUNT_IS_ACTIVE),
		errToCode(net.ErrUnableToConnect, pb.RpcAccountRevertDeletionResponseError_UNABLE_TO_CONNECT),
	)
	return &pb.RpcAccountRevertDeletionResponse{
		Status: status,
		Error: &pb.RpcAccountRevertDeletionResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountConfigUpdate(_ context.Context, req *pb.RpcAccountConfigUpdateRequest) *pb.RpcAccountConfigUpdateResponse {
	err := mw.applicationService.AccountConfigUpdate(req)
	code := mapErrorCode(err,
		errToCode(application.ErrApplicationIsNotRunning, pb.RpcAccountConfigUpdateResponseError_ACCOUNT_IS_NOT_RUNNING),
		errToCode(application.ErrFailedToWriteConfig, pb.RpcAccountConfigUpdateResponseError_FAILED_TO_WRITE_CONFIG),
	)
	return &pb.RpcAccountConfigUpdateResponse{
		Error: &pb.RpcAccountConfigUpdateResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountRecoverFromLegacyExport(cctx context.Context, req *pb.RpcAccountRecoverFromLegacyExportRequest) *pb.RpcAccountRecoverFromLegacyExportResponse {
	resp, err := mw.applicationService.RecoverFromLegacy(req)
	code := mapErrorCode(err,
		errToCode(application.ErrAccountMismatch, pb.RpcAccountRecoverFromLegacyExportResponseError_DIFFERENT_ACCOUNT),
		errToCode(application.ErrBadInput, pb.RpcAccountRecoverFromLegacyExportResponseError_BAD_INPUT),
	)
	return &pb.RpcAccountRecoverFromLegacyExportResponse{
		AccountId:       resp.AccountId,
		PersonalSpaceId: resp.PersonalSpaceId,
		Error: &pb.RpcAccountRecoverFromLegacyExportResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountEnableLocalNetworkSync(_ context.Context, req *pb.RpcAccountEnableLocalNetworkSyncRequest) *pb.RpcAccountEnableLocalNetworkSyncResponse {
	err := mw.applicationService.EnableLocalNetworkSync()
	code := mapErrorCode(err,
		errToCode(application.ErrApplicationIsNotRunning, pb.RpcAccountEnableLocalNetworkSyncResponseError_ACCOUNT_IS_NOT_RUNNING),
	)
	return &pb.RpcAccountEnableLocalNetworkSyncResponse{
		Error: &pb.RpcAccountEnableLocalNetworkSyncResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountChangeJsonApiAddr(ctx context.Context, req *pb.RpcAccountChangeJsonApiAddrRequest) *pb.RpcAccountChangeJsonApiAddrResponse {
	err := mw.applicationService.AccountChangeJsonApiAddr(ctx, req.ListenAddr)
	code := mapErrorCode(err,
		errToCode(application.ErrApplicationIsNotRunning, pb.RpcAccountChangeJsonApiAddrResponseError_ACCOUNT_IS_NOT_RUNNING),
	)
	return &pb.RpcAccountChangeJsonApiAddrResponse{
		Error: &pb.RpcAccountChangeJsonApiAddrResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountLocalLinkNewChallenge(ctx context.Context, request *pb.RpcAccountLocalLinkNewChallengeRequest) *pb.RpcAccountLocalLinkNewChallengeResponse {
	info := getClientInfo(ctx)
	info.Name = request.AppName
	challengeId, err := mw.applicationService.LinkLocalStartNewChallenge(request.Scope, &info)
	code := mapErrorCode(err,
		errToCode(session.ErrTooManyChallengeRequests, pb.RpcAccountLocalLinkNewChallengeResponseError_TOO_MANY_REQUESTS),
		errToCode(application.ErrApplicationIsNotRunning, pb.RpcAccountLocalLinkNewChallengeResponseError_ACCOUNT_IS_NOT_RUNNING),
	)

	return &pb.RpcAccountLocalLinkNewChallengeResponse{
		ChallengeId: challengeId,
		Error: &pb.RpcAccountLocalLinkNewChallengeResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountLocalLinkSolveChallenge(_ context.Context, req *pb.RpcAccountLocalLinkSolveChallengeRequest) *pb.RpcAccountLocalLinkSolveChallengeResponse {
	token, appKey, err := mw.applicationService.LinkLocalSolveChallenge(req)
	code := mapErrorCode(err,
		errToCode(session.ErrChallengeTriesExceeded, pb.RpcAccountLocalLinkSolveChallengeResponseError_CHALLENGE_ATTEMPTS_EXCEEDED),
		errToCode(session.ErrChallengeSolutionWrong, pb.RpcAccountLocalLinkSolveChallengeResponseError_INCORRECT_ANSWER),
		errToCode(session.ErrChallengeIdNotFound, pb.RpcAccountLocalLinkSolveChallengeResponseError_INVALID_CHALLENGE_ID),
		errToCode(application.ErrApplicationIsNotRunning, pb.RpcAccountLocalLinkSolveChallengeResponseError_ACCOUNT_IS_NOT_RUNNING),
	)
	return &pb.RpcAccountLocalLinkSolveChallengeResponse{
		SessionToken: token,
		AppKey:       appKey,
		Error: &pb.RpcAccountLocalLinkSolveChallengeResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountLocalLinkCreateApp(_ context.Context, req *pb.RpcAccountLocalLinkCreateAppRequest) *pb.RpcAccountLocalLinkCreateAppResponse {
	appKey, err := mw.applicationService.LinkLocalCreateApp(req)
	code := mapErrorCode(err,
		errToCode(application.ErrApplicationIsNotRunning, pb.RpcAccountLocalLinkCreateAppResponseError_ACCOUNT_IS_NOT_RUNNING),
	)
	return &pb.RpcAccountLocalLinkCreateAppResponse{
		AppKey: appKey,
		Error: &pb.RpcAccountLocalLinkCreateAppResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountLocalLinkListApps(_ context.Context, req *pb.RpcAccountLocalLinkListAppsRequest) *pb.RpcAccountLocalLinkListAppsResponse {
	apps, err := mw.applicationService.LinkLocalListApps()
	code := mapErrorCode(err,
		errToCode(application.ErrApplicationIsNotRunning, pb.RpcAccountLocalLinkListAppsResponseError_ACCOUNT_IS_NOT_RUNNING),
	)

	return &pb.RpcAccountLocalLinkListAppsResponse{
		App: apps,
		Error: &pb.RpcAccountLocalLinkListAppsResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) AccountLocalLinkRevokeApp(_ context.Context, req *pb.RpcAccountLocalLinkRevokeAppRequest) *pb.RpcAccountLocalLinkRevokeAppResponse {
	err := mw.applicationService.LinkLocalRevokeApp(req)
	code := mapErrorCode(err,
		errToCode(walletComp.ErrAppLinkNotFound, pb.RpcAccountLocalLinkRevokeAppResponseError_NOT_FOUND),
		errToCode(application.ErrApplicationIsNotRunning, pb.RpcAccountLocalLinkRevokeAppResponseError_ACCOUNT_IS_NOT_RUNNING),
	)
	return &pb.RpcAccountLocalLinkRevokeAppResponse{
		Error: &pb.RpcAccountLocalLinkRevokeAppResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func getClientInfo(ctx context.Context) pb.EventAccountLinkChallengeClientInfo {
	info, ok := grpcprocess.FromContext(ctx)
	if !ok {
		return pb.EventAccountLinkChallengeClientInfo{}
	}
	return pb.EventAccountLinkChallengeClientInfo{
		ProcessName: info.Name,
		ProcessPath: info.Path,
	}
}
