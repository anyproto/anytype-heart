package localstore

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core/smartblock"
	"github.com/anytypeio/go-anytype-library/database"
	"github.com/anytypeio/go-anytype-library/pb"
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

func (m *dsPageStore) Query(q database.Query) ([]database.Entry, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	dsq := q.DSQuery()
	dsq.Prefix = pagesDetailsBase.String() + "/"
	dsq.Filters = append([]query.Filter{&filterPagesOnly{}}, dsq.Filters...)
	res, err := txn.Query(dsq)
	if err != nil {
		return nil, fmt.Errorf("error when querying ds: %w", err)
	}

	entries, err := res.Rest()
	if err != nil {
		return nil, fmt.Errorf("error when getting q results: %w", err)
	}

	var results []database.Entry

	for _, entry := range entries {
		var details model.PageDetails
		err = proto.Unmarshal(entry.Value, &details)
		if err != nil {
			log.Errorf("failed to unmarshal: %s", err.Error())
			continue
		}

		key := ds.NewKey(entry.Key)
		keyList := key.List()
		id := keyList[len(keyList)-1]
		lastOpenedTS, _ := getLastOpened(txn, id)
		if details.Details == nil || details.Details.Fields == nil {
			details.Details = &types.Struct{Fields: map[string]*types.Value{}}
		}

		details.Details.Fields["lastOpened"] = pb.ToValue(lastOpenedTS)
		details.Details.Fields["id"] = pb.ToValue(id)

		results = append(results, database.Entry{Details: details.Details})
	}

	return results, nil
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

	outbound, err := getPagesInfo(txn, outboundsIds)
	if err != nil {
		return nil, err
	}

	return &model.PageInfoWithOutboundLinks{
		Info:          page,
		OutboundLinks: outbound,
	}, nil
}

func (m *dsPageStore) GetByIDs(ids ...string) ([]*model.PageInfo, error) {
	txn, err := m.ds.NewTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	pages, err := getPagesInfo(txn, ids)
	if err != nil {
		return nil, err
	}

	return pages, nil
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

func (m *dsPageStore) Update(state *model.State, id string, addedLinks []string, removedLinks []string, changeSnippet string, changedDetails *model.PageDetails) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	// underlying commands set the same state each time, but this shouldn't be a problem as we made it in 1 transaction
	if changedDetails != nil {
		err = m.updateDetails(txn, id, changedDetails)
		if err != nil {
			return err
		}
	}

	if addedLinks != nil {
		err = m.addLinks(txn, id, addedLinks)
		if err != nil {
			return err
		}
	}

	if removedLinks != nil {
		err = m.removeLinks(txn, id, removedLinks)
		if err != nil {
			return err
		}
	}

	if changeSnippet != "" {
		err = m.updateSnippet(txn, id, changeSnippet)
		if err != nil {
			return err
		}
	}

	err = m.updateState(txn, id, state)
	if err != nil {
		return err
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

func (m *dsPageStore) AddLinks(state *model.State, fromID string, targetIDs []string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	err = m.addLinks(txn, fromID, targetIDs)
	if err != nil {
		return err
	}

	err = m.updateState(txn, fromID, state)
	if err != nil {
		return err
	}

	return txn.Commit()
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

func (m *dsPageStore) RemoveLinks(state *model.State, fromID string, targetIDs []string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	err = m.removeLinks(txn, fromID, targetIDs)
	if err != nil {
		return err
	}

	err = m.updateState(txn, fromID, state)
	if err != nil {
		return err
	}

	return txn.Commit()
}

func (m *dsPageStore) updateState(txn ds.Txn, id string, state *model.State) error {
	stateKey := pagesLastStateBase.ChildString(id)

	b, err := proto.Marshal(state)
	if err != nil {
		return err
	}

	return txn.Put(stateKey, b)
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

func (m *dsPageStore) UpdateDetails(state *model.State, id string, details *model.PageDetails) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	err = m.updateDetails(txn, id, details)
	if err != nil {
		return err
	}

	err = m.updateState(txn, id, state)
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

func (m *dsPageStore) UpdateSnippet(state *model.State, id string, snippet string) error {
	txn, err := m.ds.NewTransaction(false)
	if err != nil {
		return fmt.Errorf("error when creating txn in datastore: %w", err)
	}
	defer txn.Discard()

	err = m.updateSnippet(txn, id, snippet)
	if err != nil {
		return err
	}

	err = m.updateState(txn, id, state)
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
