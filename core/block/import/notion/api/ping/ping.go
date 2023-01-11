package ping

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api/client"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var (
	ErrorInternal     = errors.New("internal")
	ErrorUnauthorized = errors.New("unauthorized")
)

var logger = logging.Logger("notion-ping")

const (
	endpoint = "/users?page_size=1"
)

type Service struct {
	client *client.Client
}

// New is a constructor for Service
func New(client *client.Client) *Service {
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
		err = client.TransformHTTPCodeToError(b)
		if err != nil {
			return errors.Wrap(ErrorInternal, err.Error())
		}
	}
	return nil
}
