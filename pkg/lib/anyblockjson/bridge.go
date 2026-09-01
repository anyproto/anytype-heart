// Package anyblockjson is the Heart compatibility boundary for the portable
// AnyBlock implementation owned by github.com/anyproto/any-block.
//
// Heart keeps its store resolvers and generated protobuf model. This package
// converts only at that ownership boundary; format rules, schemas, validation,
// and JSON rendering remain in the external repository.
package anyblockjson

import (
	"encoding/json"
	"math/big"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	codec "github.com/anyproto/any-block/codec/anyblockjson"
	externaldomain "github.com/anyproto/any-block/codec/anyblockjson/domain"
	externalmodel "github.com/anyproto/any-block/format/v1/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	FormatVersion      = codec.FormatVersion
	IndexFileName      = codec.IndexFileName
	PropertiesFileName = codec.PropertiesFileName
)

type (
	Issue                   = codec.Issue
	ValidationError         = codec.ValidationError
	Legend                  = codec.Legend
	KeyVocabulary           = codec.KeyVocabulary
	ScopedKeyVocabulary     = codec.ScopedKeyVocabulary
	KeyTermFacts            = codec.KeyTermFacts
	BundledKeyVocabulary    = codec.BundledKeyVocabulary
	ObjectNameResolver      = codec.ObjectNameResolver
	ObjectExistenceResolver = codec.ObjectExistenceResolver
	ObjectDeletionResolver  = codec.ObjectDeletionResolver
	ParticipantResolver     = codec.ParticipantResolver
	TypeResolver            = codec.TypeResolver
	OptionDefinition        = codec.OptionDefinition
	TypeProperty            = codec.TypeProperty
	RecommendedList         = codec.RecommendedList
	Index                   = codec.Index
	PropertyDictionary      = codec.PropertyDictionary
)

var (
	DetectFormat              = codec.DetectFormat
	SchemaJSON                = codec.SchemaJSON
	FoldKeyTerm               = codec.FoldKeyTerm
	BundledPropertyKeyByName  = codec.BundledPropertyKeyByName
	BundledPropertyKeysByFold = codec.BundledPropertyKeysByFold
	BundledTypeKeyByName      = codec.BundledTypeKeyByName
	BundledTypeKeysByFold     = codec.BundledTypeKeysByFold
	DisambiguatedKeySpelling  = codec.DisambiguatedKeySpelling
	PropertyLabel             = codec.PropertyLabel
	TypeLabel                 = codec.TypeLabel
	KeyTermExtendsName        = codec.KeyTermExtendsName
	AuthorableBlockTypeNames  = codec.AuthorableBlockTypeNames
	BlockTypeNames            = codec.BlockTypeNames
	KnownBlockProperty        = codec.KnownBlockProperty
	StructuralBlockType       = codec.StructuralBlockType
	LeafBlockType             = codec.LeafBlockType
	TextBlockType             = codec.TextBlockType
	ViewTypeNames             = codec.ViewTypeNames
	ViewCardSizeNames         = codec.ViewCardSizeNames
	ViewListSizeNames         = codec.ViewListSizeNames
	ColumnAlignNames          = codec.ColumnAlignNames
	ColumnAggregationNames    = codec.ColumnAggregationNames
	ParseMarkdownBlocks       = codec.ParseMarkdownBlocks
	ParseMarkdownBlocksLimit  = codec.ParseMarkdownBlocksLimit
	IsCompactLabelShaped      = codec.IsCompactLabelShaped
)

// FormatResolver reports the stored format of a Heart property key.
type FormatResolver func(key domain.RelationKey) (model.RelationFormat, bool)

type OptionResolver interface {
	OptionName(key domain.RelationKey, id string) (string, bool)
	OptionId(key domain.RelationKey, name string) (string, bool)
}

// PropertyDefinition is the Heart-facing form of the portable definition.
// Its only non-portable members are the generated relation enum and key type.
type PropertyDefinition struct {
	Key             domain.RelationKey
	KeyIsInternal   bool
	Name            string
	Format          model.RelationFormat
	Options         []OptionDefinition
	ObjectTypes     []string
	Description     string
	IncludeTime     *bool
	IncludeTimeSet  bool
	MaxCount        int64
	Readonly        bool
	DefaultValue    any
	DefaultValueSet bool
}

