package bundle

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	types2 "github.com/gogo/protobuf/types"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

// RequiredInternalRelations contains internal relations will be added to any new object type.
// Missing ones will be added to object on opening or during reindex
var RequiredInternalRelations = []RelationKey{
	RelationKeyId,
	RelationKeyName,
	RelationKeyDescription,
	RelationKeyIconEmoji,
	RelationKeyIconImage,
	RelationKeyType,
	RelationKeyLayout,
	RelationKeyCreatedDate,
	RelationKeyCreator,
	RelationKeyLastModifiedDate,
	RelationKeyLastModifiedBy,
	RelationKeyLastOpenedDate,
	RelationKeyIsHidden,
	RelationKeyIsArchived,
}

var FormatFilePossibleTargetObjectTypes = []string{
	TypeKeyFile.URL(),
	TypeKeyImage.URL(),
	TypeKeyVideo.URL(),
	TypeKeyAudio.URL()}

// filled in init
var LocalRelationsKeys []string   // stored only in localstore
var DerivedRelationsKeys []string // derived

var ErrNotFound = fmt.Errorf("not found")

func init() {
	for _, r := range relations {
		if r.DataSource == relation.Relation_account {
			LocalRelationsKeys = append(LocalRelationsKeys, r.Key)
		} else if r.DataSource == relation.Relation_derived {
			DerivedRelationsKeys = append(DerivedRelationsKeys, r.Key)
		}
	}
}

func GetTypeByUrl(u string) (*relation.ObjectType, error) {
	if !strings.HasPrefix(u, TypePrefix) {
		return nil, fmt.Errorf("invalid url with no bundled type prefix")
	}
	tk := TypeKey(strings.TrimPrefix(u, TypePrefix))
	if v, exists := types[tk]; exists {
		return pbtypes.CopyObjectType(v), nil
	}

	return nil, ErrNotFound
}

// MustGetType returns built-in object type by predefined TypeKey constant
// PANICS IN CASE RELATION KEY IS NOT EXISTS – DO NOT USE WITH ARBITRARY STRING
func MustGetType(tk TypeKey) *relation.ObjectType {
	if v, exists := types[tk]; exists {
		return pbtypes.CopyObjectType(v)
	}

	// we can safely panic in case TypeKey is a generated constant
	panic(ErrNotFound)
}

// MustGetRelation returns built-in relation by predefined RelationKey constant
// PANICS IN CASE RELATION KEY IS NOT EXISTS – DO NOT USE WITH ARBITRARY STRING
func MustGetRelation(rk RelationKey) *relation.Relation {
	if v, exists := relations[rk]; exists {
		return pbtypes.CopyRelation(v)
	}

	// we can safely panic in case RelationKey is a generated constant
	panic(ErrNotFound)
}

func GetRelation(rk RelationKey) (*relation.Relation, error) {
	if v, exists := relations[rk]; exists {
		return pbtypes.CopyRelation(v), nil
	}

	return nil, ErrNotFound
}

// MustGetLayout returns built-in layout by predefined Layout constant
// PANICS IN CASE RELATION KEY IS NOT EXISTS – DO NOT USE WITH ARBITRARY STRING
func MustGetLayout(lk relation.ObjectTypeLayout) *relation.Layout {
	if v, exists := Layouts[lk]; exists {
		return pbtypes.CopyLayout(&v)
	}

	// we can safely panic in case RelationKey is a generated constant
	panic(ErrNotFound)
}

func GetLayout(lk relation.ObjectTypeLayout) (*relation.Layout, error) {
	if v, exists := Layouts[lk]; exists {
		return pbtypes.CopyLayout(&v), nil
	}

	return nil, ErrNotFound
}

func ListRelations() []*relation.Relation {
	var rels []*relation.Relation
	for _, rel := range relations {
		rels = append(rels, pbtypes.CopyRelation(rel))
	}

	return rels
}

func ListRelationsKeys() []RelationKey {
	var keys []RelationKey
	for k, _ := range relations {
		keys = append(keys, k)
	}

	return keys
}

func ListRelationsUrls() []string {
	var keys []string
	for k, _ := range relations {
		keys = append(keys, addr.BundledRelationURLPrefix+k.String())
	}

	return keys
}

func HasRelation(key string) bool {
	_, exists := relations[RelationKey(key)]

	return exists
}

func HasObjectType(key string) bool {
	_, exists := types[TypeKey(key)]

	return exists
}

func EqualWithRelation(key string, rel *relation.Relation) (equal bool, exists bool) {
	v, exists := relations[RelationKey(key)]
	if !exists {
		return false, false
	}

	return pbtypes.RelationEqualOmitDictionary(v, rel), true
}

func ListTypes() ([]*relation.ObjectType, error) {
	var otypes []*relation.ObjectType
	for _, ot := range types {
		otypes = append(otypes, ot)
	}

	return otypes, nil
}

func ListTypesKeys() []TypeKey {
	var keys []TypeKey
	for k, _ := range types {
		keys = append(keys, k)
	}

	return keys
}

func GetDetailsForRelation(bundled bool, rel *relation.Relation) ([]*relation.Relation, *types2.Struct) {
	var prefix string
	if bundled {
		prefix = addr.BundledRelationURLPrefix
	} else {
		prefix = addr.CustomRelationURLPrefix
	}

	d := &types2.Struct{Fields: map[string]*types2.Value{
		RelationKeyName.String():             pbtypes.String(rel.Name),
		RelationKeyDescription.String():      pbtypes.String(rel.Description),
		RelationKeyId.String():               pbtypes.String(prefix + rel.Key),
		RelationKeyType.String():             pbtypes.StringList([]string{TypeKeyRelation.URL()}),
		RelationKeyCreator.String():          pbtypes.String(rel.Creator),
		RelationKeyLayout.String():           pbtypes.Float64(float64(relation.ObjectType_relation)),
		RelationKeyRelationFormat.String():   pbtypes.Float64(float64(rel.Format)),
		RelationKeyIsHidden.String():         pbtypes.Bool(rel.Hidden),
		RelationKeyMpAddedToLibrary.String(): pbtypes.Bool(true), // temp
	}}

	var rels []*relation.Relation
	for k := range d.Fields {
		rels = append(rels, MustGetRelation(RelationKey(k)))
	}
	return rels, d
}
