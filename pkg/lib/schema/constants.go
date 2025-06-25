package schema

// JSON Schema standard field names
const (
	// Core schema fields
	jsonSchemaFieldSchema               = "$schema"
	jsonSchemaFieldID                   = "$id"
	jsonSchemaFieldType                 = "type"
	jsonSchemaFieldTitle                = "title"
	jsonSchemaFieldDescription          = "description"
	jsonSchemaFieldProperties           = "properties"
	jsonSchemaFieldItems                = "items"
	jsonSchemaFieldRequired             = "required"
	jsonSchemaFieldAdditionalProperties = "additionalProperties"
	jsonSchemaFieldEnum                 = "enum"
	jsonSchemaFieldConst                = "const"
	jsonSchemaFieldFormat               = "format"
	jsonSchemaFieldPattern              = "pattern"
	jsonSchemaFieldDefault              = "default"
	jsonSchemaFieldExamples             = "examples"
	jsonSchemaFieldReadOnly             = "readOnly"
	jsonSchemaFieldMaxLength            = "maxLength"
)

// JSON Schema type values
const (
	jsonSchemaTypeObject  = "object"
	jsonSchemaTypeArray   = "array"
	jsonSchemaTypeString  = "string"
	jsonSchemaTypeNumber  = "number"
	jsonSchemaTypeInteger = "integer"
	jsonSchemaTypeBoolean = "boolean"
	jsonSchemaTypeNull    = "null"
)

// JSON Schema format values
const (
	jsonSchemaFormatDate     = "date"
	jsonSchemaFormatDateTime = "date-time"
	jsonSchemaFormatEmail    = "email"
	jsonSchemaFormatURI      = "uri"
	jsonSchemaFormatBytes    = "bytes"
)

// Anytype extension field names (x-* fields)
const (
	anytypeFieldApp           = "x-app"
	anytypeFieldSchemaVersion = "x-schema-version"
	anytypeFieldTypeKey       = "x-type-key"
	anytypeFieldKey           = "x-key"
	anytypeFieldFormat        = "x-format"
	anytypeFieldOrder         = "x-order"
	anytypeFieldFeatured      = "x-featured"
	anytypeFieldHidden        = "x-hidden"
	anytypeFieldPlural        = "x-plural"
	anytypeFieldIconEmoji     = "x-icon-emoji"
	anytypeFieldIconName      = "x-icon-name"
	anytypeFieldObjectTypes   = "x-object-types"
)

// Anytype format values
const (
	anytypeFormatShortText = "shorttext"
	anytypeFormatLongText  = "longtext"
	anytypeFormatFile      = "file"
	anytypeFormatCheckbox  = "checkbox"
	anytypeFormatStatus    = "status"
	anytypeFormatTag       = "tag"
	anytypeFormatPhone     = "phone"
	anytypeFormatURL       = "url"
)

// Special property names
const (
	propertyNameID         = "id"
	propertyNameCollection = "Collection"
)

// Other constants
const (
	anytypeAppName     = "Anytype"
	extensionPrefix    = "x-"
	phoneNumberPattern = "^[+]?[0-9\\s()-]+$"
	jsonSchemaVersion  = "http://json-schema.org/draft-07/schema#"
)