type PropertyResolver interface {
	PropertyById(id string) (PropertyDefinition, bool)
	PropertyId(def PropertyDefinition) (string, bool)
}

// Options preserves the existing Heart call surface while ExternalOptions
// translates it to the repository-owned codec types.
type Options struct {
	ResolveFormat       FormatResolver
	ResolveOptions      OptionResolver
	ResolveProperties   PropertyResolver
	ResolveParticipants ParticipantResolver
	ResolveObjectNames  ObjectNameResolver
	SpaceId             string
	RefNames            bool
	// TableColumnHeaders enables the external codec's API-read annotation.
	TableColumnHeaders bool
	Keys               KeyVocabulary
	Legend             Legend
	OmitIds            bool
	CompactBlockLabels bool
	// CompactIds is the former name for CompactBlockLabels. Keep it at the
	// Heart boundary while API v2 callers migrate.
	CompactIds      bool
	GenerateId      func() string
	NormalizeIndent bool
	OnWarning       func(Issue)
}

type optionResolverAdapter struct{ inner OptionResolver }

func (a optionResolverAdapter) OptionName(key externaldomain.RelationKey, id string) (string, bool) {
	return a.inner.OptionName(domain.RelationKey(key), id)
}

func (a optionResolverAdapter) OptionId(key externaldomain.RelationKey, name string) (string, bool) {
	return a.inner.OptionId(domain.RelationKey(key), name)
}

type propertyResolverAdapter struct{ inner PropertyResolver }

func (a propertyResolverAdapter) PropertyById(id string) (codec.PropertyDefinition, bool) {
	def, ok := a.inner.PropertyById(id)
	if !ok {
		return codec.PropertyDefinition{}, false
	}
	return toExternalPropertyDefinition(def), true
}

func (a propertyResolverAdapter) PropertyId(def codec.PropertyDefinition) (string, bool) {
	return a.inner.PropertyId(fromExternalPropertyDefinition(def))
}

type propertyResolverWithTypes struct {
	propertyResolverAdapter
	types TypeResolver
}

func (a propertyResolverWithTypes) TypeKeyById(id string) (string, bool) {
	return a.types.TypeKeyById(id)
}

func (a propertyResolverWithTypes) TypeIdByKey(key string) (string, bool) {
	return a.types.TypeIdByKey(key)
}

func toExternalPropertyDefinition(def PropertyDefinition) codec.PropertyDefinition {
	return codec.PropertyDefinition{
		Key:             externaldomain.RelationKey(def.Key),
		KeyIsInternal:   def.KeyIsInternal,
		Name:            def.Name,
		Format:          externalmodel.RelationFormat(def.Format),
		Options:         def.Options,
		ObjectTypes:     def.ObjectTypes,
		Description:     def.Description,
		IncludeTime:     def.IncludeTime,
		IncludeTimeSet:  def.IncludeTimeSet,
		MaxCount:        def.MaxCount,
		Readonly:        def.Readonly,
		DefaultValue:    def.DefaultValue,
		DefaultValueSet: def.DefaultValueSet,
	}
}

func fromExternalPropertyDefinition(def codec.PropertyDefinition) PropertyDefinition {
	return PropertyDefinition{
		Key:             domain.RelationKey(def.Key),
		KeyIsInternal:   def.KeyIsInternal,
		Name:            def.Name,
		Format:          model.RelationFormat(def.Format),
		Options:         def.Options,
		ObjectTypes:     def.ObjectTypes,
		Description:     def.Description,
		IncludeTime:     def.IncludeTime,
		IncludeTimeSet:  def.IncludeTimeSet,
		MaxCount:        def.MaxCount,
		Readonly:        def.Readonly,
		DefaultValue:    def.DefaultValue,
		DefaultValueSet: def.DefaultValueSet,
	}
}

