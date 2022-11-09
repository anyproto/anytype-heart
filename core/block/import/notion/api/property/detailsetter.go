package property

import (
	"fmt"

	"github.com/gogo/protobuf/types"
)

type DetailSetter struct{}

func NewDetailSetter() *DetailSetter {
	return &DetailSetter{}
}

func (*DetailSetter) SetDetailValue(key string, propertyType PropertyConfigType, property []interface{}, details map[string]*types.Value) error {
	if len(property) == 0 {
		return nil
	}
	switch propertyType {
	case PropertyConfigTypeTitle:
		for _, v := range property {
			title := v.(TitleItem)
			title.SetDetail(key, details)
		}
	case PropertyConfigTypeRichText:
		for _, v := range property {
			rt := v.(RichTextItem)
			rt.SetDetail(key, details)
		}
	case PropertyConfigTypePeople:
		for _, v := range property {
			p := v.(PeopleItem)
			p.SetDetail(key, details)
		}
	case PropertyConfigTypeRelation:
		for _, v := range property {
			r := v.(RelationItem)
			r.SetDetail(key, details)
		}
	case PropertyConfigTypeNumber:
		number := property[0].(NumberItem)
		number.SetDetail(key, details)
	case PropertyConfigTypeSelect:
		selectProperty := property[0].(SelectItem)
		selectProperty.SetDetail(key, details)
	case PropertyConfigTypeMultiSelect:
		multiSelect := property[0].(MultiSelectItem)
		multiSelect.SetDetail(key, details)
	case PropertyConfigTypeDate:
	case PropertyConfigTypeFiles:
		f := property[0].(FileItem)
		f.SetDetail(key, details)
	case PropertyConfigTypeCheckbox:
		c := property[0].(CheckboxItem)
		c.SetDetail(key, details)
	case PropertyConfigTypeURL:
		url := property[0].(UrlItem)
		url.SetDetail(key, details)
	case PropertyConfigTypeEmail:
		email := property[0].(EmailItem)
		email.SetDetail(key, details)
	case PropertyConfigTypePhoneNumber:
		phone := property[0].(PhoneItem)
		phone.SetDetail(key, details)
	case PropertyConfigTypeFormula:
		formula := property[0].(FormulaItem)
		formula.SetDetail(key, details)
	case PropertyConfigTypeRollup:
	case PropertyConfigCreatedTime:
		ct := property[0].(CreatedTimeItem)
		ct.SetDetail(key, details)
	case PropertyConfigCreatedBy:
		cb := property[0].(CreatedByItem)
		cb.SetDetail(key, details)
	case PropertyConfigLastEditedTime:
		lt := property[0].(LastEditedTimeItem)
		lt.SetDetail(key, details)
	case PropertyConfigLastEditedBy:
		lb := property[0].(LastEditedByItem)
		lb.SetDetail(key, details)
	case PropertyConfigStatus:
		lb := property[0].(StatusItem)
		lb.SetDetail(key, details)
	default:
		return fmt.Errorf("unsupported property type: %s", propertyType)
	}
	return nil
}