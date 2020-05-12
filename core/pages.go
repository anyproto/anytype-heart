package core

import (
	"sort"

	"github.com/anytypeio/go-anytype-library/pb/model"
)

func (a *Anytype) PageInfoWithLinks(id string) (*model.PageInfoWithLinks, error) {
	return a.localStore.Pages.GetWithLinksInfoByID(id)
}

func (a *Anytype) PageList() ([]*model.PageInfo, error) {
	ids, err := a.t.Logstore().Threads()
	if err != nil {
		return nil, err
	}

	var idsS []string
	for _, id := range ids {
		t, _ := SmartBlockTypeFromThreadID(id)
		if t != SmartBlockTypePage {
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

func (a *Anytype) PageUpdateLastOpened(id string) error {
	return a.localStore.Pages.UpdateLastOpened(id)
}
