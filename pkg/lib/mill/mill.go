package mill

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mr-tron/base58/base58"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("tex-mill")

var ErrMediaTypeNotSupported = fmt.Errorf("media type not supported")

type Result struct {
	File io.Reader
	Meta map[string]interface{}
}

type Mill interface {
	ID() string
	Pin() bool // pin by default
	AcceptMedia(media string) error
	Options(add map[string]interface{}) (string, error)
	Mill(r io.ReadSeeker, name string) (*Result, error)
}

func accepts(list []string, media string) error {
	for _, m := range list {
		if media == m {
			return nil
		}
	}
	return ErrMediaTypeNotSupported
}

func hashOpts(opts interface{}, add map[string]interface{}) (string, error) {
	optsd, err := json.Marshal(opts)
	if err != nil {
		return "", err
	}
	var final map[string]interface{}
	if err := json.Unmarshal(optsd, &final); err != nil {
		return "", err
	}
	for k, v := range add {
		final[k] = v
	}
	data, err := json.Marshal(final)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	return base58.FastBase58Encoding(sum[:]), nil
}
