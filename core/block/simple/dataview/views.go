package dataview

import (
	"github.com/globalsign/mgo/bson"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

const DefaultViewRelationWidth = 192

func (d *Dataview) AddFilter(viewID string, filter *model.BlockContentDataviewFilter) error {
	d.resetObjectOrderForView(viewID)

	view, err := d.GetView(viewID)
	if err != nil {
		return err
	}

	if filter.Id == "" {
		filter.Id = bson.NewObjectId().Hex()
	}
	d.setRelationFormat(filter)
	view.Filters = append(view.Filters, filter)
	return nil
}

func (d *Dataview) RemoveFilters(viewID string, filterIDs []string) error {
	d.resetObjectOrderForView(viewID)

	view, err := d.GetView(viewID)
	if err != nil {
		return err
	}

	view.Filters = slice.Filter(view.Filters, func(f *model.BlockContentDataviewFilter) bool {
		return slice.FindPos(filterIDs, f.Id) == -1
	})
	return nil
}

func (d *Dataview) ReplaceFilter(viewID string, filterID string, filter *model.BlockContentDataviewFilter) error {
	d.resetObjectOrderForView(viewID)

	view, err := d.GetView(viewID)
	if err != nil {
		return err
	}

	idx := slice.Find(view.Filters, func(f *model.BlockContentDataviewFilter) bool {
		return f.Id == filterID
	})
	if idx < 0 {
		return d.AddFilter(viewID, filter)
	}

	filter.Id = filterID
	d.setRelationFormat(filter)
	view.Filters[idx] = filter

	return nil
}

func (d *Dataview) ReorderFilters(viewID string, ids []string) error {
	view, err := d.GetView(viewID)
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

func (d *Dataview) AddSort(viewID string, sort *model.BlockContentDataviewSort) error {
	d.resetObjectOrderForView(viewID)

	view, err := d.GetView(viewID)
	if err != nil {
		return err
	}

	if sort.Id == "" {
		sort.Id = bson.NewObjectId().Hex()
	}

	view.Sorts = append(view.Sorts, sort)
	return nil
}

func (d *Dataview) RemoveSorts(viewID string, ids []string) error {
	d.resetObjectOrderForView(viewID)

	view, err := d.GetView(viewID)
	if err != nil {
		return err
	}

	view.Sorts = slice.Filter(view.Sorts, func(f *model.BlockContentDataviewSort) bool {
		return slice.FindPos(ids, getViewSortID(f)) == -1
	})
	return nil
}

func (d *Dataview) ReplaceSort(viewID string, id string, sort *model.BlockContentDataviewSort) error {
	d.resetObjectOrderForView(viewID)

	view, err := d.GetView(viewID)
	if err != nil {
		return err
	}

	idx := slice.Find(view.Sorts, func(f *model.BlockContentDataviewSort) bool {
		return getViewSortID(f) == id
	})
	if idx < 0 {
		return d.AddSort(viewID, sort)
	}

	view.Sorts[idx] = sort

	return nil
}

func (d *Dataview) ReorderSorts(viewID string, ids []string) error {
	d.resetObjectOrderForView(viewID)
	view, err := d.GetView(viewID)
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

func (d *Dataview) AddViewRelation(viewID string, relation *model.BlockContentDataviewRelation) error {
	return d.ReplaceViewRelation(viewID, relation.Key, relation)
}

func (d *Dataview) RemoveViewRelations(viewID string, relationKeys []string) error {
	view, err := d.GetView(viewID)
	if err != nil {
		return err
	}

	view.Relations = slice.Filter(view.Relations, func(f *model.BlockContentDataviewRelation) bool {
		return slice.FindPos(relationKeys, f.Key) == -1
	})
	return nil
}

func (d *Dataview) ReplaceViewRelation(viewID string, relationKey string, relation *model.BlockContentDataviewRelation) error {
	view, err := d.GetView(viewID)
	if err != nil {
		return err
	}

	idx := slice.Find(view.Relations, func(f *model.BlockContentDataviewRelation) bool {
		return f.Key == relationKey
	})
	if idx < 0 {
		view.Relations = append(view.Relations, relation)
		return nil
	}

	view.Relations[idx] = relation

	return nil
}

func (d *Dataview) ReorderViewRelations(viewID string, relationKeys []string) error {
	view, err := d.GetView(viewID)
	if err != nil {
		return err
	}

	relationsMap := make(map[string]*model.BlockContentDataviewRelation)
	for _, r := range view.Relations {
		relationsMap[r.Key] = r

		// Add missing relation keys to requested order
		if !slices.Contains(relationKeys, r.Key) {
			relationKeys = append(relationKeys, r.Key)
		}
	}

	newRelations := make([]*model.BlockContentDataviewRelation, 0, len(view.Relations))
	for _, key := range relationKeys {
		// Ignore relations that don't present in view's relations
		if r, ok := relationsMap[key]; ok {
			newRelations = append(newRelations, r)
		}
	}
	view.Relations = newRelations
	return nil
}

func (d *Dataview) setRelationFormat(filter *model.BlockContentDataviewFilter) {
	for _, relLink := range d.content.RelationLinks {
		if relLink.Key == filter.RelationKey {
			filter.Format = relLink.Format
		}
	}
}
