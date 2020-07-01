package core

import (
	"sort"

	"github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-library/localstore"
	"github.com/anytypeio/go-anytype-library/pb/model"
)

func (a *Anytype) PageStore() localstore.PageStore {
	return a.localStore.Pages
}

// deprecated, to be removed
func (a *Anytype) PageInfoWithLinks(id string) (*model.PageInfoWithLinks, error) {
	return a.localStore.Pages.GetWithLinksInfoByID(id)
}

// deprecated, to be removed
func (a *Anytype) PageList() ([]*model.PageInfo, error) {
	ids, err := a.t.Logstore().Threads()
	if err != nil {
		return nil, err
	}

	var idsS = make([]string, 0, len(ids))
	for _, id := range ids {
		t, _ := smartblock.SmartBlockTypeFromThreadID(id)
		if t != smartblock.SmartBlockTypePage {
			continue
		}

		idsS = append(idsS, id.String())
	}

	pages, err := a.localStore.Pages.GetByIDs(idsS...)
	if err != nil {
		return nil, err
	}

	sort.Slice(pages, func(i, j int) bool {
		// show pages with inbound links first
		if pages[i].HasInboundLinks && !pages[j].HasInboundLinks {
			return true
		}

		if !pages[i].HasInboundLinks && pages[j].HasInboundLinks {
			return false
		}

		// then sort by Last Opened date
		if pages[i].LastOpened > pages[j].LastOpened {
			return true
		}

		if pages[i].LastOpened < pages[j].LastOpened {
			return false
		}

		return pages[i].Id < pages[j].Id
	})

	return pages, nil
}

// deprecated, to be removed
func (a *Anytype) PageUpdateLastOpened(id string) error {
	return a.localStore.Pages.UpdateLastOpened(id)
}
