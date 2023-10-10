package notion

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api/client"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var (
	ErrorInternal          = errors.New("internal")
	ErrorUnauthorized      = errors.New("unauthorized")
	ErrorForbidden         = errors.New("forbidden")
	ErrorNotionUnavailable = errors.New("unavailable")
)

var log = logging.Logger("notion-ping")

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
	req, err := s.client.PrepareRequest(ctx, apiKey, http.MethodGet, endpoint, nil)
	if err != nil {
		log.With(zap.String("method", "PrepareRequest")).Error(err)
		return fmt.Errorf("%w: ping: %w", ErrorInternal, err)
	}
	res, err := s.client.HTTPClient.Do(req)
	if err != nil {
		log.With(zap.String("method", "Do")).Error(err)
		return fmt.Errorf("%w: ping: %w", ErrorInternal, err)
	}
	defer res.Body.Close()

	b, err := ioutil.ReadAll(res.Body)

	if err != nil {
		log.With(zap.String("method", "ioutil.ReadAll")).Error(err)
		return fmt.Errorf("%w: %w", ErrorInternal, err)
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
			return fmt.Errorf("%w: %w", ErrorInternal, err)
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
func (v TokenValidator) Validate(
	ctx context.Context, apiKey string,
) (pb.RpcObjectImportNotionValidateTokenResponseErrorCode, error) {
	err := v.ping.Ping(ctx, apiKey)
	if errors.Is(err, ErrorInternal) {
		return pb.RpcObjectImportNotionValidateTokenResponseError_INTERNAL_ERROR, err
	}
	if errors.Is(err, ErrorUnauthorized) {
		return pb.RpcObjectImportNotionValidateTokenResponseError_UNAUTHORIZED, nil
	}
	if errors.Is(err, ErrorForbidden) {
		return pb.RpcObjectImportNotionValidateTokenResponseError_FORBIDDEN, nil
	}
	if errors.Is(err, ErrorNotionUnavailable) {
		return pb.RpcObjectImportNotionValidateTokenResponseError_SERVICE_UNAVAILABLE, nil
	}
	return pb.RpcObjectImportNotionValidateTokenResponseError_NULL, nil
}
