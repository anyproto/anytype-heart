package objectstore

import (
	"errors"
	"strings"

	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

const (
	blockViewSeparator = ":"
	viewsSeparator     = ","
)

var ErrParseView = errors.New("failed to parse view")

// SetActiveViews accepts map of active views by blocks, as objects can handle multiple dataview blocks
func (s *dsObjectStore) SetActiveViews(objectId string, views map[string]string) error {
	return badgerhelper.SetValue(s.db, pagesActiveViewBase.ChildString(objectId).Bytes(), viewsMapToString(views))
}

func (s *dsObjectStore) SetActiveView(objectId, blockId, viewId string) error {
	views, err := s.GetActiveViews(objectId)
	// if active views are not found in BD, or we could not parse them, then we need to rewrite them
	if err != nil && !badgerhelper.IsNotFound(err) && !errors.Is(err, ErrParseView) {
		return err
	}
	if views == nil {
		views = make(map[string]string, 1)
	}
	views[blockId] = viewId
	return s.SetActiveViews(objectId, views)
}

// GetActiveViews returns a map of activeViews by block ids
func (s *dsObjectStore) GetActiveViews(objectId string) (views map[string]string, err error) {
	raw, err := badgerhelper.GetValue(s.db, pagesActiveViewBase.ChildString(objectId).Bytes(), bytesToString)
	if err != nil {
		return nil, err
	}
	return parseViewsMap(raw)
}

func viewsMapToString(views map[string]string) (result string) {
	for block, view := range views {
		result = result + viewsSeparator + block + blockViewSeparator + view
	}
	if len(views) != 0 {
		result = result[1:]
	}
	return result
}

func parseViewsMap(s string) (viewsMap map[string]string, err error) {
	viewsMap = make(map[string]string)
	views := strings.Split(s, viewsSeparator)
	for _, view := range views {
		parts := strings.Split(view, blockViewSeparator)
		if len(parts) != 2 {
			return nil, ErrParseView
		}
		viewsMap[parts[0]] = parts[1]
	}
	return viewsMap, nil
}
