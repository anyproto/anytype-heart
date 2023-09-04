package api

import "sync"

type PageTree struct {
	ParentPageToChildIDs   map[string][]string
	pageToChildIDsMapMutex sync.RWMutex
}

func NewPageTree() *PageTree {
	return &PageTree{ParentPageToChildIDs: make(map[string][]string, 0)}
}

func (pt *PageTree) Get(parentID string) ([]string, bool) {
	pt.pageToChildIDsMapMutex.RLock()
	defer pt.pageToChildIDsMapMutex.RUnlock()
	childIDs, ok := pt.ParentPageToChildIDs[parentID]
	return childIDs, ok
}

func (pt *PageTree) Set(parentID string, childIDs []string) {
	pt.pageToChildIDsMapMutex.Lock()
	defer pt.pageToChildIDsMapMutex.Unlock()
	pt.ParentPageToChildIDs[parentID] = childIDs
}

type BlockToPage struct {
	ParentBlockToPage         map[string]string
	parentBlockToPageMapMutex sync.RWMutex
}

func NewBlockToPage() *BlockToPage {
	return &BlockToPage{ParentBlockToPage: make(map[string]string, 0)}
}

func (bp *BlockToPage) Set(parentBlockID string, pageID string) {
	bp.parentBlockToPageMapMutex.Lock()
	defer bp.parentBlockToPageMapMutex.Unlock()
	bp.ParentBlockToPage[parentBlockID] = pageID
}

// NotionImportContext is a data object with all necessary structures for blocks
type NotionImportContext struct {
	// Need all these maps for correct mapping of pages and databases from notion to anytype
	// for such blocks as mentions or links to objects
	NotionPageIdsToAnytype     map[string]string
	NotionDatabaseIdsToAnytype map[string]string
	PageNameToID               map[string]string
	DatabaseNameToID           map[string]string
	PageTree                   *PageTree
	BlockToPage                *BlockToPage
}

func NewNotionImportContext() *NotionImportContext {
	return &NotionImportContext{
		NotionPageIdsToAnytype:     make(map[string]string, 0),
		NotionDatabaseIdsToAnytype: make(map[string]string, 0),
		PageNameToID:               make(map[string]string, 0),
		DatabaseNameToID:           make(map[string]string, 0),
		PageTree:                   NewPageTree(),
		BlockToPage:                NewBlockToPage(),
	}
}
