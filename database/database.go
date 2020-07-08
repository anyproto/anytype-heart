package database

import (
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/schema"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-datastore/query"
)

var log = logging.Logger("anytype-database")

type Entry struct {
	Details *types.Struct
}

type Database interface {
	Query(q Query) (entries []Entry, total int, err error)
	Schema() string
}

type Query struct {
	Relations []*model.BlockContentDataviewRelation // relations used to provide relations options
	Filters   []*model.BlockContentDataviewFilter   // filters results. apply sequentially
	Sorts     []*model.BlockContentDataviewSort     // order results. apply hierarchically
	Limit     int                                   // maximum number of results
	Offset    int                                   // skip given number of results
}

func (q Query) DSQuery(schemaURL string) query.Query {
	sch, err := schema.Get(schemaURL)
	if err != nil {
		log.Errorf("failed to get schema %s: %s", schemaURL, err.Error())
	}

	return query.Query{
		Filters: []query.Filter{filters{Filters: q.Filters, Relations: q.Relations, Schema: sch}},
		Orders:  []query.Order{order{Sorts: q.Sorts, Relations: q.Relations, Schema: sch}},
		Limit:   q.Limit,
		Offset:  q.Offset,
	}
}

type order struct {
	Sorts     []*model.BlockContentDataviewSort
	Relations []*model.BlockContentDataviewRelation

	Schema *schema.Schema
}

type filters struct {
	Filters   []*model.BlockContentDataviewFilter
	Relations []*model.BlockContentDataviewRelation

	Schema *schema.Schema
}

func getTime(v *types.Value, trimTime bool) time.Time {
	if v == nil {
		return time.Time{}
	}

	t := time.Unix(int64(v.GetNumberValue()), 0)
	if trimTime {
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}
	return t
}

func getRelationById(relations []*model.BlockContentDataviewRelation, id string) *model.BlockContentDataviewRelation {
	for _, relation := range relations {
		if relation.Id == id {
			return relation
		}
	}
	return nil
}

