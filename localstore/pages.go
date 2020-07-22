package localstore

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-library/database"
	"github.com/anytypeio/go-anytype-library/pb"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/structs"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
)

var (
	// PageInfo is stored in db key pattern:
	pagesPrefix           = "pages"
	pagesDetailsBase      = ds.NewKey("/" + pagesPrefix + "/details")
	pagesSnippetBase      = ds.NewKey("/" + pagesPrefix + "/snippet")
	pagesLastOpenedBase   = ds.NewKey("/" + pagesPrefix + "/lastopened")   // deprecated
	pagesLastModifiedBase = ds.NewKey("/" + pagesPrefix + "/lastmodified") // deprecated

	pagesInboundLinksBase  = ds.NewKey("/" + pagesPrefix + "/inbound")
	pagesOutboundLinksBase = ds.NewKey("/" + pagesPrefix + "/outbound")

	_ PageStore = (*dsPageStore)(nil)
)

type dsPageStore struct {
	ds ds.TxnDatastore
	l  sync.Mutex
}

type filterPagesOnly struct{}

func (m *filterPagesOnly) Filter(e query.Entry) bool {
	keyParts := strings.Split(e.Key, "/")
	id := keyParts[len(keyParts)-1]

	t, err := smartblock.SmartBlockTypeFromID(id)
	if err != nil {
		log.Errorf("failed to detect smartblock type for %s: %s", id, err.Error())
		return false
	}

	if t == smartblock.SmartBlockTypePage || t == smartblock.SmartBlockTypeProfilePage {
		return true
	}

	return false
}

func (m *dsPageStore) Schema() string {
	return "https://anytype.io/schemas/page"
}

func (m *dsPageStore) Query(q database.Query) (entries []database.Entry, total int, err error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, 0, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	dsq := q.DSQuery(m.Schema())
	dsq.Offset = 0
	dsq.Limit = 0
	dsq.Prefix = pagesDetailsBase.String() + "/"
	dsq.Filters = append([]query.Filter{&filterPagesOnly{}}, dsq.Filters...)
	res, err := txn.Query(dsq)
	if err != nil {
		return nil, 0, fmt.Errorf("error when querying ds: %w", err)
	}

	var results []database.Entry

	offset := q.Offset
	for entry := range res.Next() {
		if offset > 0 {
			offset--
			total++
			continue
		}

		if q.Limit > 0 && len(results) >= q.Limit {
			total++
			continue
		}

		var details model.PageDetails
		err = proto.Unmarshal(entry.Value, &details)
		if err != nil {
			log.Errorf("failed to unmarshal: %s", err.Error())
			continue
		}

		key := ds.NewKey(entry.Key)
		keyList := key.List()
		id := keyList[len(keyList)-1]

		if details.Details == nil || details.Details.Fields == nil {
			details.Details = &types.Struct{Fields: map[string]*types.Value{}}
		}

		details.Details.Fields["id"] = pb.ToValue(id)

		results = append(results, database.Entry{Details: details.Details})
		total++
	}

	return results, total, nil
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

	return txn.Commit()
}

func getDetails(txn ds.Txn, id string) (*model.PageDetails, error) {
	val, err := txn.Get(pagesDetailsBase.ChildString(id))
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
	return &details, nil
}

func getPageInfo(txn ds.Txn, id string) (*model.PageInfo, error) {
	var page = &model.PageInfo{Id: id}
	details, err := getDetails(txn, id)
	if err != nil && err != ds.ErrNotFound {
		return nil, fmt.Errorf("failed to get details: %w", err)
	} else if details != nil {
		page.Details = details.Details
	}

	val, err := txn.Get(pagesSnippetBase.ChildString(id))
	if err != nil && err != ds.ErrNotFound {
		return nil, fmt.Errorf("failed to get snippet: %w", err)
	} else if val != nil {
		page.Snippet = string(val)
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
	page.HasInboundLinks = inboundLinks == 1

	return page, nil
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
		Prefix:   pagesDetailsBase.String() + "/",
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
		removedLinks, addedLinks = diffSlices(exLinks, links)
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

func (m *dsPageStore) UpdateDetails(id string, details *model.PageDetails) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	err = m.updateDetails(txn, id, details)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (m *dsPageStore) updateSnippet(txn ds.Txn, id string, snippet string) error {
	snippetKey := pagesSnippetBase.ChildString(id)

	err := txn.Put(snippetKey, []byte(snippet))
	if err != nil {
		return err
	}

	return nil
}

func (m *dsPageStore) UpdateLastOpened(id string, time time.Time) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	details, err := getDetails(txn, id)
	if err != nil && err != ds.ErrNotFound {
		return err
	}

	if details == nil || details.Details == nil || details.Details.Fields == nil {
		details = &model.PageDetails{Details: &types.Struct{Fields: make(map[string]*types.Value)}}
	}

	details.Details.Fields["lastOpened"] = structs.Float64(float64(time.Unix()))

	err = m.updateDetails(txn, id, details)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (m *dsPageStore) UpdateLastModified(id string, time time.Time) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	details, err := getDetails(txn, id)
	if err != nil && err != ds.ErrNotFound {
		return err
	}

	if details == nil || details.Details == nil || details.Details.Fields == nil {
		details = &model.PageDetails{Details: &types.Struct{Fields: make(map[string]*types.Value)}}
	}

	details.Details.Fields["lastModified"] = structs.Float64(float64(time.Unix()))

	err = m.updateDetails(txn, id, details)
	if err != nil {
		return err
	}

	return txn.Commit()
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

	exists, err := txn.Has(detailsKey)
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

func (m *dsPageStore) GetDetails(id string) (*model.PageDetails, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	return getDetails(txn, id)
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
