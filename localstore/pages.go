package localstore

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
)

var (
	// PageInfo is stored in db key pattern:
	pagesPrefix         = "pages"
	pagesDetailsBase    = ds.NewKey("/" + pagesPrefix + "/details")
	pagesSnippetBase    = ds.NewKey("/" + pagesPrefix + "/snippet")
	pagesLastStateBase  = ds.NewKey("/" + pagesPrefix + "/state")
	pagesLastOpenedBase = ds.NewKey("/" + pagesPrefix + "/lastopened")

	pagesInboundLinksBase  = ds.NewKey("/" + pagesPrefix + "/inbound")
	pagesOutboundLinksBase = ds.NewKey("/" + pagesPrefix + "/outbound")

	_ PageStore = (*dsPageStore)(nil)
)

type dsPageStore struct {
	ds ds.TxnDatastore
	l  sync.Mutex
}

func (m *dsPageStore) Add(page *model.PageInfoWithOutboundLinksIDs) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	detailsKey := pagesDetailsBase.ChildString(page.Id)
	snippetKey := pagesSnippetBase.ChildString(page.Id)
	outboundKey := pagesOutboundLinksBase.ChildString(page.Id)
	stateKey := pagesLastStateBase.ChildString(page.Id)

	exists, err := txn.Has(detailsKey)
	if err != nil {
		return err
	}
	if exists {
		return ErrDuplicateKey
	}

	b, err := proto.Marshal(page.Info.Details)
	if err != nil {
		return err
	}

	err = txn.Put(detailsKey, b)
	if err != nil {
		return err
	}

	for _, targetPageId := range page.OutboundLinks {
		err = txn.Put(outboundKey.ChildString(targetPageId), nil)
		if err != nil {
			return err
		}

		// add inbound link for the target page
		inboundKey := pagesInboundLinksBase.ChildString(targetPageId)
		err = txn.Put(inboundKey.ChildString(page.Id), nil)
		if err != nil {
			return err
		}
	}

	err = txn.Put(snippetKey, []byte(page.Info.Snippet))
	if err != nil {
		return err
	}

	b, err = proto.Marshal(page.State)
	if err != nil {
		return err
	}

	err = txn.Put(stateKey, b)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func getPageInfo(txn ds.Txn, id string) (*model.PageInfo, error) {
	val, err := txn.Get(pagesLastStateBase.ChildString(id))
	if err != nil {
		return nil, fmt.Errorf("failed to get last state: %w", err)
	}

	var state model.State
	err = proto.Unmarshal(val, &state)
	if err != nil {
		return nil, err
	}

	val, err = txn.Get(pagesDetailsBase.ChildString(id))
	if err != nil && err != ds.ErrNotFound {
		return nil, fmt.Errorf("failed to get details: %w", err)
	}

	var details model.PageDetails
	if val != nil {
		err = proto.Unmarshal(val, &details)
		if err != nil {
			return nil, err
		}
	}

	val, err = txn.Get(pagesSnippetBase.ChildString(id))
	if err != nil && err != ds.ErrNotFound {
		return nil, fmt.Errorf("failed to get snippet: %w", err)
	}

	lastOpened, err := getLastOpened(txn, id)
	if err != nil && err != ds.ErrNotFound {
		return nil, fmt.Errorf("failed to get last opened: %w", err)
	}

	inboundResults, err := txn.Query(query.Query{
		Prefix:   pagesInboundLinksBase.String() + "/" + id + "/",
		Limit:    1, // we only need to know if there is at least 1 inbound link
		KeysOnly: true,
	})

	// max is 1
	inboundLinks, err := CountAllKeysFromResults(inboundResults)

	val, err = txn.Get(pagesSnippetBase.ChildString(id))
	if err != nil && err != ds.ErrNotFound {
		return nil, fmt.Errorf("failed to get snippet: %w", err)
	}

	return &model.PageInfo{Id: id, Details: details.Details, Snippet: string(val), State: &state, LastOpened: lastOpened, HasInboundLinks: inboundLinks == 1}, nil
}

func getPagesInfo(txn ds.Txn, ids []string) ([]*model.PageInfo, error) {
	var pages []*model.PageInfo
	for _, id := range ids {
		var val *model.PageInfo
		val, err := getPageInfo(txn, id)
		if err != nil {
			if strings.HasSuffix(err.Error(), "key not found") {
				continue
			}

			return nil, err
		}

		pages = append(pages, val)
	}

	return pages, nil
}

