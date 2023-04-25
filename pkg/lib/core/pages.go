package core

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

// deprecated, to be removed
func (a *Anytype) ObjectInfoWithLinks(id string) (*model.ObjectInfoWithLinks, error) {
	return a.objectStore.GetWithLinksInfoByID(id)
}