func (filters filters) Filter(e query.Entry) bool {
	if len(filters.Filters) == 0 {
		return true
	}

	var details model.PageDetails
	err := proto.Unmarshal(e.Value, &details)
	if err != nil {
		log.Errorf("query filters decode error: %s", err.Error())
		return false
	}

	if details.Details == nil {
		return false
	}

	total := true
	for _, filter := range filters.Filters {
		var res bool
		rel, err := filters.Schema.GetRelationById(filter.RelationId)
		if err != nil {
			log.Errorf("failed to find relation %s for the filter: %s", filter.RelationId, err.Error())
			continue
		}
		isDate := rel.Type == "https://anytype.io/schemas/types/date"

		var dateTrimTime = true
		if isDate {
			relation := getRelationById(filters.Relations, filter.RelationId)
			if relation == nil {
				log.Errorf("failed to get relation options for %s: %s", filter.RelationId, err.Error())
			} else {
				dateTrimTime = relation.GetDateOptions() != nil && !relation.GetDateOptions().IncludeTime
			}
		}

		// todo: compare nil and empty
		switch filter.Condition {
		case model.BlockContentDataviewFilter_Equal:
			if v1, ok := filter.Value.Kind.(*types.Value_StringValue); ok {
				if details.Details == nil || details.Details.Fields == nil || details.Details.Fields[filter.RelationId] == nil {
					res = v1.String() == ""
				} else if v2, ok := details.Details.Fields[filter.RelationId].Kind.(*types.Value_StringValue); ok {
					res = strings.EqualFold(v1.String(), v2.String())
				}
			} else {
				if isDate {
					val := getTime(details.Details.Fields[filter.RelationId], dateTrimTime)
					filterVal := getTime(filter.Value, dateTrimTime)
					res = filterVal.Equal(val)
				} else {
					res = filter.Value.Equal(details.Details.Fields[filter.RelationId])
				}
			}
		case model.BlockContentDataviewFilter_NotEqual:
			if isDate {
				val := getTime(details.Details.Fields[filter.RelationId], dateTrimTime)
				filterVal := getTime(filter.Value, dateTrimTime)
				res = !filterVal.Equal(val)
			} else {
				res = !filter.Value.Equal(details.Details.Fields[filter.RelationId])
			}
		case model.BlockContentDataviewFilter_Greater:
			if isDate {
				val := getTime(details.Details.Fields[filter.RelationId], dateTrimTime)
				filterVal := getTime(filter.Value, dateTrimTime)
				res = val.After(filterVal)
			} else {
				res = filter.Value.Compare(details.Details.Fields[filter.RelationId]) == -1
			}
		case model.BlockContentDataviewFilter_Less:
			if isDate {
				val := getTime(details.Details.Fields[filter.RelationId], dateTrimTime)
				filterVal := getTime(filter.Value, dateTrimTime)
				res = val.Before(filterVal)
			} else {
				res = filter.Value.Compare(details.Details.Fields[filter.RelationId]) == 1
			}
		case model.BlockContentDataviewFilter_GreaterOrEqual:
			if isDate {
				val := getTime(details.Details.Fields[filter.RelationId], dateTrimTime)
				filterVal := getTime(filter.Value, dateTrimTime)
				res = val.After(filterVal) || val.Equal(filterVal)
			} else {
				res = filter.Value.Compare(details.Details.Fields[filter.RelationId]) <= 0
			}
		case model.BlockContentDataviewFilter_LessOrEqual:
			if isDate {
				val := getTime(details.Details.Fields[filter.RelationId], dateTrimTime)
				filterVal := getTime(filter.Value, dateTrimTime)
				res = val.Before(filterVal) || val.Equal(filterVal)
			} else {
				res = filter.Value.Compare(details.Details.Fields[filter.RelationId]) >= 0
			}
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

			if strings.Contains(strings.ToLower(relation.GetStringValue()), strings.ToLower(filter.Value.GetStringValue())) {
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

			if !strings.Contains(strings.ToLower(relation.GetStringValue()), strings.ToLower(filter.Value.GetStringValue())) {
				res = true
				break
			}
		case model.BlockContentDataviewFilter_In, model.BlockContentDataviewFilter_NotIn:
			var list *types.ListValue
			if list = filter.Value.GetListValue(); list == nil {
				log.Errorf("In filters should provide List value")
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
		case model.BlockContentDataviewFilter_Empty:
			switch v := details.Details.Fields[filter.RelationId].Kind.(type) {
			case *types.Value_NullValue:
				res = true
			case *types.Value_StringValue:
				res = v.StringValue == ""
			case *types.Value_ListValue:
				res = v.ListValue == nil || len(v.ListValue.Values) == 0
			case *types.Value_StructValue:
				res = v.StructValue == nil
			case *types.Value_NumberValue:
				if isDate {
					res = getTime(details.Details.Fields[filter.RelationId], dateTrimTime).IsZero()
				}
			default:
				res = false
			}
		case model.BlockContentDataviewFilter_NotEmpty:
			switch v := details.Details.Fields[filter.RelationId].Kind.(type) {
			case *types.Value_NullValue:
				res = false
			case *types.Value_StringValue:
				res = v.StringValue != ""
			case *types.Value_ListValue:
				res = v.ListValue != nil && len(v.ListValue.Values) > 0
			case *types.Value_StructValue:
				res = v.StructValue != nil
			case *types.Value_NumberValue:
				if isDate {
					res = !getTime(details.Details.Fields[filter.RelationId], dateTrimTime).IsZero()
				}
			default:
				res = true
			}
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
		log.Errorf("query filters decode error: %s", err.Error())
		return -1
	}

	var bDetails model.PageDetails
	err = proto.Unmarshal(b.Value, &bDetails)
	if err != nil {
		log.Errorf("query filters decode error: %s", err.Error())
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
