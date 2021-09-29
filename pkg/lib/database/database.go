package database

import (
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/schema"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-datastore/query"
)

var log = logging.Logger("anytype-database")

const RecordIDField = "id"

type Record struct {
	Details *types.Struct
}

type Reader interface {
	Query(schema *schema.Schema, q Query) (records []Record, total int, err error)
	QueryAndSubscribeForChanges(schema *schema.Schema, q Query, subscription Subscription) (records []Record, close func(), total int, err error)

	QueryById(ids []string) (records []Record, err error)
	QueryByIdAndSubscribeForChanges(ids []string, subscription Subscription) (records []Record, close func(), err error)

	GetRelation(key string) (relation *model.Relation, err error)

	// ListRelations returns both indexed and bundled relations
	ListRelations(objType string) (relations []*model.Relation, err error)
	ListRelationsKeys() ([]string, error)

	AggregateRelationsFromObjectsOfType(objType string) (relations []*model.Relation, err error)
	AggregateRelationsFromSetsOfType(objType string) (relations []*model.Relation, err error)
	AggregateObjectIdsByOptionForRelation(relationKey string) (objectsByOptionId map[string][]string, err error)
	AggregateObjectIdsForOptionAndRelation(relationKey, optionId string) (objIds []string, err error)
}

type Writer interface {
	// Creating record involves some additional operations that may change
	// the record. So we return potentially modified record as a result.
	// in case subscription is not nil it will be subscribed to the record updates
	Create(relations []*model.Relation, rec Record, sub Subscription, templateId string) (Record, error)

	Update(id string, relations []*model.Relation, rec Record) error
	DeleteRelationOption(id string, relKey string, optionId string) error

	ModifyExtraRelations(id string, modifier func(current []*model.Relation) ([]*model.Relation, error)) error
	UpdateRelationOption(id string, relKey string, option model.RelationOption) (optionId string, err error)

	Delete(id string) error
}

type Database interface {
	Reader
	Writer

	//Schema() string
}

type Query struct {
	FullText               string
	Relations              []*model.BlockContentDataviewRelation // relations used to provide relations options
	Filters                []*model.BlockContentDataviewFilter   // filters results. apply sequentially
	Sorts                  []*model.BlockContentDataviewSort     // order results. apply hierarchically
	Limit                  int                                   // maximum number of results
	Offset                 int                                   // skip given number of results
	WithSystemObjects      bool
	IncludeArchivedObjects bool
	ObjectTypeFilter       []string
	WorkspaceId            string
	SearchInWorkspace      bool
}

func (q Query) DSQuery(sch *schema.Schema) (qq query.Query, err error) {
	qq.Limit = q.Limit
	qq.Offset = q.Offset
	f, err := newFilters(q, sch)
	if err != nil {
		return
	}
	qq.Filters = []query.Filter{f}
	if f.hasOrders() {
		qq.Orders = []query.Order{f}
	}
	qq.String()
	return
}

func PreFilters(includeArchived bool, sch *schema.Schema) filter.Filter {
	var preFilter filter.AndFilters
	if sch.ObjType != nil {
		relTypeFilter := filter.OrFilters{
			filter.Eq{
				Key:   bundle.RelationKeyType.String(),
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: pbtypes.String(sch.ObjType.Url),
			},
		}
		if sch.ObjType.Url == bundle.TypeKeyPage.URL() {
			relTypeFilter = append(relTypeFilter, filter.Empty{
				Key: bundle.RelationKeyType.String(),
			})
		}
		preFilter = append(preFilter, relTypeFilter)
	}

	if !includeArchived {
		preFilter = append(preFilter, filter.Not{Filter: filter.Eq{
			Key:   bundle.RelationKeyIsArchived.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.Bool(true),
		}})
	}

	return preFilter
}

