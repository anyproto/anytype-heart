package core

import (
	"context"
	"fmt"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/configfetcher"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/core/domain"
)

func (mw *Middleware) refreshRemoteAccountState() {
	fetcher := mw.app.MustComponent(configfetcher.CName).(configfetcher.ConfigFetcher)
	fetcher.Refetch()
}

func (mw *Middleware) AccountCreate(cctx context.Context, req *pb.RpcAccountCreateRequest) *pb.RpcAccountCreateResponse {
	response := func(account *model.Account, code pb.RpcAccountCreateResponseErrorCode, err error) *pb.RpcAccountCreateResponse {
		var clientConfig *pb.RpcAccountConfig
		m := &pb.RpcAccountCreateResponse{Config: clientConfig, Account: account, Error: &pb.RpcAccountCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	newAccount, err := mw.accountService.AccountCreate(cctx, req)
	code, err := domain.UnwrapCodeFromError[pb.RpcAccountCreateResponseErrorCode](err)
	return response(newAccount, code, err)
}

func (mw *Middleware) AccountRecover(cctx context.Context, _ *pb.RpcAccountRecoverRequest) *pb.RpcAccountRecoverResponse {
	response := func(code pb.RpcAccountRecoverResponseErrorCode, err error) *pb.RpcAccountRecoverResponse {
		m := &pb.RpcAccountRecoverResponse{Error: &pb.RpcAccountRecoverResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	err := mw.accountService.AccountRecover()
	code, err := domain.UnwrapCodeFromError[pb.RpcAccountRecoverResponseErrorCode](err)
	return response(code, err)
}

func (mw *Middleware) AccountSelect(cctx context.Context, req *pb.RpcAccountSelectRequest) *pb.RpcAccountSelectResponse {
	response := func(account *model.Account, code pb.RpcAccountSelectResponseErrorCode, err error) *pb.RpcAccountSelectResponse {
		var clientConfig *pb.RpcAccountConfig
		m := &pb.RpcAccountSelectResponse{Config: clientConfig, Account: account, Error: &pb.RpcAccountSelectResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	acc, err := mw.accountService.AccountSelect(cctx, req)
	code, err := domain.UnwrapCodeFromError[pb.RpcAccountSelectResponseErrorCode](err)
	return response(acc, code, err)
}

func (mw *Middleware) AccountStop(cctx context.Context, req *pb.RpcAccountStopRequest) *pb.RpcAccountStopResponse {
	response := func(code pb.RpcAccountStopResponseErrorCode, err error) *pb.RpcAccountStopResponse {
		m := &pb.RpcAccountStopResponse{Error: &pb.RpcAccountStopResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	err := mw.accountService.AccountStop(req)
	code, err := domain.UnwrapCodeFromError[pb.RpcAccountStopResponseErrorCode](err)
	return response(code, err)
}

func (mw *Middleware) AccountMove(cctx context.Context, req *pb.RpcAccountMoveRequest) *pb.RpcAccountMoveResponse {
	response := func(code pb.RpcAccountMoveResponseErrorCode, err error) *pb.RpcAccountMoveResponse {
		m := &pb.RpcAccountMoveResponse{Error: &pb.RpcAccountMoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	err := mw.accountService.AccountMove(req)
	code, err := domain.UnwrapCodeFromError[pb.RpcAccountMoveResponseErrorCode](err)
	return response(code, err)
}

func (mw *Middleware) AccountDelete(cctx context.Context, req *pb.RpcAccountDeleteRequest) *pb.RpcAccountDeleteResponse {
	response := func(status *model.AccountStatus, code pb.RpcAccountDeleteResponseErrorCode, err error) *pb.RpcAccountDeleteResponse {
		m := &pb.RpcAccountDeleteResponse{Error: &pb.RpcAccountDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Status = status
		}

		return m
	}

	var st *model.AccountStatus
	err := mw.doAccountService(func(a space.Service) (err error) {
		resp, err := a.DeleteAccount(cctx, req.Revert)
		if err != nil {
			return
		}
		st = &model.AccountStatus{
			StatusType:   model.AccountStatusType(resp.Status),
			DeletionDate: resp.DeletionDate.Unix(),
		}
		return
	})

	// so we will receive updated account status
	mw.refreshRemoteAccountState()

	if err == nil {
		return response(st, pb.RpcAccountDeleteResponseError_NULL, nil)
	}
	code := pb.RpcAccountDeleteResponseError_UNKNOWN_ERROR
	switch err {
	case space.ErrSpaceIsDeleted:
		code = pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ALREADY_DELETED
	case space.ErrSpaceDeletionPending:
		code = pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ALREADY_DELETED
	case space.ErrSpaceIsCreated:
		code = pb.RpcAccountDeleteResponseError_ACCOUNT_IS_ACTIVE
	default:
		break
	}
	return response(nil, code, err)
}

func (mw *Middleware) AccountConfigUpdate(_ context.Context, req *pb.RpcAccountConfigUpdateRequest) *pb.RpcAccountConfigUpdateResponse {
	response := func(code pb.RpcAccountConfigUpdateResponseErrorCode, err error) *pb.RpcAccountConfigUpdateResponse {
		m := &pb.RpcAccountConfigUpdateResponse{Error: &pb.RpcAccountConfigUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	if mw.app == nil {
		return response(pb.RpcAccountConfigUpdateResponseError_ACCOUNT_IS_NOT_RUNNING, fmt.Errorf("anytype node not set"))
	}

	conf := mw.app.MustComponent(config.CName).(*config.Config)
	cfg := config.ConfigRequired{}
	cfg.TimeZone = req.TimeZone
	cfg.CustomFileStorePath = req.IPFSStorageAddr
	err := config.WriteJsonConfig(conf.GetConfigPath(), cfg)
	if err != nil {
		return response(pb.RpcAccountConfigUpdateResponseError_FAILED_TO_WRITE_CONFIG, err)
	}

	return response(pb.RpcAccountConfigUpdateResponseError_NULL, err)
}

func (mw *Middleware) AccountRecoverFromLegacyExport(cctx context.Context,
	req *pb.RpcAccountRecoverFromLegacyExportRequest) *pb.RpcAccountRecoverFromLegacyExportResponse {
	response := func(address string, code pb.RpcAccountRecoverFromLegacyExportResponseErrorCode, err error) *pb.RpcAccountRecoverFromLegacyExportResponse {
		m := &pb.RpcAccountRecoverFromLegacyExportResponse{AccountId: address, Error: &pb.RpcAccountRecoverFromLegacyExportResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	address, err := mw.accountService.CreateAccountFromExport(req)
	code, err := domain.UnwrapCodeFromError[pb.RpcAccountRecoverFromLegacyExportResponseErrorCode](err)
	return response(address, code, err)
}