// ExternalOptions is used by Heart-only adapters such as bundle composition.
func ExternalOptions(opts Options) codec.Options {
	out := codec.Options{
		ResolveParticipants: opts.ResolveParticipants,
		ResolveObjectNames:  opts.ResolveObjectNames,
		SpaceId:             opts.SpaceId,
		RefNames:            opts.RefNames,
		TableColumnHeaders:  opts.TableColumnHeaders,
		Keys:                opts.Keys,
		Legend:              opts.Legend,
		OmitIds:             opts.OmitIds,
		CompactBlockLabels:  opts.CompactBlockLabels || opts.CompactIds,
		GenerateId:          opts.GenerateId,
		NormalizeIndent:     opts.NormalizeIndent,
		OnWarning:           opts.OnWarning,
	}
	if opts.ResolveFormat != nil {
		out.ResolveFormat = func(key externaldomain.RelationKey) (externalmodel.RelationFormat, bool) {
			format, ok := opts.ResolveFormat(domain.RelationKey(key))
			return externalmodel.RelationFormat(format), ok
		}
	}
	if opts.ResolveOptions != nil {
		out.ResolveOptions = optionResolverAdapter{inner: opts.ResolveOptions}
	}
	if opts.ResolveProperties != nil {
		base := propertyResolverAdapter{inner: opts.ResolveProperties}
		if typeResolver, ok := opts.ResolveProperties.(TypeResolver); ok {
			out.ResolveProperties = propertyResolverWithTypes{propertyResolverAdapter: base, types: typeResolver}
		} else {
			out.ResolveProperties = base
		}
	}
	return out
}

// Validate checks a document with the external codec's zero options.
func Validate(data []byte) error {
	return codec.Validate(data, codec.Options{})
}

// ValidateWarn checks a document and reports warning-grade issues through
// onWarning. The external codec now carries this sink in Options.
func ValidateWarn(data []byte, onWarning func(Issue)) error {
	return codec.Validate(data, codec.Options{OnWarning: onWarning})
}

// UnmarshalIndex validates and decodes a bundle index with zero options.
func UnmarshalIndex(data []byte) (*Index, error) {
	return codec.UnmarshalIndex(data, codec.Options{})
}

// UnmarshalPropertyDictionary validates and decodes a property dictionary
// with zero options.
func UnmarshalPropertyDictionary(data []byte) (*PropertyDictionary, error) {
	return codec.UnmarshalPropertyDictionary(data, codec.Options{})
}

func FormatName(format model.RelationFormat) string {
	return codec.FormatName(externalmodel.RelationFormat(format))
}

func FormatByName(name string) (model.RelationFormat, bool) {
	format, ok := codec.FormatByName(name)
	return model.RelationFormat(format), ok
}

func Marshal(sbType model.SmartBlockType, snapshot *model.SmartBlockSnapshotBase, opts Options) ([]byte, error) {
	external, err := ToExternalSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	return codec.Marshal(externalmodel.SmartBlockType(sbType), external, ExternalOptions(opts))
}

func Unmarshal(data []byte, opts Options) (model.SmartBlockType, *model.SmartBlockSnapshotBase, error) {
	sbType, snapshot, err := codec.Unmarshal(data, ExternalOptions(opts))
	if err != nil {
		return 0, nil, err
	}
	heart, err := FromExternalSnapshot(snapshot)
	return model.SmartBlockType(sbType), heart, err
}

func MarshalPropertyValue(key string, value *types.Value, opts Options) (any, map[string]string) {
	return codec.MarshalPropertyValue(key, value, ExternalOptions(opts))
}

func UnmarshalPropertyValue(key string, value any, opts Options) *types.Value {
	return codec.UnmarshalPropertyValue(key, value, ExternalOptions(opts))
}

func BuildRecommendedLists(properties []TypeProperty, opts Options) ([]RecommendedList, error) {
	return codec.BuildRecommendedLists(properties, ExternalOptions(opts))
}

func MarshalBlockSubtree(blocks []*model.Block, opts Options) (json.RawMessage, error) {
	external, err := ToExternalBlocks(blocks)
	if err != nil {
		return nil, err
	}
	return codec.MarshalBlockSubtree(external, ExternalOptions(opts))
}

func UnmarshalBlock(raw json.RawMessage, forcedId string, opts Options) ([]*model.Block, error) {
	blocks, err := codec.UnmarshalBlock(raw, forcedId, ExternalOptions(opts))
	if err != nil {
		return nil, err
	}
	return FromExternalBlocks(blocks)
}

func UnmarshalBlocks(raw []json.RawMessage, opts Options) ([]*model.Block, []string, error) {
	blocks, topIds, err := codec.UnmarshalBlocks(raw, ExternalOptions(opts))
	if err != nil {
		return nil, nil, err
	}
	heart, err := FromExternalBlocks(blocks)
	return heart, topIds, err
}

