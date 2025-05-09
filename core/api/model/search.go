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
	Query string      `json:"query" example:"test"`                                                                                      // The search term to look for in object names and snippets
	Types []string    `json:"types" example:"page,678043f0cda9133be777049f,bafyreightzrdts2ymxyaeyzspwdfo2juspyam76ewq6qq7ixnw3523gs7q"` // The types of objects to search for, specified by key or ID
	Sort  SortOptions `json:"sort"`                                                                                                      // The sorting criteria and direction for the search results
}

type SortOptions struct {
	PropertyKey SortProperty  `json:"property_key" binding:"required" enums:"created_date,last_modified_date,last_opened_date,name" default:"last_modified_date"` // The property to sort the search results by
	Direction   SortDirection `json:"direction" binding:"required" enums:"asc,desc" default:"desc"`                                                               // The direction to sort the search results
}
