package core

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const wordCount int = 12

func (mw *Middleware) WalletCreate(cctx context.Context, req *pb.RpcWalletCreateRequest) *pb.RpcWalletCreateResponse {
	response := func(mnemonic string, code pb.RpcWalletCreateResponseErrorCode, err error) *pb.RpcWalletCreateResponse {
		m := &pb.RpcWalletCreateResponse{Mnemonic: mnemonic, Error: &pb.RpcWalletCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	mw.m.Lock()
	defer mw.m.Unlock()

	mw.rootPath = req.RootPath
	mw.foundAccounts = nil

	err := os.MkdirAll(mw.rootPath, 0700)
	if err != nil {
		return response("", pb.RpcWalletCreateResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	mnemonic, err := core.WalletGenerateMnemonic(wordCount)
	if err != nil {
		return response("", pb.RpcWalletCreateResponseError_UNKNOWN_ERROR, err)
	}

	mw.mnemonic = mnemonic

	return response(mnemonic, pb.RpcWalletCreateResponseError_NULL, nil)
}

func (mw *Middleware) WalletRecover(cctx context.Context, req *pb.RpcWalletRecoverRequest) *pb.RpcWalletRecoverResponse {
	response := func(code pb.RpcWalletRecoverResponseErrorCode, err error) *pb.RpcWalletRecoverResponse {
		m := &pb.RpcWalletRecoverResponse{Error: &pb.RpcWalletRecoverResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	mw.accountSearchCancel()

	mw.m.Lock()
	defer mw.m.Unlock()

	if mw.mnemonic == req.Mnemonic {
		return response(pb.RpcWalletRecoverResponseError_NULL, nil)
	}

	// test if mnemonic is correct
	_, err := core.WalletAccountAt(req.Mnemonic, 0, "")
	if err != nil {
		return response(pb.RpcWalletRecoverResponseError_BAD_INPUT, err)
	}

	err = os.MkdirAll(req.RootPath, 0700)
	if err != nil {
		return response(pb.RpcWalletRecoverResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	mw.mnemonic = req.Mnemonic
	mw.rootPath = req.RootPath
	mw.foundAccounts = nil

	return response(pb.RpcWalletRecoverResponseError_NULL, nil)
}

func (mw *Middleware) WalletConvert(cctx context.Context, req *pb.RpcWalletConvertRequest) *pb.RpcWalletConvertResponse {
	response := func(mnemonic, entropy string, code pb.RpcWalletConvertResponseErrorCode, err error) *pb.RpcWalletConvertResponse {
		m := &pb.RpcWalletConvertResponse{Mnemonic: mnemonic, Entropy: entropy, Error: &pb.RpcWalletConvertResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if req.Mnemonic == "" && req.Entropy != "" {
		b, err := base64.RawStdEncoding.DecodeString(req.Entropy)
		if err != nil {
			return response("", "", pb.RpcWalletConvertResponseError_BAD_INPUT, fmt.Errorf("invalid base64 format for entropy: %w", err))
		}

		w, err := wallet.WalletFromEntropy(b)
		if err != nil {
			return response("", "", pb.RpcWalletConvertResponseError_BAD_INPUT, fmt.Errorf("invalid entropy: %w", err))
		}
		return response(w.RecoveryPhrase, "", pb.RpcWalletConvertResponseError_NULL, nil)
	} else if req.Entropy == "" && req.Mnemonic != "" {
		w := wallet.WalletFromMnemonic(req.Mnemonic)
		entropy, err := w.Entropy()
		if err != nil {
			return response("", "", pb.RpcWalletConvertResponseError_BAD_INPUT, err)
		}

		base64Entropy := base64.RawStdEncoding.EncodeToString(entropy)
		return response("", base64Entropy, pb.RpcWalletConvertResponseError_NULL, nil)
	}

	return response("", "", pb.RpcWalletConvertResponseError_BAD_INPUT, fmt.Errorf("you should specify neither entropy or mnemonic to convert"))
}

func (mw *Middleware) WalletCreateSession(cctx context.Context, req *pb.RpcWalletCreateSessionRequest) *pb.RpcWalletCreateSessionResponse {
	response := func(token string, code pb.RpcWalletCreateSessionResponseErrorCode, err error) *pb.RpcWalletCreateSessionResponse {
		m := &pb.RpcWalletCreateSessionResponse{Token: token, Error: &pb.RpcWalletCreateSessionResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	// test if mnemonic is correct
	acc, err := core.WalletAccountAt(req.Mnemonic, 0, "")
	if err != nil {
		return response("", pb.RpcWalletCreateSessionResponseError_BAD_INPUT, err)
	}
	priv, err := acc.MarshalBinary()
	if err != nil {
		return response("", pb.RpcWalletCreateSessionResponseError_UNKNOWN_ERROR, err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"expiresAt": time.Now().Add(10 * time.Minute).Unix(),
		"seed":      RandStringRunes(8),
	})

	// Sign and get the complete encoded token as a string using the secret
	tokenString, err := token.SignedString(priv)
	if err != nil {
		return response("", pb.RpcWalletCreateSessionResponseError_UNKNOWN_ERROR, err)
	}

	return response(tokenString, pb.RpcWalletCreateSessionResponseError_NULL, nil)
}

func (mw *Middleware) WalletCloseSession(cctx context.Context, req *pb.RpcWalletCloseSessionRequest) *pb.RpcWalletCloseSessionResponse {
	response := func(code pb.RpcWalletCloseSessionResponseErrorCode, err error) *pb.RpcWalletCloseSessionResponse {
		m := &pb.RpcWalletCloseSessionResponse{Error: &pb.RpcWalletCloseSessionResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if sender, ok := mw.EventSender.(*event.GrpcSender); ok {
		sender.CloseSession(req.Token)
	}
	return response(pb.RpcWalletCloseSessionResponseError_NULL, nil)
}

func (mw *Middleware) Authorize(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		log.Errorf("missing metadata")
	}

	v := md.Get("token")
	if len(v) > 0 {
		tok := v[0]

		token, err := jwt.Parse(tok, func(token *jwt.Token) (interface{}, error) {
			// Don't forget to validate the alg is what you expect:
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}

			acc, err := core.WalletAccountAt(mw.mnemonic, 0, "")
			if err != nil {
				return nil, err
			}
			priv, err := acc.MarshalBinary()
			if err != nil {
				return nil, err
			}
			// hmacSampleSecret is a []byte containing your secret, e.g. []byte("my_secret_key")
			return priv, nil
		})
		if err != nil {
			log.Errorf("parse token: %s", err)
		}

		if token != nil && !token.Valid {
			log.Errorf("invalid token")
		}

		if token != nil {
			fmt.Println(token.Claims)
		}
	}

	resp, err = handler(ctx, req)
	return
}
