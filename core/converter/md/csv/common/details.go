package common

import (
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
)

func GetValueAsString(details *domain.Details, localDetails *domain.Details, key domain.RelationKey) string {
	str := getStringValueFromDetail(details, key)
	if str == "" {
		return getStringValueFromDetail(localDetails, key)
	}
	return str
}

func getStringValueFromDetail(details *domain.Details, key domain.RelationKey) string {
	if details == nil {
		return ""
	}
	if boolValue, ok := details.TryBool(key); ok {
		return fmt.Sprintf("%t", boolValue)
	}
	if number, ok := details.TryFloat64(key); ok {
		return fmt.Sprintf("%g", number)
	}
	if intNumber, ok := details.TryInt64(key); ok {
		return fmt.Sprintf("%d", intNumber)
	}
	if str, ok := details.TryString(key); ok {
		return str
	}
	if strList, ok := details.TryStringList(key); ok {
		return strings.Join(strList, ", ")
	}
	return ""
}
