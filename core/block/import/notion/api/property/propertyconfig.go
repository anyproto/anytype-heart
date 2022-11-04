package property

import (
	"encoding/json"
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
)

const (
	PropertyConfigTypeTitle       PropertyConfigType = "title"
	PropertyConfigTypeRichText    PropertyConfigType = "rich_text"
	PropertyConfigTypeNumber      PropertyConfigType = "number"
	PropertyConfigTypeSelect      PropertyConfigType = "select"
	PropertyConfigTypeMultiSelect PropertyConfigType = "multi_select"
	PropertyConfigTypeDate        PropertyConfigType = "date"
	PropertyConfigTypePeople      PropertyConfigType = "people"
	PropertyConfigTypeFiles       PropertyConfigType = "files"
	PropertyConfigTypeCheckbox    PropertyConfigType = "checkbox"
	PropertyConfigTypeURL         PropertyConfigType = "url"
	PropertyConfigTypeEmail       PropertyConfigType = "email"
	PropertyConfigTypePhoneNumber PropertyConfigType = "phone_number"
	PropertyConfigTypeFormula     PropertyConfigType = "formula"
	PropertyConfigTypeRelation    PropertyConfigType = "relation"
	PropertyConfigTypeRollup      PropertyConfigType = "rollup"
	PropertyConfigCreatedTime     PropertyConfigType = "created_time"
	PropertyConfigCreatedBy       PropertyConfigType = "created_by"
	PropertyConfigLastEditedTime  PropertyConfigType = "last_edited_time"
	PropertyConfigLastEditedBy    PropertyConfigType = "last_edited_by"
)

type PropertyConfigType string

type PropertyObject interface {
	GetPropertyType() PropertyConfigType
	GetID() string
}

type TitlePropertyConfig struct {
	ID    string             `json:"id,omitempty"`
	Type  PropertyConfigType `json:"type"`
	Title interface{}        `json:"title"`
}

func (t *TitlePropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeTitle
}

func (t *TitlePropertyConfig) GetID() string {
	return t.ID
}

type RichTextPropertyConfig struct {
	ID       string             `json:"id,omitempty"`
	Type     PropertyConfigType `json:"type"`
	RichText interface{}        `json:"rich_text"`
}

func (rt *RichTextPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeRichText
}

func (rt *RichTextPropertyConfig) GetID() string {
	return rt.ID
}

type NumberPropertyConfig struct {
	ID     string             `json:"id,omitempty"`
	Type   PropertyConfigType `json:"type"`
	Number NumberFormatType   `json:"format"`
}

func (n *NumberPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeNumber
}

func (n *NumberPropertyConfig) GetID() string {
	return n.ID
}

type NumberFormatType string

const (
	Number           NumberFormatType = "number"
	NumberWithCommas NumberFormatType = "number_with_commas"
	Percent          NumberFormatType = "percent"
	Dollar           NumberFormatType = "dollar"
	CanadianDollar   NumberFormatType = "canadian_dollar"
	Euro             NumberFormatType = "euro"
	Pound            NumberFormatType = "pound"
	Ruble            NumberFormatType = "ruble"
	Rupee            NumberFormatType = "rupee"
	Won              NumberFormatType = "won"
	Yuan             NumberFormatType = "yuan"
	Real             NumberFormatType = "real"
	Lira             NumberFormatType = "lira"
	Rupiah           NumberFormatType = "rupiah"
	Franc            NumberFormatType = "franc"
	HongKongDollar   NumberFormatType = "hong_kong_dollar"
	NewZealandDollar NumberFormatType = "new_zealand_dollar"
	Krona            NumberFormatType = "krona"
	NorwegianKrone   NumberFormatType = "norwegian_krone"
	MexicanPeso      NumberFormatType = "mexican_peso"
	Rand             NumberFormatType = "rand"
	NewTaiwanDollar  NumberFormatType = "new_taiwan_dollar"
	DanishKrone      NumberFormatType = "danish_krone"
	Zloty            NumberFormatType = "zloty"
	Baht             NumberFormatType = "baht"
	Forint           NumberFormatType = "forint"
	Koruna           NumberFormatType = "koruna"
	Shekel           NumberFormatType = "shekel"
	ChileanPeso      NumberFormatType = "chilean_peso"
	PhilippinePeso   NumberFormatType = "philippine_peso"
	Dirham           NumberFormatType = "dirham"
	ColombianPeso    NumberFormatType = "colombian_peso"
	Riyal            NumberFormatType = "riyal"
	Ringgit          NumberFormatType = "ringgit"
	Leu              NumberFormatType = "leu"
	ArgentinePeso    NumberFormatType = "argentine_peso"
	Uruguayan_Pso    NumberFormatType = "uruguayan_peso"
)

