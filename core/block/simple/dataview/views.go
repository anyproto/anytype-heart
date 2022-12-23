package dataview

import (
	"fmt"

	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

func (l *Dataview) AddFilter(viewId string, filter *model.BlockContentDataviewFilter) error {
	view, err := l.GetView(viewId)
	if err != nil {
		return err
	}

	if filter.Id == "" {
		filter.Id = bson.NewObjectId().Hex()
	}
	view.Filters = append(view.Filters, filter)
	return nil
}

func (l *Dataview) RemoveFilters(viewId string, filterIDs []string) error {
	view, err := l.GetView(viewId)
	if err != nil {
		return err
	}

	view.Filters = slice.Filter(view.Filters, func(f *model.BlockContentDataviewFilter) bool {
		return slice.FindPos(filterIDs, f.Id) == -1
	})
	return nil
}

func (l *Dataview) ReplaceFilter(viewId string, filterID string, filter *model.BlockContentDataviewFilter) error {
	view, err := l.GetView(viewId)
	if err != nil {
		return err
	}

	idx := slice.Find(view.Filters, func(f *model.BlockContentDataviewFilter) bool {
		return f.Id == filterID
	})
	if idx < 0 {
		return fmt.Errorf("filter with id %s is not found", filter.RelationKey)
	}

	filter.Id = filterID
	view.Filters[idx] = filter

	return nil
}

func (l *Dataview) ReorderFilters(viewId string, ids []string) error {
	view, err := l.GetView(viewId)
	if err != nil {
		return err
	}

	filtersMap := make(map[string]*model.BlockContentDataviewFilter)
	for _, f := range view.Filters {
		filtersMap[f.Id] = f
	}

	view.Filters = view.Filters[:0]
	for _, id := range ids {
		if f, ok := filtersMap[id]; ok {
			view.Filters = append(view.Filters, f)
		}
	}

	return nil
}

func (l *Dataview) AddSort(viewId string, sort *model.BlockContentDataviewSort) error {
	view, err := l.GetView(viewId)
	if err != nil {
		return err
	}

	view.Sorts = append(view.Sorts, sort)
	return nil
}

func (l *Dataview) RemoveSorts(viewId string, sortIDs []string) error {
	view, err := l.GetView(viewId)
	if err != nil {
		return err
	}

	view.Sorts = slice.Filter(view.Sorts, func(f *model.BlockContentDataviewSort) bool {
		return slice.FindPos(sortIDs, getViewSortID(f)) == -1
	})
	return nil
}

func (l *Dataview) ReplaceSort(viewId string, sortID string, sort *model.BlockContentDataviewSort) error {
	view, err := l.GetView(viewId)
	if err != nil {
		return err
	}

	idx := slice.Find(view.Sorts, func(f *model.BlockContentDataviewSort) bool {
		return getViewSortID(f) == sortID
	})
	if idx < 0 {
		return fmt.Errorf("sort with id %s is not found", sort.RelationKey)
	}

	view.Sorts[idx] = sort

	return nil
}

func (l *Dataview) ReorderSorts(viewId string, ids []string) error {
	view, err := l.GetView(viewId)
	if err != nil {
		return err
	}

	sortsMap := make(map[string]*model.BlockContentDataviewSort)
	for _, f := range view.Sorts {
		sortsMap[getViewSortID(f)] = f
	}

	view.Sorts = view.Sorts[:0]
	for _, id := range ids {
		if f, ok := sortsMap[id]; ok {
			view.Sorts = append(view.Sorts, f)
		}
	}
	return nil
}