func newFilters(q Query, sch *schema.Schema) (f *filters, err error) {
	f = new(filters)
	mainFilter := filter.AndFilters{}
	if sch != nil {
		for _, rel := range sch.Relations {
			if rel.Format == model.RelationFormat_date {
				if relation := getRelationByKey(q.Relations, rel.Key); relation == nil || !relation.DateIncludeTime {
					f.dateKeys = append(f.dateKeys, rel.Key)
				}
			}
		}
		if sch.ObjType != nil {
			for _, rel := range sch.ObjType.Relations {
				if rel.Format == model.RelationFormat_date {
					if relation := getRelationByKey(q.Relations, rel.Key); relation == nil || !relation.DateIncludeTime {
						f.dateKeys = append(f.dateKeys, rel.Key)
					}
				}
			}
		}
		for _, qf := range q.Filters {
			if slice.FindPos(f.dateKeys, qf.RelationKey) != -1 {
				qf.Value = dateOnly(qf.Value)
			}
		}

		preFilter := PreFilters(q.IncludeArchivedObjects, sch)
		if preFilter != nil {
			mainFilter = append(mainFilter, preFilter)
		}
	}
	qFilter, err := filter.MakeAndFilter(q.Filters)
	if err != nil {
		return
	}

	if len(qFilter.(filter.AndFilters)) > 0 {
		mainFilter = append(mainFilter, qFilter)
	}
	if q.SearchInWorkspace {
		if q.WorkspaceId != "" {
			threads.WorkspaceLogger.
				With("workspace id", q.WorkspaceId).
				With("text", q.FullText).
				Info("searching for text in workspace")
			filterOr := filter.OrFilters{
				filter.Eq{
					Key:   bundle.RelationKeyWorkspaceId.String(),
					Cond:  model.BlockContentDataviewFilter_Equal,
					Value: pbtypes.String(q.WorkspaceId),
				},
				filter.Like{
					Key:   bundle.RelationKeyType.String(),
					Value: pbtypes.String(bundle.TypeKeyObjectType.String()),
				},
				filter.Like{
					Key:   bundle.RelationKeyId.String(),
					Value: pbtypes.String(addr.BundledRelationURLPrefix),
				},
			}
			mainFilter = append(mainFilter, filterOr)
		} else {
			threads.WorkspaceLogger.
				With("workspace id", q.WorkspaceId).
				With("text", q.FullText).
				Info("searching for text in account")
			// it can also be the case that we want to search in current account
			// which is also kinda workspace
			mainFilter = append(mainFilter, filter.Empty{
				Key: bundle.RelationKeyWorkspaceId.String(),
			})
		}
	} else {
		threads.WorkspaceLogger.
			Info("searching in all workspaces and account")
	}
	f.filter = mainFilter
	if len(q.Sorts) > 0 {
		ord := filter.SetOrder{}
		for _, s := range q.Sorts {
			var emptyLast bool
			if s.RelationKey == bundle.RelationKeyName.String() {
				emptyLast = true
			}
			ord = append(ord, filter.KeyOrder{
				Key:       s.RelationKey,
				Type:      s.Type,
				EmptyLast: emptyLast,
			})
		}
		f.order = ord
	}
	return
}

type filterGetter struct {
	dateKeys []string
	curEl    *types.Struct
}

func (f filterGetter) Get(key string) *types.Value {
	res := pbtypes.Get(f.curEl, key)
	if slice.FindPos(f.dateKeys, key) != -1 {
		res = dateOnly(res)
	}
	return res
}

type filters struct {
	filter   filter.Filter
	order    filter.Order
	dateKeys []string
}

func (f *filters) Filter(e query.Entry) bool {
	g := f.unmarshal(e)
	if g == nil {
		return false
	}
	return f.filter.FilterObject(g)
}

func (f *filters) Compare(a, b query.Entry) int {
	if f.order == nil {
		return 0
	}
	ag := f.unmarshal(a)
	if ag == nil {
		return 0
	}
	bg := f.unmarshal(b)
	if bg == nil {
		return 0
	}
	return f.order.Compare(ag, bg)
}

func (f *filters) unmarshal(e query.Entry) filter.Getter {
	var details model.ObjectDetails
	err := proto.Unmarshal(e.Value, &details)
	if err != nil {
		log.Errorf("query filters decode error: %s", err.Error())
		return nil
	}
	return filterGetter{dateKeys: f.dateKeys, curEl: details.Details}
}

func (f *filters) hasOrders() bool {
	return f.order != nil
}

func (f *filters) String() string {
	var fs string
	if f.filter != nil {
		fs = fmt.Sprintf("WHERE %v", f.filter.String())
	}
	// TODO: order to string
	return fs
}

func dateOnly(v *types.Value) *types.Value {
	if n, isNumber := v.GetKind().(*types.Value_NumberValue); isNumber {
		tm := time.Unix(int64(n.NumberValue), 0).In(time.UTC)                 // we have all values stored in UTC, including filters
		tm = time.Date(tm.Year(), tm.Month(), tm.Day(), 0, 0, 0, 0, time.UTC) // reset time, preserving UTC tz
		return pbtypes.Float64(float64(tm.Unix()))
	}
	// reset to NULL otherwise
	return &types.Value{Kind: &types.Value_NullValue{}}
}

func getRelationByKey(relations []*model.BlockContentDataviewRelation, key string) *model.BlockContentDataviewRelation {
	for _, relation := range relations {
		if relation.Key == key {
			return relation
		}
	}
	return nil
}
