package filter

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Parser handles parsing of query parameters into filters
type Parser struct{}

// NewParser creates a new filter parser
func NewParser() *Parser {
	return &Parser{}
}

// conditionPattern matches filter conditions in square brackets
var conditionPattern = regexp.MustCompile(`^(.+)\[(\w+)\]$`)

// ParseQueryParams parses query parameters into filters
func (p *Parser) ParseQueryParams(c *gin.Context) (*ParsedFilters, error) {
	queryParams := c.Request.URL.Query()
	filters := make([]Filter, 0)

	skipParams := map[string]bool{
		"offset": true,
		"limit":  true,
		"sort":   true,
		"order":  true,
	}

	for key, values := range queryParams {
		if skipParams[key] || len(values) == 0 {
			continue
		}

		property, condition, err := p.parseFilterKey(key)
		if err != nil {
			return nil, util.ErrBadInput(fmt.Sprintf("invalid filter key %q: %s", key, err.Error()))
		}

		value, err := p.parseFilterValue(values[0], condition)
		if err != nil {
			return nil, util.ErrBadInput(fmt.Sprintf("invalid filter value for %q: %s", key, err.Error()))
		}

		filters = append(filters, Filter{
			PropertyKey: property,
			Condition:   condition,
			Value:       value,
		})
	}

	return &ParsedFilters{Filters: filters}, nil
}

// parseFilterKey extracts property name and condition from a filter key
func (p *Parser) parseFilterKey(key string) (property string, condition model.BlockContentDataviewFilterCondition, err error) {
	if matches := conditionPattern.FindStringSubmatch(key); len(matches) == 3 {
		property = matches[1]
		conditionStr := strings.ToLower(matches[2])

		cond, ok := ToInternalCondition(apimodel.FilterCondition(conditionStr))
		if !ok {
			return "", 0, util.ErrBadInput(fmt.Sprintf("unsupported condition: %s", conditionStr))
		}
		condition = cond
	} else {
		property = key
		condition = model.BlockContentDataviewFilter_Equal
	}

	if property == "" {
		return "", 0, util.ErrBadInput("empty property name")
	}

	return property, condition, nil
}

// parseFilterValue parses the filter value based on the condition
func (p *Parser) parseFilterValue(rawValue string, condition model.BlockContentDataviewFilterCondition) (interface{}, error) {
	decodedValue, err := url.QueryUnescape(rawValue)
	if err != nil {
		return nil, util.ErrBadInput(fmt.Sprintf("failed to decode value: %s", err.Error()))
	}

	switch condition {
	case model.BlockContentDataviewFilter_Empty, model.BlockContentDataviewFilter_NotEmpty:
		if decodedValue == "" || decodedValue == "true" || decodedValue == "1" {
			return true, nil
		}
		return false, nil

	case model.BlockContentDataviewFilter_In,
		model.BlockContentDataviewFilter_NotIn,
		model.BlockContentDataviewFilter_AllIn:
		if decodedValue == "" {
			return []string{}, nil
		}
		values := strings.Split(decodedValue, ",")
		for i, v := range values {
			values[i] = strings.TrimSpace(v)
		}
		return values, nil

	default:
		return decodedValue, nil
	}
}
