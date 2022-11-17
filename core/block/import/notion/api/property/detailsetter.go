package property

import (
	"github.com/gogo/protobuf/types"
)

type DetailValueSetter struct{}

// New is a constructor for DetailValueSetter
func NewDetailSetter() *DetailValueSetter {
	return &DetailValueSetter{}
}

// SetDetailValue creates Detail based on property type and value
func (*DetailValueSetter) SetDetailValue(key string, propertyType PropertyConfigType, property []DetailSetter, details map[string]*types.Value) error {
	if len(property) == 0 {
		return nil
	}
	if IsVector(propertyType) {
		for _, pr := range property {
			pr.SetDetail(key, details)
		}
	} else {
		property[0].SetDetail(key, details)
	}
	return nil
}