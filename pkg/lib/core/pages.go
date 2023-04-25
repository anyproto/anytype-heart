package core

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

// Deprecated, use filestore component directly
func (a *Anytype) FileStore() filestore.FileStore {
	return a.fileStore
}

// deprecated, to be removed
func (a *Anytype) ObjectInfoWithLinks(id string) (*model.ObjectInfoWithLinks, error) {
	return a.objectStore.GetWithLinksInfoByID(id)
}
