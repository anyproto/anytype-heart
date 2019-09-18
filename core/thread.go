package core

import (
	"errors"
	"fmt"

	logging "github.com/ipfs/go-log"
	"github.com/mr-tron/base58"
	tcore "github.com/textileio/go-textile/core"
	"github.com/textileio/go-textile/crypto"
	m "github.com/textileio/go-textile/mill"
	tpb "github.com/textileio/go-textile/pb"
)

var log = logging.Logger("tex-core")

func readFile(t *tcore.Textile, file *tpb.FileIndex) ([]byte, error) {
	if file == nil {
		return nil, errors.New("fileIndex is nil")
	}

	data, err := t.DataAtPath(file.Hash)
	if err != nil {
		return nil, fmt.Errorf("DataAtPath error: %s", err.Error())
	}

	if file.Key == "" {
		return data, nil
	}

	keyb, err := base58.Decode(file.Key)
	if err != nil {
		return nil, fmt.Errorf("key decode error: %s", err.Error())
	}

	plain, err := crypto.DecryptAES(data, keyb)
	if err != nil {
		return nil, fmt.Errorf("decryption error: %s", err.Error())
	}

	return plain, nil
}

func writeJSON(t *tcore.Textile, plaintext []byte) (*tpb.FileIndex, error) {
	mill := &m.Json{}

	conf := tcore.AddFileConfig{
		Media:     "application/json",
		Plaintext: false,
		Input:     plaintext,
		//Gzip:      true,
	}

	added, err := t.AddFileIndex(mill, conf)
	if err != nil {
		return nil, fmt.Errorf("AddFileIndex error: %s", err.Error())
	}

	return added, nil
}
