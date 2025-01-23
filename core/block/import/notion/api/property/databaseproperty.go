package property

import (
	"encoding/json"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// DatabaseProperties represent database properties (their structure is different from pages properties)
// use it when database doesn't have pages, so we can't extract properties from pages
type DatabaseProperties map[string]DatabasePropertyHandler

func (p *DatabaseProperties) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	props := parseDatabaseProperty(raw)
	*p = props
	return nil
}

func parseDatabaseProperty(raw map[string]interface{}) DatabaseProperties {
	result := make(DatabaseProperties)
	for k, v := range raw {
		p := getDatabasePropertyHandler(v)
		if p == nil {
			continue
		}
		if p != nil {
			result[k] = p
		}
	}
	return result
}

func getDatabasePropertyHandler(v interface{}) DatabasePropertyHandler {
	var p DatabasePropertyHandler
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
			return nil
		case PropertyConfigTypeRelation:
			p = &DatabaseRelation{}
		case PropertyConfigTypeRollup:
			// Database property Rollup doesn't have information about its format in database properties, so we don't add it
			return nil
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
		case PropertyConfigUniqueID:
			p = &DatabaseUnique{}
		default:
			log.Errorf("failed to get notion properties: unsupported property type: %s", rawProperty["type"].(string))
			return nil
		}
		b, err := json.Marshal(rawProperty)
		if err != nil {
			log.Errorf("failed to get notion properties, error: %s", err)
			return nil
		}

		if err = json.Unmarshal(b, &p); err != nil {
			log.Errorf("failed to get notion properties, error: %s", err)
			return nil
		}
	default:
		log.Errorf("failed to get notion properties: unsupported property format %T", v)
		return nil
	}
	return p
}

type DatabasePropertyHandler interface {
	FormatGetter
	IDGetter
	DetailSetter
}

type Property struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

type DatabaseTitle struct {
	Property
}

func (t *DatabaseTitle) GetID() string {
	return t.ID
}

func (t *DatabaseTitle) SetDetail(key string, details *domain.Details) {
	details.SetString(domain.RelationKey(key), "")
}

func (t *DatabaseTitle) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type DatabaseRichText struct {
	Property
}

func (rt *DatabaseRichText) GetID() string {
	return rt.ID
}

func (rt *DatabaseRichText) SetDetail(key string, details *domain.Details) {
	details.SetString(domain.RelationKey(key), "")
}

func (rt *DatabaseRichText) GetFormat() model.RelationFormat {
	return model.RelationFormat_longtext
}

type DatabaseNumber struct {
	Property
}

func (np *DatabaseNumber) GetID() string {
	return np.ID
}

func (np *DatabaseNumber) SetDetail(key string, details *domain.Details) {
	details.SetFloat64(domain.RelationKey(key), 0)
}

func (np *DatabaseNumber) GetFormat() model.RelationFormat {
	return model.RelationFormat_number
}

type DatabaseSelect struct {
	Property
}

func (sp *DatabaseSelect) GetID() string {
	return sp.ID
}

func (sp *DatabaseSelect) SetDetail(key string, details *domain.Details) {
	details.SetStringList(domain.RelationKey(key), []string{})
}

func (sp *DatabaseSelect) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

type DatabaseMultiSelect struct {
	Property
}

func (ms *DatabaseMultiSelect) GetID() string {
	return ms.ID
}

func (ms *DatabaseMultiSelect) SetDetail(key string, details *domain.Details) {
	details.SetStringList(domain.RelationKey(key), []string{})
}

func (ms *DatabaseMultiSelect) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

type DatabaseDate struct {
	Property
}

func (dp *DatabaseDate) GetID() string {
	return dp.ID
}

func (dp *DatabaseDate) SetDetail(key string, details *domain.Details) {
	details.SetFloat64(domain.RelationKey(key), 0)
}

func (dp *DatabaseDate) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

type DatabaseRelation struct {
	Property
}

func (rp *DatabaseRelation) GetID() string {
	return rp.ID
}

func (rp *DatabaseRelation) SetDetail(key string, details *domain.Details) {
	details.SetString(domain.RelationKey(key), "")
}