func getOutboundLinks(txn ds.Txn, id string) ([]string, error) {
	outboundResults, err := txn.Query(query.Query{
		Prefix:   pagesOutboundLinksBase.String() + "/" + id + "/",
		Limit:    0,
		KeysOnly: true,
	})
	if err != nil {
		return nil, err
	}
	return GetAllKeysFromResults(outboundResults)
}

func (m *dsPageStore) GetWithLinksInfoByID(id string) (*model.PageInfoWithLinks, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	pages, err := getPagesInfo(txn, []string{id})
	if err != nil {
		return nil, err
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("page not found")
	}
	page := pages[0]

	inboundResults, err := txn.Query(query.Query{
		Prefix:   pagesInboundLinksBase.String() + "/" + id + "/",
		Limit:    0,
		KeysOnly: true,
	})
	if err != nil {
		return nil, err
	}

	inboundIds, err := GetAllKeysFromResults(inboundResults)
	if err != nil {
		return nil, err
	}

	outboundResults, err := txn.Query(query.Query{
		Prefix:   pagesOutboundLinksBase.String() + "/" + id + "/",
		Limit:    0,
		KeysOnly: true,
	})
	if err != nil {
		return nil, err
	}
	outboundsIds, err := GetAllKeysFromResults(outboundResults)
	if err != nil {
		return nil, err
	}

	inbound, err := getPagesInfo(txn, inboundIds)
	if err != nil {
		return nil, err
	}

	outbound, err := getPagesInfo(txn, outboundsIds)
	if err != nil {
		return nil, err
	}

	return &model.PageInfoWithLinks{
		Id:   id,
		Info: page,
		Links: &model.PageLinksInfo{
			Inbound:  inbound,
			Outbound: outbound,
		},
	}, nil
}

func (m *dsPageStore) GetWithOutboundLinksInfoById(id string) (*model.PageInfoWithOutboundLinks, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	pages, err := getPagesInfo(txn, []string{id})
	if err != nil {
		return nil, err
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("page not found")
	}
	page := pages[0]

	outboundsIds, err := getOutboundLinks(txn, id)
	if err != nil {
		return nil, err
	}

	outbound, err := getPagesInfo(txn, outboundsIds)
	if err != nil {
		return nil, err
	}

	return &model.PageInfoWithOutboundLinks{
		Info:          page,
		OutboundLinks: outbound,
	}, nil
}

func (m *dsPageStore) List() ([]*model.PageInfo, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()
	inboundResults, err := txn.Query(query.Query{
		Prefix:   pagesLastStateBase.String() + "/",
		Limit:    0,
		KeysOnly: true,
	})
	if err != nil {
		return nil, err
	}

	ids, err := GetAllKeysFromResults(inboundResults)
	if err != nil {
		return nil, err
	}

	return getPagesInfo(txn, ids)
}

func (m *dsPageStore) GetByIDs(ids ...string) ([]*model.PageInfo, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return getPagesInfo(txn, ids)
}

func (m *dsPageStore) GetStateByID(id string) (*model.State, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	val, err := txn.Get(pagesLastStateBase.ChildString(id))
	if err != nil {
		return nil, err
	}

	var state model.State
	err = proto.Unmarshal(val, &state)
	if err != nil {
		return nil, err
	}

	return &state, nil
}

func diffSlices(a, b []string) (removed []string, added []string) {
	var amap = map[string]struct{}{}
	var bmap = map[string]struct{}{}

	for _, item := range a {
		amap[item] = struct{}{}
	}

	for _, item := range b {
		if _, exists := amap[item]; !exists {
			added = append(added, item)
		}
		bmap[item] = struct{}{}
	}

	for _, item := range a {
		if _, exists := bmap[item]; !exists {
			removed = append(removed, item)
		}
	}
	return
}

