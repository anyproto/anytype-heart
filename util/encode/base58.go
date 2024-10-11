package encode

import (
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/mr-tron/base58/base58"
)

func EncodeKeyToBase58(key crypto.SymKey) (string, error) {
	raw, err := key.Raw()
	if err != nil {
		return "", err
	}
	return base58.Encode(raw), nil
}

func DecodeKeyFromBase58(rawString string) (crypto.SymKey, error) {
	raw, err := base58.Decode(rawString)
	if err != nil {
		return nil, err
	}
	return crypto.UnmarshallAESKey(raw)
}