func (rp *DatabaseRelation) GetFormat() model.RelationFormat {
	return model.RelationFormat_object
}

type DatabasePeople struct {
	Property
}

func (p *DatabasePeople) GetID() string {
	return p.ID
}

func (p *DatabasePeople) SetDetail(key string, details *domain.Details) {
	details.SetStringList(domain.RelationKey(key), []string{})
}

func (p *DatabasePeople) GetFormat() model.RelationFormat {
	return model.RelationFormat_tag
}

type DatabaseFile struct {
	Property
}

func (f *DatabaseFile) GetID() string {
	return f.ID
}

func (f *DatabaseFile) SetDetail(key string, details *domain.Details) {
	details.SetString(domain.RelationKey(key), "")
}

func (f *DatabaseFile) GetFormat() model.RelationFormat {
	return model.RelationFormat_file
}

type DatabaseCheckbox struct {
	Property
}

func (c *DatabaseCheckbox) GetID() string {
	return c.ID
}

func (c *DatabaseCheckbox) SetDetail(key string, details *domain.Details) {
	details.SetBool(domain.RelationKey(key), false)
}

func (c *DatabaseCheckbox) GetFormat() model.RelationFormat {
	return model.RelationFormat_checkbox
}

type DatabaseURL struct {
	Property
}

func (u *DatabaseURL) GetID() string {
	return u.ID
}

func (u *DatabaseURL) SetDetail(key string, details *domain.Details) {
	details.SetString(domain.RelationKey(key), "")
}

func (u *DatabaseURL) GetFormat() model.RelationFormat {
	return model.RelationFormat_url
}

type DatabaseEmail struct {
	Property
}

func (e *DatabaseEmail) GetID() string {
	return e.ID
}

func (e *DatabaseEmail) SetDetail(key string, details *domain.Details) {
	details.SetString(domain.RelationKey(key), "")
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

func (ct *DatabaseCreatedTime) GetID() string {
	return ct.ID
}

func (ct *DatabaseCreatedTime) SetDetail(key string, details *domain.Details) {
	details.SetFloat64(domain.RelationKey(key), 0)
}

func (ct *DatabaseCreatedTime) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

type DatabaseCreatedBy struct {
	Property
}

func (cb *DatabaseCreatedBy) GetID() string {
	return cb.ID
}

func (cb *DatabaseCreatedBy) SetDetail(key string, details *domain.Details) {
	details.SetString(domain.RelationKey(key), "")
}

func (cb *DatabaseCreatedBy) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type DatabaseLastEditedTime struct {
	Property
}

func (le *DatabaseLastEditedTime) GetID() string {
	return le.ID
}

func (le *DatabaseLastEditedTime) SetDetail(key string, details *domain.Details) {
	details.SetFloat64(domain.RelationKey(key), 0)
}

func (le *DatabaseLastEditedTime) GetFormat() model.RelationFormat {
	return model.RelationFormat_date
}

type DatabaseLastEditedBy struct {
	Property
}

func (lb *DatabaseLastEditedBy) GetID() string {
	return lb.ID
}

func (lb *DatabaseLastEditedBy) SetDetail(key string, details *domain.Details) {
	details.SetString(domain.RelationKey(key), "")
}

func (lb *DatabaseLastEditedBy) GetFormat() model.RelationFormat {
	return model.RelationFormat_shorttext
}

type DatabaseStatus struct {
	Property
}

func (sp *DatabaseStatus) GetID() string {
	return sp.ID
}

func (sp *DatabaseStatus) SetDetail(key string, details *domain.Details) {
	details.SetStringList(domain.RelationKey(key), []string{})
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

func (v *DatabaseVerification) GetID() string {
	return v.ID
}

func (v *DatabaseVerification) SetDetail(key string, details *domain.Details) {
	details.SetStringList(domain.RelationKey(key), []string{})
}

type DatabaseUnique struct {
	Property
}

func (u *DatabaseUnique) GetFormat() model.RelationFormat {
	return model.RelationFormat_longtext
}

func (u *DatabaseUnique) GetID() string {
	return u.ID
}

func (u *DatabaseUnique) SetDetail(key string, details *domain.Details) {
	details.SetStringList(domain.RelationKey(key), []string{})
}
