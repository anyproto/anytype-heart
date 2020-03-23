package mill

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	logging "github.com/ipfs/go-log"
	"github.com/mr-tron/base58/base58"
)

var log = logging.Logger("tex-mill")

var ErrMediaTypeNotSupported = fmt.Errorf("media type not supported")

type Result struct {
	File []byte
	Meta map[string]interface{}
}

type Mill interface {
	ID() string
	Encrypt() bool // encryption allowed
	Pin() bool     // pin by default
	AcceptMedia(media string) error
	Options(add map[string]interface{}) (string, error)
	Mill(input []byte, name string) (*Result, error)
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
