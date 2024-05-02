package bundle

import (
	"fmt"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func makeSystemRelationsMap() map[domain.RelationKey]struct{} {
	res := make(map[domain.RelationKey]struct{}, len(SystemRelations))
	for _, k := range SystemRelations {
		res[k] = struct{}{}
	}
	return res
}

var systemRelationsMap = makeSystemRelationsMap()

func IsSystemRelation(relationKey domain.RelationKey) bool {
	_, ok := systemRelationsMap[relationKey]
	return ok
}

func makeInternalTypesTypesMap() map[domain.TypeKey]struct{} {
	res := make(map[domain.TypeKey]struct{}, len(SystemTypes))
	for _, k := range InternalTypes {
		res[k] = struct{}{}
	}
	return res
}

var internalTypesTypesMap = makeInternalTypesTypesMap()

func IsInternalType(typeKey domain.TypeKey) bool {
	_, ok := internalTypesTypesMap[typeKey]
	return ok
}

var DefaultObjectTypePerSmartblockType = map[coresb.SmartBlockType]domain.TypeKey{
	coresb.SmartBlockTypePage:        TypeKeyPage,
	coresb.SmartBlockTypeProfilePage: TypeKeyProfile,
	coresb.SmartBlockTypeHome:        TypeKeyDashboard,
	coresb.SmartBlockTypeTemplate:    TypeKeyTemplate,
	coresb.SmartBlockTypeWidget:      TypeKeyDashboard,
	coresb.SmartBlockTypeObjectType:  TypeKeyObjectType,
	coresb.SmartBlockTypeRelation:    TypeKeyRelation,
	coresb.SmartBlockTypeSpaceView:   TypeKeySpace,
}

// filled in init
var LocalRelationsKeys []string   // stored only in localstore
var DerivedRelationsKeys []string // derived
var LocalAndDerivedRelationKeys []string

var ErrNotFound = fmt.Errorf("not found")

func init() {
	for _, r := range relations {
		if r.DataSource == model.Relation_account || r.DataSource == model.Relation_local {
			LocalRelationsKeys = append(LocalRelationsKeys, r.Key)
		} else if r.DataSource == model.Relation_derived {
			DerivedRelationsKeys = append(DerivedRelationsKeys, r.Key)
		}
	}
	LocalAndDerivedRelationKeys = slices.Clone(DerivedRelationsKeys)
	LocalAndDerivedRelationKeys = append(LocalAndDerivedRelationKeys, LocalRelationsKeys...)
}

func HasObjectTypeID(id string) bool {
	if !strings.HasPrefix(id, TypePrefix) {
		return false
	}
	tk := domain.TypeKey(strings.TrimPrefix(id, TypePrefix))
	_, exists := types[tk]
	return exists
}

// GetTypeByUrl is deprecated, use GetType instead
func GetTypeByUrl(u string) (*model.ObjectType, error) {
	if !strings.HasPrefix(u, TypePrefix) {
		return nil, fmt.Errorf("invalid url with no bundled type prefix")
	}
	tk := domain.TypeKey(strings.TrimPrefix(u, TypePrefix))
	if v, exists := types[tk]; exists {
		t := pbtypes.CopyObjectType(v)
		t.Key = tk.String()
		return t, nil
	}

	return nil, ErrNotFound
}

func GetType(tk domain.TypeKey) (*model.ObjectType, error) {
	if v, exists := types[tk]; exists {
		t := pbtypes.CopyObjectType(v)
		t.Key = tk.String()
		return t, nil
	}

	return nil, ErrNotFound
}

// MustGetType returns built-in object type by predefined TypeKey constant
// PANICS IN CASE RELATION KEY IS NOT EXISTS – DO NOT USE WITH ARBITRARY STRING
func MustGetType(tk domain.TypeKey) *model.ObjectType {
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
func MustGetRelation(rk domain.RelationKey) *model.Relation {
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
func MustGetRelationLink(rk domain.RelationKey) *model.RelationLink {
	if v, exists := relations[rk]; exists {
		return &model.RelationLink{Key: v.Key, Format: v.Format}
	}

	// we can safely panic in case RelationKey is a generated constant
	panic(ErrNotFound)
}

func GetRelation(rk domain.RelationKey) (*model.Relation, error) {
	if v, exists := relations[rk]; exists {
		v := pbtypes.CopyRelation(v)
		v.Id = addr.BundledRelationURLPrefix + v.Key
		return v, nil
	}

	return nil, ErrNotFound
}

// PickRelation returns relation without copy by key, or nil if not found
// you must NEVER modify it without copying
func PickRelation(rk domain.RelationKey) (*model.Relation, error) {
	if v, exists := relations[rk]; exists {
		return v, nil
	}

	return nil, ErrNotFound
}

func GetLayout(lk model.ObjectTypeLayout) (*model.Layout, error) {
	if v, exists := Layouts[lk]; exists {
		return pbtypes.CopyLayout(&v), nil
	}

	return nil, ErrNotFound
}

func ListRelationsUrls() []string {
	var keys []string
	for k, _ := range relations {
		keys = append(keys, addr.BundledRelationURLPrefix+k.String())
	}

	return keys
}

func HasRelation(key string) bool {
	_, exists := relations[domain.RelationKey(key)]

	return exists
}

func HasObjectTypeByKey(key domain.TypeKey) bool {
	_, exists := types[key]

	return exists
}

func ListTypesKeys() []domain.TypeKey {
	var keys []domain.TypeKey
	for k, _ := range types {
		keys = append(keys, k)
	}

	return keys
}

func TypeKeyFromUrl(url string) (domain.TypeKey, error) {
	if strings.HasPrefix(url, addr.BundledObjectTypeURLPrefix) {
		return domain.TypeKey(strings.TrimPrefix(url, addr.BundledObjectTypeURLPrefix)), nil
	}

	if strings.HasPrefix(url, addr.ObjectTypeKeyToIdPrefix) {
		return domain.TypeKey(strings.TrimPrefix(url, addr.ObjectTypeKeyToIdPrefix)), nil
	}

	return "", fmt.Errorf("invalid type url: no prefix found")
}

func RelationKeyFromID(id string) (domain.RelationKey, error) {
	if strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return domain.RelationKey(strings.TrimPrefix(id, addr.BundledRelationURLPrefix)), nil
	}

	if strings.HasPrefix(id, addr.RelationKeyToIdPrefix) {
		return domain.RelationKey(strings.TrimPrefix(id, addr.RelationKeyToIdPrefix)), nil
	}

	return "", fmt.Errorf("invalid type url: no prefix found")
}
