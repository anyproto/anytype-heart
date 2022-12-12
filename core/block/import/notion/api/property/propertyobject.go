package property

import (
	"encoding/json"
	"fmt"
)

type Properties map[string]Object

func (p *Properties) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	props, err := parsePropertyConfigs(raw)
	if err != nil {
		return err
	}

	*p = props
	return nil
}

func parsePropertyConfigs(raw map[string]interface{}) (Properties, error) {
	result := make(Properties)
	for k, v := range raw {
		var p Object
		switch rawProperty := v.(type) {
		case map[string]interface{}:
			switch ConfigType(rawProperty["type"].(string)) {
			case PropertyConfigTypeTitle:
				p = &TitleItem{}
			case PropertyConfigTypeRichText:
				p = &RichTextItem{}
			case PropertyConfigTypeNumber:
				p = &NumberItem{}
			case PropertyConfigTypeSelect:
				p = &SelectItem{}
			case PropertyConfigTypeMultiSelect:
				p = &MultiSelectItem{}
			case PropertyConfigTypeDate:
				p = &DateItem{}
			case PropertyConfigTypePeople:
				p = &PeopleItem{}
			case PropertyConfigTypeFiles:
				p = &FileItem{}
			case PropertyConfigTypeCheckbox:
				p = &CheckboxItem{}
			case PropertyConfigTypeURL:
				p = &UrlItem{}
			case PropertyConfigTypeEmail:
				p = &EmailItem{}
			case PropertyConfigTypePhoneNumber:
				p = &PhoneItem{}
			case PropertyConfigTypeFormula:
				p = &FormulaItem{}
			case PropertyConfigTypeRelation:
				p = &RelationItem{}
			case PropertyConfigTypeRollup:
				p = &RollupItem{}
			case PropertyConfigCreatedTime:
				p = &CreatedTimeItem{}
			case PropertyConfigCreatedBy:
				p = &CreatedByItem{}
			case PropertyConfigLastEditedTime:
				p = &LastEditedTimeItem{}
			case PropertyConfigLastEditedBy:
				p = &LastEditedByItem{}
			case PropertyConfigStatus:
				p = &StatusItem{}
			default:
				return nil, fmt.Errorf("unsupported property type: %s", rawProperty["type"].(string))
			}
			b, err := json.Marshal(rawProperty)
			if err != nil {
				return nil, err
			}

			if err = json.Unmarshal(b, &p); err != nil {
				return nil, err
			}

			result[k] = p
		default:
			return nil, fmt.Errorf("unsupported property format %T", v)
		}
	}

	return result, nil
}