func (m *dsPageStore) Update(id string, details *types.Struct, links []string, snippet *string) error {
	m.l.Lock()
	defer m.l.Unlock()

	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	if details != nil || snippet != nil {
		exInfo, _ := getPageInfo(txn, id)
		if exInfo != nil {
			if exInfo.Details.Equal(details) {
				details = nil
			}

			if snippet != nil && exInfo.Snippet == *snippet {
				snippet = nil
			}
		}
	}

	var addedLinks, removedLinks []string

	if links != nil {
		exLinks, _ := getOutboundLinks(txn, id)
		addedLinks, removedLinks = diffSlices(exLinks, links)
	}

	// underlying commands set the same state each time, but this shouldn't be a problem as we made it in 1 transaction
	if details != nil {
		err = m.updateDetails(txn, id, &model.PageDetails{Details: details})
		if err != nil {
			return err
		}
	}

	if len(addedLinks) > 0 {
		err = m.addLinks(txn, id, addedLinks)
		if err != nil {
			return err
		}
	}

	if len(removedLinks) > 0 {
		err = m.removeLinks(txn, id, removedLinks)
		if err != nil {
			return err
		}
	}

	if snippet != nil {
		err = m.updateSnippet(txn, id, *snippet)
		if err != nil {
			return err
		}
	}

	return txn.Commit()
}

func (m *dsPageStore) addLinks(txn ds.Txn, fromID string, targetIDs []string) error {
	for _, targetID := range targetIDs {
		outboundKey := pagesOutboundLinksBase.ChildString(fromID).ChildString(targetID)
		inboundKey := pagesInboundLinksBase.ChildString(targetID).ChildString(fromID)
		err := txn.Put(outboundKey, nil)
		if err != nil {
			return err
		}

		err = txn.Put(inboundKey, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *dsPageStore) removeLinks(txn ds.Txn, fromID string, targetIDs []string) error {
	for _, targetID := range targetIDs {
		outboundKey := pagesOutboundLinksBase.ChildString(fromID).ChildString(targetID)
		inboundKey := pagesInboundLinksBase.ChildString(targetID).ChildString(fromID)
		err := txn.Delete(outboundKey)
		if err != nil {
			return err
		}

		err = txn.Delete(inboundKey)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *dsPageStore) updateDetails(txn ds.Txn, id string, details *model.PageDetails) error {
	detailsKey := pagesDetailsBase.ChildString(id)
	b, err := proto.Marshal(details)
	if err != nil {
		return err
	}

	err = txn.Put(detailsKey, b)
	if err != nil {
		return err
	}

	return nil
}

func (m *dsPageStore) updateSnippet(txn ds.Txn, id string, snippet string) error {
	snippetKey := pagesSnippetBase.ChildString(id)

	err := txn.Put(snippetKey, []byte(snippet))
	if err != nil {
		return err
	}

	return nil
}

func (m *dsPageStore) Delete(id string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	detailsKey := pagesDetailsBase.ChildString(id)
	snippetKey := pagesSnippetBase.ChildString(id)
	outboundKey := pagesOutboundLinksBase.ChildString(id)
	inboundKey := pagesInboundLinksBase.ChildString(id)
	stateKey := pagesLastStateBase.ChildString(id)

	exists, err := txn.Has(stateKey)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}

	err = txn.Delete(detailsKey)
	if err != nil {
		return err
	}

	err = txn.Delete(snippetKey)
	if err != nil {
		return err
	}

	err = txn.Delete(outboundKey)
	if err != nil {
		return err
	}

	err = txn.Delete(stateKey)
	if err != nil {
		return err
	}

	inboundResults, err := txn.Query(query.Query{
		Prefix:   inboundKey.String(),
		Limit:    0,
		KeysOnly: true,
	})
	if err != nil {
		return err
	}

	inboundIds, err := GetAllKeysFromResults(inboundResults)
	if err != nil {
		return err
	}

	// remove indexed outbound links from the source pages
	// todo: we have ghost links left
	for _, inboundLinkPageId := range inboundIds {
		err = txn.Delete(pagesOutboundLinksBase.ChildString(inboundLinkPageId).ChildString(id))
		if err != nil {
			return err
		}
	}

	err = txn.Delete(inboundKey)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (m *dsPageStore) UpdateLastOpened(id string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	var b = make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(time.Now().Unix()))

	err = txn.Put(pagesLastOpenedBase.ChildString(id), b)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func getLastOpened(txn ds.Txn, id string) (int64, error) {
	b, err := txn.Get(pagesLastOpenedBase.ChildString(id))
	if err != nil {
		return 0, err
	}

	ts := binary.LittleEndian.Uint64(b)

	return int64(ts), nil
}

func NewPageStore(ds ds.TxnDatastore) PageStore {
	return &dsPageStore{
		ds: ds,
	}
}

func (m *dsPageStore) Prefix() string {
	return "pages"
}

func (m *dsPageStore) Indexes() []Index {
	return []Index{}
}
