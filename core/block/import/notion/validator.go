package notion

import (
	"context"
	"errors"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/ping"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type TokenValidator struct {
	ping *ping.Service
}

func NewTokenValidator() *TokenValidator {
	cl := client.NewClient()
	return &TokenValidator{
		ping: ping.New(cl),
	}
}

// Validate calls Notion API with given api key and check, if error is unauthorized
func (v TokenValidator) Validate(ctx context.Context,
	apiKey string) pb.RpcObjectImportNotionTokenValidateResponseErrorCode {
	err := v.ping.Ping(ctx, apiKey)
	if errors.Is(err, ping.ErrorInternal) {
		return pb.RpcObjectImportNotionTokenValidateResponseError_INTERNAL_ERROR
	}
	if errors.Is(err, ping.ErrorUnauthorized) {
		return pb.RpcObjectImportNotionTokenValidateResponseError_UNAUTHORIZED
	}
	return pb.RpcObjectImportNotionTokenValidateResponseError_NULL
}
