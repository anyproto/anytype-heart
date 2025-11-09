package filter

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// topLevelAttributes maps JSON field names to internal relation keys
// These attributes default to "contains" search and don't require a spaceId
var topLevelAttributes = map[string]string{
	"name":        bundle.RelationKeyName.String(),
	"global_name": bundle.RelationKeyGlobalName.String(),
	"snippet":     bundle.RelationKeySnippet.String(),
}

type Parser struct {
	apiService ApiService
}

func NewParser(apiService ApiService) *Parser {
	return &Parser{
		apiService: apiService,
	}
}

// conditionPattern matches filter conditions in square brackets
var conditionPattern = regexp.MustCompile(`^(.+)\[(\w+)\]$`)

// ParseQueryParams parses query parameters into filters
func (p *Parser) ParseQueryParams(c *gin.Context, spaceId string) (*ParsedFilters, error) {
	queryParams := c.Request.URL.Query()
	filters := make([]Filter, 0)

	skipParams := map[string]bool{
		pagination.QueryParamOffset: true,
		pagination.QueryParamLimit:  true,
	}

	for key, values := range queryParams {
		if skipParams[key] || len(values) == 0 {
			continue
		}

		relationKey, condition, err := p.parseFilterKey(key, spaceId)
		if err != nil {
			return nil, util.ErrBadInput(fmt.Sprintf("invalid filter key %q: %s", key, err.Error()))
		}

		value, err := p.parseFilterValue(values[0], condition)
		if err != nil {
			return nil, util.ErrBadInput(fmt.Sprintf("invalid filter value for %q: %s", key, err.Error()))
		}

		filters = append(filters, Filter{
			PropertyKey: relationKey,
			Condition:   condition,
			Value:       value,
		})
	}

	return &ParsedFilters{Filters: filters}, nil
}

// parseFilterKey extracts relation key and condition from a filter key (e.g., "name[eq]" -> "name", Equal)
func (p *Parser) parseFilterKey(key string, spaceId string) (relationKey string, condition model.BlockContentDataviewFilterCondition, err error) {
	if matches := conditionPattern.FindStringSubmatch(key); len(matches) == 3 {
		relationKey = matches[1]
		conditionStr := strings.ToLower(matches[2])

		cond, ok := ToInternalCondition(apimodel.FilterCondition(conditionStr))
		if !ok {
			return "", 0, util.ErrBadInput(fmt.Sprintf("unsupported condition: %q", conditionStr))
		}
		condition = cond
	} else {
		relationKey = key
		condition = p.getDefaultCondition(relationKey, spaceId)
	}

	if relationKey == "" {
		return "", 0, util.ErrBadInput("empty property name")
	}

	// Resolve JSON field names to internal relation keys
	if rk, ok := topLevelAttributes[relationKey]; ok {
		relationKey = rk
	} else if spaceId != "" {
		propertyMap := p.apiService.GetCachedProperties(spaceId)
		if rk, found := p.apiService.ResolvePropertyApiKey(propertyMap, relationKey); found {
			relationKey = rk
		}
	}

	return relationKey, condition, nil
}

// getDefaultCondition returns the default condition for a property
func (p *Parser) getDefaultCondition(propertyKey string, spaceId string) model.BlockContentDataviewFilterCondition {
	// Top-level attributes default to Contains
	if _, isTopLevel := topLevelAttributes[propertyKey]; isTopLevel {
		return model.BlockContentDataviewFilter_Like // Contains
	}

	if spaceId == "" {
		return model.BlockContentDataviewFilter_Equal
	}

	propertyMap := p.apiService.GetCachedProperties(spaceId)
	rk, found := p.apiService.ResolvePropertyApiKey(propertyMap, propertyKey)
	if !found {
		return model.BlockContentDataviewFilter_Equal
	}

	prop, exists := propertyMap[rk]
	if !exists {
		return model.BlockContentDataviewFilter_Equal
	}

	// Text properties default to Contains, others to Equal
	switch prop.Format {
	case apimodel.PropertyFormatText, apimodel.PropertyFormatUrl,
		apimodel.PropertyFormatEmail, apimodel.PropertyFormatPhone:
		return model.BlockContentDataviewFilter_Like // Contains
	default:
		return model.BlockContentDataviewFilter_Equal
	}
}

// parseFilterValue parses the filter value based on the condition
func (p *Parser) parseFilterValue(rawValue string, condition model.BlockContentDataviewFilterCondition) (interface{}, error) {
	decodedValue, err := url.QueryUnescape(rawValue)
	if err != nil {
		return nil, util.ErrBadInput(fmt.Sprintf("failed to decode value: %s", err.Error()))
	}

	switch condition {
	case model.BlockContentDataviewFilter_Empty, model.BlockContentDataviewFilter_NotEmpty:
		if decodedValue == "" {
			return true, nil
		}
		boolValue, err := strconv.ParseBool(decodedValue)
		if err != nil {
			return nil, util.ErrBadInput(fmt.Sprintf("invalid boolean value %q", decodedValue))
		}
		return boolValue, nil

	case model.BlockContentDataviewFilter_In, model.BlockContentDataviewFilter_NotIn, model.BlockContentDataviewFilter_AllIn:
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
