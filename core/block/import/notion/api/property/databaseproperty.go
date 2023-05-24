package property

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// DatabaseProperties represent database properties (their structure is different from pages properties)
// use it when database doesn't have pages, so we can't extract properties from pages
type DatabaseProperties map[string]interface{}

func (p *DatabaseProperties) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	props, err := parseDatabaseProperty(raw)
	if err != nil {
		return err
	}

	*p = props
	return nil
}

func parseDatabaseProperty(raw map[string]interface{}) (DatabaseProperties, error) {
	result := make(DatabaseProperties)
	for k, v := range raw {
		p, err := getFormatGetter(v)
		if err != nil {
			return nil, err
		}
		if p != nil {
			result[k] = p
		}
	}

	return result, nil
}

func getFormatGetter(v interface{}) (FormatGetter, error) {
	var p FormatGetter
	switch rawProperty := v.(type) {
	case map[string]interface{}:
		switch ConfigType(rawProperty["type"].(string)) {
		case PropertyConfigTypeTitle:
			p = &DatabaseTitle{}
		case PropertyConfigTypeRichText:
			p = &DatabaseRichText{}
		case PropertyConfigTypeNumber:
			p = &DatabaseNumber{}
		case PropertyConfigTypeSelect:
			p = &DatabaseSelect{}
		case PropertyConfigTypeMultiSelect:
			p = &DatabaseMultiSelect{}
		case PropertyConfigTypeDate:
			p = &DatabaseDate{}
		case PropertyConfigTypePeople:
			p = &DatabasePeople{}
		case PropertyConfigTypeFiles:
			p = &DatabaseFile{}
		case PropertyConfigTypeCheckbox:
			p = &DatabaseCheckbox{}
		case PropertyConfigTypeURL:
			p = &DatabaseURL{}
		case PropertyConfigTypeEmail:
			p = &DatabaseEmail{}
		case PropertyConfigTypePhoneNumber:
			p = &DatabaseNumber{}
		case PropertyConfigTypeFormula:
			// Database property Formula doesn't have information about its format in database properties, so we don't add it
			return nil, nil
		case PropertyConfigTypeRelation:
			p = &DatabaseRelation{}
		case PropertyConfigTypeRollup:
			// Database property Rollup doesn't have information about its format in database properties, so we don't add it
			return nil, nil
		case PropertyConfigCreatedTime:
			p = &DatabaseCreatedTime{}
		case PropertyConfigCreatedBy:
			p = &DatabaseCreatedBy{}
		case PropertyConfigLastEditedTime:
			p = &DatabaseLastEditedTime{}
		case PropertyConfigLastEditedBy:
			p = &DatabaseLastEditedBy{}
		case PropertyConfigStatus:
			p = &DatabaseStatus{}
		case PropertyConfigVerification:
			p = &DatabaseVerification{}
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

	default:
		return nil, fmt.Errorf("unsupported property format %T", v)
	}
	return p, nil
}

type FormatGetter interface {
	GetFormat() model.RelationFormat
}

type Property struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

type DatabaseTitle struct {
	Property
}

func (t *DatabaseTitle) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type DatabaseRichText struct {
	Property
}

func (rt *DatabaseRichText) GetFormat() model.RelationFormat {
	return model.RelationFormat_longtext
}

type DatabaseNumber struct {
	Property
}

func (np *DatabaseNumber) GetFormat() model.RelationFormat {
	return model.RelationFormat_number
}

type DatabaseSelect struct {
	Property
}

func (sp *DatabaseSelect) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

type DatabaseMultiSelect struct {
	Property
}

func (ms *DatabaseMultiSelect) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

type DatabaseDate struct {
	Property
}

func (dp *DatabaseDate) GetFormat() model.RelationFormat {
	return model.RelationFormat_longtext
}

type DatabaseRelation struct {
	Property
}

func (rp *DatabaseRelation) GetFormat() model.RelationFormat {
	return model.RelationFormat_object
}

type DatabasePeople struct {
	Property
}

func (p *DatabasePeople) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

type DatabaseFile struct {
	Property
}

func (f *DatabaseFile) GetFormat() model.RelationFormat {
	return model.RelationFormat_file
}

type DatabaseCheckbox struct {
	Property
}

func (c *DatabaseCheckbox) GetFormat() model.RelationFormat {
	return model.RelationFormat_checkbox
}

type DatabaseURL struct {
	Property
}

func (u *DatabaseURL) GetFormat() model.RelationFormat {
	return model.RelationFormat_url
}

type DatabaseEmail struct {
	Property
}

func (e *DatabaseEmail) GetFormat() model.RelationFormat {
	return model.RelationFormat_email
}

type DatabasePhone struct {
	Property
}

func (p *DatabasePhone) GetFormat() model.RelationFormat {
	return model.RelationFormat_phone
}

type DatabaseCreatedTime struct {
	Property
}

func (ct *DatabaseCreatedTime) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

type DatabaseCreatedBy struct {
	Property
}

func (cb *DatabaseCreatedBy) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type DatabaseLastEditedTime struct {
	Property
}

func (le *DatabaseLastEditedTime) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

type DatabaseLastEditedBy struct {
	Property
}

func (lb *DatabaseLastEditedBy) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type DatabaseStatus struct {
	Property
}

func (sp *DatabaseStatus) GetFormat() model.RelationFormat {
	return model.RelationFormat_status
}

type DatabasePhoneNumber struct {
	Property
}

func (r *DatabasePhoneNumber) GetFormat() model.RelationFormat {
	return model.RelationFormat_phone
}

type DatabaseVerification struct {
	Property
}

func (v *DatabaseVerification) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}
