package files

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema"
	"github.com/anyproto/anytype-heart/util/encode"
	uio "github.com/ipfs/boxo/ipld/unixfs/io"
	ipld "github.com/ipfs/go-ipld-format"
)

type PublishKeyObject struct {
	Cid string `json:"cid"`
	Key string `json:"key"`
}

type PublishResult struct {
	Cid string
	Key string
}

func (s *service) addToCommonAndUioDir(ctx context.Context, dagService ipld.DAGService, outer uio.Directory, fileName string, content []byte) (string, error) {
	node, err := s.commonFile.AddFile(ctx, bytes.NewReader(content))
	if err != nil {
		return "", fmt.Errorf("common add file: %w", err)
	}

	err = dagService.Add(ctx, node)
	if err != nil {
		return "", fmt.Errorf("dagService add file: %w", err)
	}

	cid := node.Cid().String()
	err = helpers.AddLinkToDirectory(ctx, dagService, outer, fileName, cid)
	if err != nil {
		return "", fmt.Errorf("add link to dir: %w", err)
	}

	return cid, nil
}
func (s *service) PublishingAdd(ctx context.Context, spaceId string, files []FileWithName) (*AddResult, error) {
	dagService := s.dagServiceForSpace(spaceId)
	outer := uio.NewDirectory(dagService)
	outer.SetCidBuilder(cidBuilder)

	mainKey, err := crypto.NewRandomAES()
	if err != nil {
		return nil, err
	}
	// will be converted to json and encrypted by main key
	keys := make(map[string]PublishKeyObject, 0)

	// add all files via common file, to outer ipfs dir and to keys
	for _, file := range files {
		key, err := crypto.NewRandomAES()
		if err != nil {
			return nil, fmt.Errorf("random key: %w", err)
		}

		encContent, err := key.Encrypt(file.Data)
		if err != nil {
			return nil, fmt.Errorf("encrypt file: %w", err)
		}

		cid, err := s.addToCommonAndUioDir(ctx, dagService, outer, file.Name, encContent)
		if err != nil {
			return nil, fmt.Errorf("addToCommonAndUioDir: %w", err)
		}

		var keyStr string
		keyStr, err = encode.EncodeKeyToBase58(key)
		if err != nil {
			return nil, fmt.Errorf("encode keystr: %w", err)
		}

		keys[file.Name] = PublishKeyObject{
			Cid: cid,
			Key: keyStr,
		}
	}

	// now, add keys to files and encrypt with the main key which will be returned
	keysJson, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("keys.json: %w", err)
	}

	encKeys, err := mainKey.Encrypt(keysJson)
	if err != nil {
		return nil, fmt.Errorf("main key encrypt: %w", err)
	}

	_, err = s.addToCommonAndUioDir(ctx, dagService, outer, "keys.json", encKeys)
	if err != nil {
		return nil, fmt.Errorf("addToCommonAndUioDir: %w", err)
	}

	rootPath := filepath.Join("objects", pageId+".pb")
	_, err = s.addToCommonAndUioDir(ctx, dagService, outer, "rootPath", []byte(rootPath))
	if err != nil {
		return nil, fmt.Errorf("addToCommonAndUioDir: %w", err)
	}

	mainKeyStr, err := encode.EncodeKeyToBase58(mainKey)
	if err != nil {
		return nil, fmt.Errorf("main key encrypt: %w", err)
	}

	outerNode, err := outer.GetNode()
	if err != nil {
		return nil, fmt.Errorf("outer get node: %w", err)
	}

	err = dagService.Add(ctx, outerNode)
	if err != nil {
		return nil, fmt.Errorf("dagService add: %w", err)
	}

	outerNodeCid := outerNode.Cid().String()

	// and return node Cid and mainKey
	fileId := domain.FileId(outerNodeCid)
	encryptionKeys := &domain.FileEncryptionKeys{
		FileId:         fileId,
		EncryptionKeys: map[string]string{schema.LinkFile: mainKeyStr},
	}

	addResult := &AddResult{
		FileId:         fileId,
		EncryptionKeys: encryptionKeys,
	}

	return addResult, nil

}
