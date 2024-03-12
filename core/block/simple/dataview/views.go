package dataview

import (
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (l *Dataview) AddFilter(viewID string, filter *model.BlockContentDataviewFilter) error {
	l.resetObjectOrderForView(viewID)

	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	if filter.Id == "" {
		filter.Id = bson.NewObjectId().Hex()
	}
	l.setRelationFormat(filter)
	view.Filters = append(view.Filters, filter)
	return nil
}

func (l *Dataview) RemoveFilters(viewID string, filterIDs []string) error {
	l.resetObjectOrderForView(viewID)

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
	l.resetObjectOrderForView(viewID)

	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	idx := slice.Find(view.Filters, func(f *model.BlockContentDataviewFilter) bool {
		return f.Id == filterID
	})
	if idx < 0 {
		return l.AddFilter(viewID, filter)
	}

	filter.Id = filterID
	l.setRelationFormat(filter)
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
	l.resetObjectOrderForView(viewID)

	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	if sort.Id == "" {
		sort.Id = bson.NewObjectId().Hex()
	}

	view.Sorts = append(view.Sorts, sort)
	return nil
}

func (l *Dataview) RemoveSorts(viewID string, ids []string) error {
	l.resetObjectOrderForView(viewID)

	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	view.Sorts = slice.Filter(view.Sorts, func(f *model.BlockContentDataviewSort) bool {
		return slice.FindPos(ids, getViewSortID(f)) == -1
	})
	return nil
}

func (l *Dataview) ReplaceSort(viewID string, id string, sort *model.BlockContentDataviewSort) error {
	l.resetObjectOrderForView(viewID)

	view, err := l.GetView(viewID)
	if err != nil {
		return err
	}

	idx := slice.Find(view.Sorts, func(f *model.BlockContentDataviewSort) bool {
		return getViewSortID(f) == id
	})
	if idx < 0 {
		return l.AddSort(viewID, sort)
	}

	view.Sorts[idx] = sort

	return nil
}

func (l *Dataview) ReorderSorts(viewID string, ids []string) error {
	view, err := l.GetView(viewID)
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
		return l.AddViewRelation(viewID, relation)
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

func (l *Dataview) setRelationFormat(filter *model.BlockContentDataviewFilter) {
	for _, relLink := range l.content.RelationLinks {
		if relLink.Key == filter.RelationKey {
			filter.Format = relLink.Format
		}
	}
}
