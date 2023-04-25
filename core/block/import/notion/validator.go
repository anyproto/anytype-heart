package notion

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var (
	ErrorInternal          = errors.New("internal")
	ErrorUnauthorized      = errors.New("unauthorized")
	ErrorForbidden         = errors.New("forbidden")
	ErrorNotionUnavailable = errors.New("unavailable")
)

var logger = logging.Logger("notion-ping")

const (
	endpoint = "/users?page_size=1"
)

type Service struct {
	client *client.Client
}

// NewPingService is a constructor for PingService
func NewPingService(client *client.Client) *Service {
	return &Service{
		client: client,
	}
}

// Ping is function to validate token, it calls users endpoint and checks given error,
func (s *Service) Ping(ctx context.Context, apiKey string) error {
	req, err := s.client.PrepareRequest(ctx, apiKey, http.MethodGet, endpoint, &bytes.Buffer{})
	if err != nil {
		logger.With(zap.String("method", "PrepareRequest")).Error(err)
		return errors.Wrap(ErrorInternal, fmt.Sprintf("ping: %s", err.Error()))
	}
	res, err := s.client.HTTPClient.Do(req)
	if err != nil {
		logger.With(zap.String("method", "Do")).Error(err)
		return errors.Wrap(ErrorInternal, fmt.Sprintf("ping: %s", err.Error()))
	}
	defer res.Body.Close()

	b, err := ioutil.ReadAll(res.Body)

	if err != nil {
		logger.With(zap.String("method", "ioutil.ReadAll")).Error(err)
		return errors.Wrap(ErrorInternal, err.Error())
	}
	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusUnauthorized {
			return ErrorUnauthorized
		}
		if res.StatusCode == http.StatusForbidden {
			return ErrorForbidden
		}
		if isNotionUnavailableError(res.StatusCode) {
			return ErrorNotionUnavailable
		}
		err = client.TransformHTTPCodeToError(b)
		if err != nil {
			return errors.Wrap(ErrorInternal, err.Error())
		}
	}
	return nil
}

func isNotionUnavailableError(code int) bool {
	return code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}

type TokenValidator struct {
	ping *Service
}

func NewTokenValidator() *TokenValidator {
	cl := client.NewClient()
	return &TokenValidator{
		ping: NewPingService(cl),
	}
}

// Validate calls Notion API with given api key and check, if error is unauthorized
func (v TokenValidator) Validate(ctx context.Context,
	apiKey string) pb.RpcObjectImportNotionValidateTokenResponseErrorCode {
	err := v.ping.Ping(ctx, apiKey)
	if errors.Is(err, ErrorInternal) {
		return pb.RpcObjectImportNotionValidateTokenResponseError_INTERNAL_ERROR
	}
	if errors.Is(err, ErrorUnauthorized) {
		return pb.RpcObjectImportNotionValidateTokenResponseError_UNAUTHORIZED
	}
	if errors.Is(err, ErrorForbidden) {
		return pb.RpcObjectImportNotionValidateTokenResponseError_FORBIDDEN
	}
	if errors.Is(err, ErrorNotionUnavailable) {
		return pb.RpcObjectImportNotionValidateTokenResponseError_SERVICE_UNAVAILABLE
	}
	return pb.RpcObjectImportNotionValidateTokenResponseError_NULL
}
