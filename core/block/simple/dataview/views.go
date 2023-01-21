package dataview

import (
	"fmt"

	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

func (l *Dataview) AddFilter(viewID string, filter *model.BlockContentDataviewFilter) error {
	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	if filter.Id == "" {
		filter.Id = bson.NewObjectId().Hex()
	}
	view.Filters = append(view.Filters, filter)
	return nil
}

func (l *Dataview) RemoveFilters(viewID string, filterIDs []string) error {
	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	view.Filters = slice.Filter(view.Filters, func(f *model.BlockContentDataviewFilter) bool {
		return slice.FindPos(filterIDs, f.Id) == -1
	})
	return nil
}

func (l *Dataview) ReplaceFilter(viewID string, filterID string, filter *model.BlockContentDataviewFilter) error {
	view, err := l.GetView(viewID)
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

func (l *Dataview) ReorderFilters(viewID string, ids []string) error {
	view, err := l.GetView(viewID)
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

func (l *Dataview) AddSort(viewID string, sort *model.BlockContentDataviewSort) error {
	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	view.Sorts = append(view.Sorts, sort)
	return nil
}

func (l *Dataview) RemoveSorts(viewID string, relationKeys []string) error {
	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	view.Sorts = slice.Filter(view.Sorts, func(f *model.BlockContentDataviewSort) bool {
		return slice.FindPos(relationKeys, getViewSortID(f)) == -1
	})
	return nil
}

func (l *Dataview) ReplaceSort(viewID string, relationKey string, sort *model.BlockContentDataviewSort) error {
	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	idx := slice.Find(view.Sorts, func(f *model.BlockContentDataviewSort) bool {
		return getViewSortID(f) == relationKey
	})
	if idx < 0 {
		return fmt.Errorf("sort with id %s is not found", sort.RelationKey)
	}

	view.Sorts[idx] = sort

	return nil
}

func (l *Dataview) ReorderSorts(viewID string, relationKeys []string) error {
	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	sortsMap := make(map[string]*model.BlockContentDataviewSort)
	for _, f := range view.Sorts {
		sortsMap[getViewSortID(f)] = f
	}

	view.Sorts = view.Sorts[:0]
	for _, id := range relationKeys {
		if f, ok := sortsMap[id]; ok {
			view.Sorts = append(view.Sorts, f)
		}
	}
	return nil
}

func (l *Dataview) AddViewRelation(viewID string, relation *model.BlockContentDataviewRelation) error {
	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	view.Relations = append(view.Relations, relation)
	return nil
}

func (l *Dataview) RemoveViewRelations(viewID string, relationKeys []string) error {
	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	view.Relations = slice.Filter(view.Relations, func(f *model.BlockContentDataviewRelation) bool {
		return slice.FindPos(relationKeys, f.Key) == -1
	})
	return nil
}

func (l *Dataview) ReplaceViewRelation(viewID string, relationKey string, relation *model.BlockContentDataviewRelation) error {
	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	idx := slice.Find(view.Relations, func(f *model.BlockContentDataviewRelation) bool {
		return f.Key == relationKey
	})
	if idx < 0 {
		return fmt.Errorf("relation with key %s is not found", relationKey)
	}

	view.Relations[idx] = relation

	return nil
}

func (l *Dataview) ReorderViewRelations(viewID string, relationKeys []string) error {
	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	relationsMap := make(map[string]*model.BlockContentDataviewRelation)
	for _, r := range view.Relations {
		relationsMap[r.Key] = r
	}

	view.Relations = view.Relations[:0]
	for _, key := range relationKeys {
		if r, ok := relationsMap[key]; ok {
			view.Relations = append(view.Relations, r)
		}
	}
	return nil
}
