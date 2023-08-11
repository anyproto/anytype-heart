package core

import (
	"context"
	"github.com/anyproto/anytype-heart/core/application"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) AccountCreate(cctx context.Context, req *pb.RpcAccountCreateRequest) *pb.RpcAccountCreateResponse {
	newAccount, err := mw.applicationService.AccountCreate(cctx, req)
	code := mapErrorCode(err,
		errToCode(application.ErrFailedToStopApplication, pb.RpcAccountCreateResponseError_FAILED_TO_STOP_RUNNING_NODE),
		errToCode(application.ErrFailedToStartApplication, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE),
		errToCode(application.ErrFailedToCreateLocalRepo, pb.RpcAccountCreateResponseError_FAILED_TO_CREATE_LOCAL_REPO),
		errToCode(application.ErrFailedToWriteConfig, pb.RpcAccountCreateResponseError_FAILED_TO_WRITE_CONFIG),
		errToCode(application.ErrSetDetails, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME),
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

func (mw *Middleware) AccountSelect(cctx context.Context, req *pb.RpcAccountSelectRequest) *pb.RpcAccountSelectResponse {
	account, err := mw.applicationService.AccountSelect(cctx, req)
	code := mapErrorCode(err,
		errToCode(application.ErrEmptyAccountID, pb.RpcAccountSelectResponseError_BAD_INPUT),
		errToCode(application.ErrFailedToStopApplication, pb.RpcAccountSelectResponseError_FAILED_TO_STOP_SEARCHER_NODE),
		errToCode(application.ErrNoMnemonicProvided, pb.RpcAccountSelectResponseError_LOCAL_REPO_NOT_EXISTS_AND_MNEMONIC_NOT_SET),
		errToCode(application.ErrFailedToCreateLocalRepo, pb.RpcAccountSelectResponseError_FAILED_TO_CREATE_LOCAL_REPO),
		errToCode(application.ErrFailedToFindAccountInfo, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO),
		errToCode(application.ErrAnotherProcessIsRunning, pb.RpcAccountSelectResponseError_ANOTHER_ANYTYPE_PROCESS_IS_RUNNING),
		errToCode(application.ErrIncompatibleVersion, pb.RpcAccountSelectResponseError_FAILED_TO_FETCH_REMOTE_NODE_HAS_INCOMPATIBLE_PROTO_VERSION),
		errToCode(application.ErrFailedToStartApplication, pb.RpcAccountSelectResponseError_FAILED_TO_RUN_NODE),
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

func (mw *Middleware) AccountDelete(cctx context.Context, req *pb.RpcAccountDeleteRequest) *pb.RpcAccountDeleteResponse {
	status, err := mw.applicationService.AccountDelete(cctx, req)
	code := mapErrorCode(err,
		errToCode(application.ErrAccountIsAlreadyDeleted, pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ALREADY_DELETED),
		errToCode(application.ErrAccountIsActive, pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ACTIVE),
	)
	return &pb.RpcAccountDeleteResponse{
		Status: status,
		Error: &pb.RpcAccountDeleteResponseError{
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
	accountID, err := mw.applicationService.CreateAccountFromExport(req)
	code := mapErrorCode(err,
		errToCode(application.ErrAccountMismatch, pb.RpcAccountRecoverFromLegacyExportResponseError_DIFFERENT_ACCOUNT),
		errToCode(application.ErrBadInput, pb.RpcAccountRecoverFromLegacyExportResponseError_BAD_INPUT),
	)
	return &pb.RpcAccountRecoverFromLegacyExportResponse{
		AccountId: accountID,
		Error: &pb.RpcAccountRecoverFromLegacyExportResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}