type SelectPropertyConfig struct {
	ID     string             `json:"id,omitempty"`
	Type   PropertyConfigType `json:"type"`
	Select Select             `json:"select"`
}

func (s *SelectPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeSelect
}

func (s *SelectPropertyConfig) GetID() string {
	return s.ID
}

type Select struct {
	Options []SelectOption `json:"options"`
}

type SelectOption struct {
	ID    string    `json:"id,omitempty"`
	Name  string    `json:"name"`
	Color api.Color `json:"color"`
}

type MultiSelectPropertyConfig struct {
	ID          string             `json:"id,omitempty"`
	Type        PropertyConfigType `json:"type"`
	MultiSelect Select             `json:"multi_select"`
}

func (m *MultiSelectPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeMultiSelect
}

func (m *MultiSelectPropertyConfig) GetID() string {
	return m.ID
}

type DatePropertyConfig struct {
	ID   string             `json:"id,omitempty"`
	Type PropertyConfigType `json:"type"`
	Date interface{}        `json:"date"`
}

func (d *DatePropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeDate
}

func (d *DatePropertyConfig) GetID() string {
	return d.ID
}

type PeoplePropertyConfig struct {
	ID     string             `json:"id,omitempty"`
	Type   PropertyConfigType `json:"type"`
	People interface{}        `json:"people"`
}

func (p *PeoplePropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypePeople
}

func (p *PeoplePropertyConfig) GetID() string {
	return p.ID
}

type FilesPropertyConfig struct {
	ID    string             `json:"id,omitempty"`
	Type  PropertyConfigType `json:"type"`
	Files interface{}        `json:"files"`
}

func (f *FilesPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeFiles
}

func (f *FilesPropertyConfig) GetID() string {
	return f.ID
}

type CheckboxPropertyConfig struct {
	ID       string             `json:"id,omitempty"`
	Type     PropertyConfigType `json:"type"`
	Checkbox interface{}        `json:"checkbox"`
}

func (*CheckboxPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeCheckbox
}

func (c *CheckboxPropertyConfig) GetID() string {
	return c.ID
}

type URLPropertyConfig struct {
	ID   string             `json:"id,omitempty"`
	Type PropertyConfigType `json:"type"`
	URL  interface{}        `json:"url"`
}

func (u *URLPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeURL
}

func (u *URLPropertyConfig) GetID() string {
	return u.ID
}

type EmailPropertyConfig struct {
	ID    string             `json:"id,omitempty"`
	Type  PropertyConfigType `json:"type"`
	Email interface{}        `json:"email"`
}

func (e *EmailPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeEmail
}

func (e *EmailPropertyConfig) GetID() string {
	return e.ID
}

type PhoneNumberPropertyConfig struct {
	ID          string             `json:"id,omitempty"`
	Type        PropertyConfigType `json:"type"`
	PhoneNumber interface{}        `json:"phone_number"`
}

func (p *PhoneNumberPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypePhoneNumber
}

func (p *PhoneNumberPropertyConfig) GetID() string {
	return p.ID
}

type FormulaPropertyConfig struct {
	ID      string             `json:"id,omitempty"`
	Type    PropertyConfigType `json:"type"`
	Formula FormulaConfig      `json:"formula"`
}

func (f *FormulaPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeFormula
}

func (f *FormulaPropertyConfig) GetID() string {
	return f.ID
}

type FormulaConfig struct {
	Expression string `json:"expression"`
}

type RelationPropertyConfig struct {
	ID       string             `json:"id,omitempty"`
	Type     PropertyConfigType `json:"type"`
	Relation RelationConfig     `json:"relation"`
}

func (r *RelationPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeRelation
}

func (r *RelationPropertyConfig) GetID() string {
	return r.ID
}

type RelationType string

const (
	SingleRelation RelationType = "single_property"
	DualRelation   RelationType = "dual_property"
)

type RelationConfig struct {
	DatabaseID     string       `json:"database_id"`
	Type           RelationType `json:"type"`
	SingleProperty interface{}  `json:"single_property,omitempty"`
	DualProperty   DualProperty `json:"dual_property,omitempty"`
}

type DualProperty struct {
	SyncedPropertyID   string `json:"synced_property_id,omitempty"`
	SyncedPropertyName string `json:"synced_property_name,omitempty"`
}

type RollupPropertyConfig struct {
	ID     string             `json:"id,omitempty"`
	Type   PropertyConfigType `json:"type"`
	Rollup RollupConfig       `json:"rollup"`
}

func (r *RollupPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigTypeRollup
}

func (r *RollupPropertyConfig) GetID() string {
	return r.ID
}

type FunctionType string

