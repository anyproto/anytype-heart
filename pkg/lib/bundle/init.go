package bundle

import (
	"fmt"
	"strings"

	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func makeSystemRelationsMap() map[RelationKey]struct{} {
	res := make(map[RelationKey]struct{}, len(SystemRelations))
	for _, k := range SystemRelations {
		res[k] = struct{}{}
	}
	return res
}

var systemRelationsMap = makeSystemRelationsMap()

func (rk RelationKey) IsSystem() bool {
	_, ok := systemRelationsMap[rk]
	return ok
}

var FormatFilePossibleTargetObjectTypes = []TypeKey{
	TypeKeyFile,
	TypeKeyImage,
	TypeKeyVideo,
	TypeKeyAudio,
}

var DefaultObjectTypePerSmartblockType = map[coresb.SmartBlockType]TypeKey{
	coresb.SmartBlockTypePage:        TypeKeyPage,
	coresb.SmartBlockTypeProfilePage: TypeKeyProfile,
	coresb.SmartBlockTypeHome:        TypeKeyDashboard,
	coresb.SmartBlockTypeTemplate:    TypeKeyTemplate,
	coresb.SmartBlockTypeWidget:      TypeKeyDashboard,
	coresb.SmartBlockTypeObjectType:  TypeKeyObjectType,
	coresb.SmartBlockTypeRelation:    TypeKeyRelation,
}

// filled in init
var LocalRelationsKeys []string   // stored only in localstore
var DerivedRelationsKeys []string // derived

var ErrNotFound = fmt.Errorf("not found")

func init() {
	for _, r := range relations {
		if r.DataSource == model.Relation_account || r.DataSource == model.Relation_local {
			LocalRelationsKeys = append(LocalRelationsKeys, r.Key)
		} else if r.DataSource == model.Relation_derived {
			DerivedRelationsKeys = append(DerivedRelationsKeys, r.Key)
		}
	}
}

func HasObjectTypeID(id string) bool {
	if !strings.HasPrefix(id, TypePrefix) {
		return false
	}
	tk := TypeKey(strings.TrimPrefix(id, TypePrefix))
	_, exists := types[tk]
	return exists
}

func GetTypeByUrl(u string) (*model.ObjectType, error) {
	if !strings.HasPrefix(u, TypePrefix) {
		return nil, fmt.Errorf("invalid url with no bundled type prefix")
	}
	tk := TypeKey(strings.TrimPrefix(u, TypePrefix))
	if v, exists := types[tk]; exists {
		t := pbtypes.CopyObjectType(v)
		t.Key = tk.String()
		return t, nil
	}

	return nil, ErrNotFound
}

// MustGetType returns built-in object type by predefined TypeKey constant
// PANICS IN CASE RELATION KEY IS NOT EXISTS – DO NOT USE WITH ARBITRARY STRING
func MustGetType(tk TypeKey) *model.ObjectType {
	if v, exists := types[tk]; exists {
		t := pbtypes.CopyObjectType(v)
		t.Key = tk.String()
		return t
	}

	// we can safely panic in case TypeKey is a generated constant
	panic(ErrNotFound)
}

// MustGetRelation returns built-in relation by predefined RelationKey constant
// PANICS IN CASE RELATION KEY IS NOT EXISTS – DO NOT USE WITH ARBITRARY STRING
func MustGetRelation(rk RelationKey) *model.Relation {
	if v, exists := relations[rk]; exists {
		d := pbtypes.CopyRelation(v)
		d.Id = addr.BundledRelationURLPrefix + d.Key
		return d
	}

	// we can safely panic in case RelationKey is a generated constant
	panic(ErrNotFound)
}

// MustGetRelation returns built-in relation by predefined RelationKey constant
// PANICS IN CASE RELATION KEY IS NOT EXISTS – DO NOT USE WITH ARBITRARY STRING
func MustGetRelationLink(rk RelationKey) *model.RelationLink {
	if v, exists := relations[rk]; exists {
		return &model.RelationLink{Key: v.Key, Format: v.Format}
	}

	// we can safely panic in case RelationKey is a generated constant
	panic(ErrNotFound)
}

func MustGetRelations(rks []RelationKey) []*model.Relation {
	rels := make([]*model.Relation, 0, len(rks))
	for _, rk := range rks {
		rels = append(rels, MustGetRelation(rk))
	}
	return rels
}

func GetRelation(rk RelationKey) (*model.Relation, error) {
	if v, exists := relations[rk]; exists {
		v := pbtypes.CopyRelation(v)
		v.Id = addr.BundledRelationURLPrefix + v.Key
		return v, nil
	}

	return nil, ErrNotFound
}

// MustGetLayout returns built-in layout by predefined Layout constant
// PANICS IN CASE RELATION KEY IS NOT EXISTS – DO NOT USE WITH ARBITRARY STRING
func MustGetLayout(lk model.ObjectTypeLayout) *model.Layout {
	if v, exists := Layouts[lk]; exists {
		return pbtypes.CopyLayout(&v)
	}

	// we can safely panic in case RelationKey is a generated constant
	panic(ErrNotFound)
}

func GetLayout(lk model.ObjectTypeLayout) (*model.Layout, error) {
	if v, exists := Layouts[lk]; exists {
		return pbtypes.CopyLayout(&v), nil
	}

	return nil, ErrNotFound
}

func ListRelations() []*model.Relation {
	var rels []*model.Relation
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

func HasObjectTypeByKey(key TypeKey) bool {
	_, exists := types[key]

	return exists
}

func EqualWithRelation(key string, rel *model.Relation) (equal bool, exists bool) {
	v, exists := relations[RelationKey(key)]
	if !exists {
		return false, false
	}

	return pbtypes.RelationEqualOmitDictionary(v, rel), true
}

func ListTypes() ([]*model.ObjectType, error) {
	var otypes []*model.ObjectType
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

func HasRelationKey(rels []RelationKey, rel RelationKey) bool {
	for _, rel1 := range rels {
		if rel1 == rel {
			return true
		}
	}

	return false
}

func TypeKeyFromUrl(url string) (TypeKey, error) {
	if strings.HasPrefix(url, addr.BundledObjectTypeURLPrefix) {
		return TypeKey(strings.TrimPrefix(url, addr.BundledObjectTypeURLPrefix)), nil
	}

	if strings.HasPrefix(url, addr.ObjectTypeKeyToIdPrefix) {
		return TypeKey(strings.TrimPrefix(url, addr.ObjectTypeKeyToIdPrefix)), nil
	}

	return "", fmt.Errorf("invalid type url: no prefix found")
}

func RelationKeyFromID(id string) (RelationKey, error) {
	if strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return RelationKey(strings.TrimPrefix(id, addr.BundledRelationURLPrefix)), nil
	}

	if strings.HasPrefix(id, addr.RelationKeyToIdPrefix) {
		return RelationKey(strings.TrimPrefix(id, addr.RelationKeyToIdPrefix)), nil
	}

	return "", fmt.Errorf("invalid type url: no prefix found")
}

func FilterRelationKeys(keys []RelationKey, cond func(RelationKey) bool) []RelationKey {
	var res = make([]RelationKey, 0, len(keys))
	for _, key := range keys {
		if cond(key) {
			res = append(res, key)
		}
	}
	return res
}
