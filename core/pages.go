package core

import (
	"sort"
	"time"

	"github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/structs"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
)

func (a *Anytype) PageInfoWithLinks(id string) (*model.PageInfoWithLinks, error) {
	return a.localStore.Pages.GetWithLinksInfoByID(id)
}

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
		var lastOpenedI, lastOpenedJ int64

		if pages[i].Details != nil {
			if pages[i].Details.Fields["lastOpened"] != nil {
				lastOpenedI = int64(pages[i].Details.Fields["lastOpened"].GetNumberValue())
			}
		}

		if pages[j].Details != nil {
			if pages[j].Details.Fields["lastOpened"] != nil {
				lastOpenedJ = int64(pages[j].Details.Fields["lastOpened"].GetNumberValue())
			}
		}

		if lastOpenedI > lastOpenedJ {
			return true
		}

		if lastOpenedI < lastOpenedJ {
			return false
		}

		return pages[i].Id < pages[j].Id
	})

	return pages, nil
}

func (a *Anytype) PageUpdateLastOpened(id string) error {
	// lock here for the concurrent details changes
	a.lock.Lock()
	defer a.lock.Unlock()

	details, err := a.localStore.Pages.GetDetails(id)
	if err != nil {
		if err == ds.ErrNotFound {
			details = &model.PageDetails{Details: &types.Struct{Fields: make(map[string]*types.Value)}}
		} else {
			return err
		}
	}

	details.Details.Fields["lastOpened"] = structs.Float64(float64(time.Now().Unix()))

	return a.localStore.Pages.UpdateDetails(nil, id, details)
}
