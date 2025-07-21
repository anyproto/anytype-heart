package exporter

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"

	"github.com/anyproto/anytype-heart/util/ziputil"
)

type ImportResult struct {
	List       list.AclList
	Storage    objecttree.Storage
	FolderPath string
	Store      anystore.DB
}

func (i ImportResult) CreateReadableTree(fullTree bool, beforeId string) (objecttree.ReadableObjectTree, error) {
	return objecttree.BuildNonVerifiableHistoryTree(objecttree.HistoryTreeParams{
		Storage:         i.Storage,
		AclList:         i.List,
		Heads:           i.Heads(fullTree, beforeId),
		IncludeBeforeId: true,
	})
}

func (i ImportResult) Heads(fullTree bool, beforeId string) []string {
	if fullTree {
		return nil
	}
	return []string{beforeId}
}

func ImportStorage(ctx context.Context, path string) (res ImportResult, err error) {
	targetDir := strings.TrimSuffix(path, filepath.Ext(path))
	if _, err = os.Stat(targetDir); err == nil {
		err = os.RemoveAll(targetDir)
		if err != nil {
			return
		}
	}
	if err = ziputil.UnzipFolder(path, targetDir); err != nil {
		return
	}
	anyStore, err := anystore.Open(ctx, filepath.Join(targetDir, "db"), nil)
	if err != nil {
		return
	}
	var (
		aclId  string
		treeId string
	)
	headStorage, err := headstorage.New(ctx, anyStore)
	if err != nil {
		return
	}
	err = headStorage.IterateEntries(ctx, headstorage.IterOpts{}, func(entry headstorage.HeadsEntry) (bool, error) {
		if entry.CommonSnapshot == "" {
			aclId = entry.Id
			return true, nil
		}
		treeId = entry.Id
		return true, nil
	})
	if err != nil {
		return
	}
	listStorage, err := list.NewStorage(ctx, aclId, headStorage, anyStore)
	if err != nil {
		return
	}
	randomKeys, err := accountdata.NewRandom()
	if err != nil {
		return
	}
	acl, err := list.BuildAclListWithIdentity(randomKeys, listStorage, recordVerifier{})
	if err != nil {
		return
	}
	treeStorage, err := objecttree.NewStorage(ctx, treeId, headStorage, anyStore)
	if err != nil {
		return
	}
	return ImportResult{
		List:       acl,
		Storage:    treeStorage,
		FolderPath: targetDir,
		Store:      anyStore,
	}, nil
}
