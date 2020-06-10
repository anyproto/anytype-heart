package database

import (
	"strings"

	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-datastore/query"
)

var log = logging.Logger("anytype-database")

type Entry struct {
	Details *types.Struct
}

type Database interface {
	Query(q Query) ([]Entry, error)
	Schema() string
}

type Query struct {
	Filters []*model.BlockContentDataviewFilter // filter results. apply sequentially
	Sorts   []*model.BlockContentDataviewSort   // order results. apply hierarchically
	Limit   int                                 // maximum number of results
	Offset  int                                 // skip given number of results
}

func (q Query) DSQuery() query.Query {
	return query.Query{
		Filters: []query.Filter{filter{Filters: q.Filters}},
		Orders:  []query.Order{order{Sorts: q.Sorts}},
		Limit:   q.Limit,
		Offset:  q.Offset,
	}
}

type order struct {
	Sorts []*model.BlockContentDataviewSort
}

type filter struct {
	Filters []*model.BlockContentDataviewFilter
}

func (filter filter) Filter(e query.Entry) bool {
	if len(filter.Filters) == 0 {
		return true
	}

	var details model.PageDetails
	err := proto.Unmarshal(e.Value, &details)
	if err != nil {
		log.Errorf("query filter decode error: %s", err.Error())
		return false
	}

	if details.Details == nil {
		return false
	}

	total := true
	for _, filter := range filter.Filters {
		var res bool
		// todo: compare nil and empty
		switch filter.Condition {
		case model.BlockContentDataviewFilter_Equal:
			res = filter.Value.Equal(details.Details.Fields[filter.RelationId])
		case model.BlockContentDataviewFilter_NotEqual:
			res = !filter.Value.Equal(details.Details.Fields[filter.RelationId])
		case model.BlockContentDataviewFilter_Greater:
			res = filter.Value.Compare(details.Details.Fields[filter.RelationId]) == -1
		case model.BlockContentDataviewFilter_Less:
			res = filter.Value.Compare(details.Details.Fields[filter.RelationId]) == 1
		case model.BlockContentDataviewFilter_Like:
			// todo: support for SQL LIKE query symbols like %?
			if filter.Value.GetStringValue() == "" {
				res = false
				break
			}

			relation := details.Details.Fields[filter.RelationId]
			if relation == nil {
				res = false
				break
			}
			if strings.Contains(relation.GetStringValue(), filter.Value.GetStringValue()) {
				res = true
				break
			}

		case model.BlockContentDataviewFilter_NotLike:
			// todo: support for SQL LIKE query symbols like %?
			if filter.Value.GetStringValue() == "" {
				res = false
				break
			}

			relation := details.Details.Fields[filter.RelationId]
			if relation == nil {
				res = true
				break
			}

			if !strings.Contains(relation.GetStringValue(), filter.Value.GetStringValue()) {
				res = true
				break
			}
		case model.BlockContentDataviewFilter_In, model.BlockContentDataviewFilter_NotIn:
			var list *types.ListValue
			if list = filter.Value.GetListValue(); list == nil {
				log.Errorf("In filter should provide List value")
				res = false
				break
			}

			detail := details.Details.Fields[filter.RelationId]
			if detail == nil {
				res = false
				break
			}

			var matchFound bool
			for _, item := range list.Values {
				if item.Equal(detail) {
					matchFound = true
					break
				}
			}

			res = matchFound

			if filter.Condition == model.BlockContentDataviewFilter_NotIn {
				res = !res
			}
			break
		}

		if filter.Operator == model.BlockContentDataviewFilter_And {
			total = total && res
		} else {
			total = total || res
		}
	}
	return total
}

func (order order) Compare(a query.Entry, b query.Entry) int {
	if len(order.Sorts) == 0 {
		// todo: default sort?
		return 0
	}

	var aDetails model.PageDetails
	err := proto.Unmarshal(a.Value, &aDetails)
	if err != nil {
		log.Errorf("query filter decode error: %s", err.Error())
		return -1
	}

	var bDetails model.PageDetails
	err = proto.Unmarshal(b.Value, &bDetails)
	if err != nil {
		log.Errorf("query filter decode error: %s", err.Error())
		return -1
	}

	for _, sort := range order.Sorts {
		var arelation, brelation *types.Value
		if aDetails.Details != nil && aDetails.Details.Fields != nil {
			arelation = aDetails.Details.Fields[sort.RelationId]
		}

		if bDetails.Details != nil && bDetails.Details.Fields != nil {
			brelation = bDetails.Details.Fields[sort.RelationId]
		}

		res := arelation.Compare(brelation)
		if sort.Type == model.BlockContentDataviewSort_Asc {
			if res == -1 {
				return -1
			}

			if res == 1 {
				return 1
			}
		} else if sort.Type == model.BlockContentDataviewSort_Desc {
			if res == -1 {
				return 1
			}

			if res == 1 {
				return -1
			}
		}
	}

	return 0
}
