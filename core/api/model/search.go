package apimodel

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api/util"
)

type SortDirection string

const (
	Asc  SortDirection = "asc"
	Desc SortDirection = "desc"
)

func (sd *SortDirection) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch SortDirection(s) {
	case Asc, Desc:
		*sd = SortDirection(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid sort direction: %q", s))
	}
}

type SortProperty string

const (
	CreatedDate      SortProperty = "created_date"
	LastModifiedDate SortProperty = "last_modified_date"
	LastOpenedDate   SortProperty = "last_opened_date"
	Name             SortProperty = "name"
)

func (sp *SortProperty) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch SortProperty(s) {
	case CreatedDate, LastModifiedDate, LastOpenedDate, Name:
		*sp = SortProperty(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid sort property: %q", s))
	}
}

type SearchRequest struct {
	Query   string            `json:"query" example:"test"`               // The text to search within object names and content; use types field for type filtering
	Types   []string          `json:"types" example:"page,task,bookmark"` // The types of objects to include in results (e.g., "page", "task", "bookmark"); see ListTypes endpoint for valid values
	Sort    SortOptions       `json:"sort"`                               // The sorting options for the search results
	Filters *FilterExpression `json:"filters,omitempty"`                  // Expression filter with nested AND/OR conditions
}

type SortOptions struct {
	PropertyKey SortProperty  `json:"property_key" enums:"created_date,last_modified_date,last_opened_date,name" default:"last_modified_date"` // The key of the property to sort the search results by
	Direction   SortDirection `json:"direction" enums:"asc,desc" default:"desc"`                                                               // The direction to sort the search results by
}