const (
	Count            FunctionType = "count"
	CountValues      FunctionType = "count_values"
	Empty            FunctionType = "empty"
	NotEmpty         FunctionType = "not_empty"
	Unique           FunctionType = "unique"
	ShowUnique       FunctionType = "show_unique"
	PercentEmpty     FunctionType = "percent_empty"
	PercentNotEmpty  FunctionType = "percent_not_empty"
	Sum              FunctionType = "sum"
	Average          FunctionType = "average"
	Median           FunctionType = "median"
	Min              FunctionType = "min"
	Max              FunctionType = "max"
	RangeFunction    FunctionType = "rangeFunction"
	EarliestDate     FunctionType = "earliest_date"
	LatestDate       FunctionType = "latest_date"
	DateRange        FunctionType = "date_range"
	Checked          FunctionType = "checked"
	Unchecked        FunctionType = "unchecked"
	PercentChecked   FunctionType = "percent_checked"
	PercentUnchecked FunctionType = "percent_unchecked"
	CountPerGroup    FunctionType = "count_per_group"
	PercentPerGroup  FunctionType = "percent_per_group"
	ShowOriginal     FunctionType = "show_original"
)

type RollupConfig struct {
	RelationPropertyName string       `json:"relation_property_name"`
	RelationPropertyID   string       `json:"relation_property_id"`
	RollupPropertyName   string       `json:"rollup_property_name"`
	RollupPropertyID     string       `json:"rollup_property_id"`
	Function             FunctionType `json:"function"`
}

type CreatedTimePropertyConfig struct {
	ID          string             `json:"id,omitempty"`
	Type        PropertyConfigType `json:"type"`
	CreatedTime interface{}        `json:"created_time"`
}

func (*CreatedTimePropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigCreatedTime
}

func (c *CreatedTimePropertyConfig) GetID() string {
	return c.ID
}

type CreatedByPropertyConfig struct {
	ID        string             `json:"id"`
	Type      PropertyConfigType `json:"type"`
	CreatedBy interface{}        `json:"created_by"`
}

func (*CreatedByPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigCreatedBy
}

func (c *CreatedByPropertyConfig) GetID() string {
	return c.ID
}

type LastEditedTimePropertyConfig struct {
	ID             string             `json:"id"`
	Type           PropertyConfigType `json:"type"`
	LastEditedTime interface{}        `json:"last_edited_time"`
}

func (*LastEditedTimePropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigLastEditedTime
}

func (l *LastEditedTimePropertyConfig) GetID() string {
	return l.ID
}

type LastEditedByPropertyConfig struct {
	ID           string             `json:"id"`
	Type         PropertyConfigType `json:"type"`
	LastEditedBy interface{}        `json:"last_edited_by"`
}

func (*LastEditedByPropertyConfig) GetPropertyType() PropertyConfigType {
	return PropertyConfigLastEditedBy
}

func (p LastEditedByPropertyConfig) GetType() PropertyConfigType {
	return p.Type
}

func (p *LastEditedByPropertyConfig) GetID() string {
	return p.ID
}

type Properties map[string]PropertyObject

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
		var p PropertyObject
		switch rawProperty := v.(type) {
		case map[string]interface{}:
			switch PropertyConfigType(rawProperty["type"].(string)) {
			case PropertyConfigTypeTitle:
				p = &TitlePropertyConfig{}
			case PropertyConfigTypeRichText:
				p = &RichTextPropertyConfig{}
			case PropertyConfigTypeNumber:
				p = &NumberPropertyConfig{}
			case PropertyConfigTypeSelect:
				p = &SelectPropertyConfig{}
			case PropertyConfigTypeMultiSelect:
				p = &MultiSelectPropertyConfig{}
			case PropertyConfigTypeDate:
				p = &DatePropertyConfig{}
			case PropertyConfigTypePeople:
				p = &PeoplePropertyConfig{}
			case PropertyConfigTypeFiles:
				p = &FilesPropertyConfig{}
			case PropertyConfigTypeCheckbox:
				p = &CheckboxPropertyConfig{}
			case PropertyConfigTypeURL:
				p = &URLPropertyConfig{}
			case PropertyConfigTypeEmail:
				p = &EmailPropertyConfig{}
			case PropertyConfigTypePhoneNumber:
				p = &PhoneNumberPropertyConfig{}
			case PropertyConfigTypeFormula:
				p = &FormulaPropertyConfig{}
			case PropertyConfigTypeRelation:
				p = &RelationPropertyConfig{}
			case PropertyConfigTypeRollup:
				p = &RollupPropertyConfig{}
			case PropertyConfigCreatedTime:
				p = &CreatedTimePropertyConfig{}
			case PropertyConfigCreatedBy:
				p = &CreatedByPropertyConfig{}
			case PropertyConfigLastEditedTime:
				p = &LastEditedTimePropertyConfig{}
			case PropertyConfigLastEditedBy:
				p = &LastEditedByPropertyConfig{}
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