func UnmarshalFilters(raw json.RawMessage, opts Options) ([]*model.BlockContentDataviewFilter, error) {
	filters, err := codec.UnmarshalFilters(raw, ExternalOptions(opts))
	if err != nil {
		return nil, err
	}
	out := make([]*model.BlockContentDataviewFilter, len(filters))
	for i, filter := range filters {
		if filter == nil {
			continue
		}
		out[i] = &model.BlockContentDataviewFilter{}
		if err := transcode(filter, out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func UnmarshalSorts(raw json.RawMessage, opts Options) ([]*model.BlockContentDataviewSort, error) {
	sorts, err := codec.UnmarshalSorts(raw, ExternalOptions(opts))
	if err != nil {
		return nil, err
	}
	out := make([]*model.BlockContentDataviewSort, len(sorts))
	for i, sort := range sorts {
		if sort == nil {
			continue
		}
		out[i] = &model.BlockContentDataviewSort{}
		if err := transcode(sort, out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func ParseInlineText(markdown string) (string, []*model.BlockContentTextMark, error) {
	text, marks, err := codec.ParseInlineText(markdown)
	if err != nil {
		return "", nil, err
	}
	out := make([]*model.BlockContentTextMark, len(marks))
	for i, mark := range marks {
		if mark == nil {
			continue
		}
		out[i] = &model.BlockContentTextMark{}
		if err := transcode(mark, out[i]); err != nil {
			return "", nil, err
		}
	}
	return text, out, nil
}

func RenderInlineText(text string, marks []*model.BlockContentTextMark) string {
	external := make([]*externalmodel.BlockContentTextMark, len(marks))
	for i, mark := range marks {
		if mark == nil {
			continue
		}
		external[i] = &externalmodel.BlockContentTextMark{}
		if err := transcode(mark, external[i]); err != nil {
			return text
		}
	}
	return codec.RenderInlineText(text, external)
}

// ToExternalSnapshot and FromExternalSnapshot use the shared AnyBlock v1
// protobuf wire contract. Unknown fields are retained by protobuf, so the
// bridge does not need a second hand-written model mapping.
func ToExternalSnapshot(snapshot *model.SmartBlockSnapshotBase) (*externalmodel.SmartBlockSnapshotBase, error) {
	if snapshot == nil {
		return nil, nil
	}
	out := &externalmodel.SmartBlockSnapshotBase{}
	return out, transcode(snapshot, out)
}

func FromExternalSnapshot(snapshot *externalmodel.SmartBlockSnapshotBase) (*model.SmartBlockSnapshotBase, error) {
	if snapshot == nil {
		return nil, nil
	}
	out := &model.SmartBlockSnapshotBase{}
	return out, transcode(snapshot, out)
}

func ToExternalBlocks(blocks []*model.Block) ([]*externalmodel.Block, error) {
	out := make([]*externalmodel.Block, len(blocks))
	for i, block := range blocks {
		if block == nil {
			continue
		}
		out[i] = &externalmodel.Block{}
		if err := transcode(block, out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func FromExternalBlocks(blocks []*externalmodel.Block) ([]*model.Block, error) {
	out := make([]*model.Block, len(blocks))
	for i, block := range blocks {
		if block == nil {
			continue
		}
		out[i] = &model.Block{}
		if err := transcode(block, out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func transcode(from, to proto.Message) error {
	data, err := proto.Marshal(from)
	if err != nil {
		return err
	}
	return proto.Unmarshal(data, to)
}

const exactJSONIntegerDetailPrefix = "\x00anyblockjson:exact-integers:"

var maxSafeJSONInteger = big.NewInt(9007199254740991)

// ExactJSONIntegerMetadata remains Heart-side because it records precision
// metadata in Heart Details; it is not part of the AnyBlock document format.
func ExactJSONIntegerMetadata(key string, value any) (metadataKey, lexeme string, ok bool) {
	metadataKey = ExactJSONIntegerMetadataKey(key)
	number, ok := value.(json.Number)
	if !ok {
		return metadataKey, "", false
	}
	rational, ok := new(big.Rat).SetString(number.String())
	if !ok || !rational.IsInt() || new(big.Int).Abs(rational.Num()).Cmp(maxSafeJSONInteger) <= 0 {
		return metadataKey, "", false
	}
	return metadataKey, number.String(), true
}

func ExactJSONIntegerMetadataKey(key string) string {
	return exactJSONIntegerDetailPrefix + key
}
