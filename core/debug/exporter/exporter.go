package exporter

import (
	"context"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/recordverifier"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"github.com/anyproto/any-sync/util/crypto"
	"golang.org/x/exp/slices"
)

type recordVerifier struct {
}

func (r recordVerifier) Init(a *app.App) (err error) {
	return nil
}

func (r recordVerifier) Name() (name string) {
	return recordverifier.CName
}

func (r recordVerifier) VerifyAcceptor(rec *consensusproto.RawRecord) (err error) {
	return nil
}

func (r recordVerifier) ShouldValidate() bool {
	return false
}

type DataConverter interface {
	Unmarshall(dataType string, decrypted []byte) (any, error)
	Marshall(model any) (data []byte, dataType string, err error)
}

func prepareExport(ctx context.Context, readable objecttree.ReadableObjectTree, store anystore.DB) (objecttree.ObjectTree, error) {
	headStorage, err := headstorage.New(ctx, store)
	if err != nil {
		return nil, err
	}
	acl := readable.AclList()
	root := acl.Root()
	listStorage, err := list.CreateStorage(ctx, root, headStorage, store)
	if err != nil {
		return nil, err
	}
	keys, err := accountdata.NewRandom()
	if err != nil {
		return nil, err
	}
	newAcl, err := list.BuildAclListWithIdentity(keys, listStorage, recordVerifier{})
	if err != nil {
		return nil, err
	}
	treeStorage, err := objecttree.CreateStorage(ctx, readable.Header(), headStorage, store)
	if err != nil {
		return nil, err
	}
	writeTree, err := objecttree.BuildTestableTree(treeStorage, newAcl)
	if err != nil {
		return nil, err
	}
	return writeTree, nil
}

type ExportParams struct {
	Readable  objecttree.ReadableObjectTree
	Store     anystore.DB
	Converter DataConverter
}

func ExportTree(ctx context.Context, params ExportParams) error {
	writeTree, err := prepareExport(ctx, params.Readable, params.Store)
	if err != nil {
		return err
	}
	writeTree.Lock()
	defer writeTree.Unlock()
	var (
		changeBuilder = objecttree.NewChangeBuilder(crypto.NewKeyStorage(), writeTree.Header())
		converter     = params.Converter
		changes       []*treechangeproto.RawTreeChangeWithId
	)
	err = params.Readable.IterateRoot(
		func(change *objecttree.Change, decrypted []byte) (any, error) {
			return converter.Unmarshall(change.DataType, decrypted)
		},
		func(change *objecttree.Change) bool {
			if change.Id == writeTree.Id() {
				return true
			}
			var (
				data     []byte
				dataType string
			)
			data, dataType, err = converter.Marshall(change.Model)
			if err != nil {
				return false
			}
			// that means that change is unencrypted
			change.ReadKeyId = ""
			change.Data = data
			change.DataType = dataType
			var raw *treechangeproto.RawTreeChangeWithId
			raw, err = changeBuilder.Marshall(change)
			if err != nil {
				return false
			}
			changes = append(changes, raw)
			return true
		})
	if err != nil {
		return err
	}
	payload := objecttree.RawChangesPayload{
		NewHeads:   writeTree.Heads(),
		RawChanges: changes,
	}
	res, err := writeTree.AddRawChanges(ctx, payload)
	if err != nil {
		return err
	}
	if !slices.Equal(res.Heads, writeTree.Heads()) {
		return fmt.Errorf("heads mismatch: %v != %v", res.Heads, writeTree.Heads())
	}
	return nil
}
