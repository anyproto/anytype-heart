package api

import "sync"

// NotionImportContext is a data object with all necessary structures for blocks
type NotionImportContext struct {
	Blocks []interface{}
	// Need all these maps for correct mapping of pages and databases from notion to anytype
	// for such blocks as mentions or links to objects
	NotionPageIdsToAnytype     map[string]string
	NotionDatabaseIdsToAnytype map[string]string
	PageNameToID               map[string]string
	DatabaseNameToID           map[string]string
	ParentPageToChildIDs       map[string][]string
	pageToChildIDsMapMutex     sync.RWMutex
	ParentBlockToPage          map[string]string
	parentBlockToPageMapMutex  sync.RWMutex
}

func NewNotionImportContext() *NotionImportContext {
	return &NotionImportContext{
		NotionPageIdsToAnytype:     make(map[string]string, 0),
		NotionDatabaseIdsToAnytype: make(map[string]string, 0),
		PageNameToID:               make(map[string]string, 0),
		DatabaseNameToID:           make(map[string]string, 0),
		ParentPageToChildIDs:       make(map[string][]string, 0),
		ParentBlockToPage:          make(map[string]string, 0),
	}
}

func (n *NotionImportContext) ReadParentPageToChildIDsMap(parentID string) ([]string, bool) {
	n.pageToChildIDsMapMutex.RLock()
	defer n.pageToChildIDsMapMutex.RUnlock()
	childIDs, ok := n.ParentPageToChildIDs[parentID]
	return childIDs, ok
}

func (n *NotionImportContext) WriteToParentPageToChildIDsMap(parentID string, childIDs []string) {
	n.pageToChildIDsMapMutex.Lock()
	defer n.pageToChildIDsMapMutex.Unlock()
	n.ParentPageToChildIDs[parentID] = childIDs
}

func (n *NotionImportContext) WriteToParentBlockToPageMap(parentBlockID string, pageID string) {
	n.parentBlockToPageMapMutex.Lock()
	defer n.parentBlockToPageMapMutex.Unlock()
	n.ParentBlockToPage[parentBlockID] = pageID
}
