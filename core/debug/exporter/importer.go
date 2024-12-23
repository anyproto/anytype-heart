package exporter

import (
	"context"
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
	Tree       objecttree.ReadableObjectTree
	FolderPath string
}

type ImportParams struct {
	FullTree bool
	BeforeId string
}

func (i ImportParams) Heads() []string {
	if i.FullTree {
		return nil
	}
	return []string{i.BeforeId}
}

func ImportTree(ctx context.Context, path string, params ImportParams) (res ImportResult, err error) {
	targetDir := strings.TrimSuffix(path, filepath.Ext(path))
	if err = ziputil.UnzipFolder(path, targetDir); err != nil {
		return
	}
	anyStore, err := anystore.Open(ctx, targetDir, nil)
	if err != nil {
		return
	}
	defer anyStore.Close()
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
	acl, err := list.BuildAclListWithIdentity(randomKeys, listStorage, list.NoOpAcceptorVerifier{})
	if err != nil {
		return
	}
	treeStorage, err := objecttree.NewStorage(ctx, treeId, headStorage, anyStore)
	if err != nil {
		return
	}
	objectTree, err := objecttree.BuildNonVerifiableHistoryTree(objecttree.HistoryTreeParams{
		Storage:         treeStorage,
		AclList:         acl,
		Heads:           params.Heads(),
		IncludeBeforeId: true,
	})
	if err != nil {
		return
	}
	return ImportResult{
		Tree:       objectTree,
		FolderPath: targetDir,
	}, nil
}
