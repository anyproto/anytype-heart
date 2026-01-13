package domain

import "github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"

type RelationKey string

func (rk RelationKey) String() string {
	return string(rk)
}
func (rk RelationKey) URL() string {
	return string(addr.RelationKeyToIdPrefix + rk)
}
func (rk RelationKey) BundledURL() string {
	return string(addr.BundledRelationURLPrefix + rk)
}

type TypeKey string

func (tk TypeKey) String() string {
	return string(tk)
}
func (tk TypeKey) URL() string {
	return string(addr.ObjectTypeKeyToIdPrefix + tk)
}
func (tk TypeKey) BundledURL() string {
	return string(addr.BundledObjectTypeURLPrefix + tk)
}

func MarshalTypeKeys(typeKeys []TypeKey) []string {
	res := make([]string, 0, len(typeKeys))
	for _, tk := range typeKeys {
		res = append(res, tk.URL())
	}
	return res
}

type ChangeType uint32

const (
	ChangeTypeUserChange ChangeType = iota
	ChangeTypeHistoryOperation
	ChangeTypeActiveViewSet
	ChangeTypeOrderOperation
	ChangeTypeLayoutSync
	ChangeTypeCleanupTables
	ChangeTypeObjectInit
	ChangeTypeObjectReinstall
	ChangeTypeIndexing
	ChangeTypeSystemObjectReviserMigration
)

func (c ChangeType) String() string {
	switch c {
	case ChangeTypeUserChange:
		return "UserChange"
	case ChangeTypeHistoryOperation:
		return "HistoryOperation"
	case ChangeTypeActiveViewSet:
		return "ActiveViewSet"
	case ChangeTypeOrderOperation:
		return "OrderOperation"
	case ChangeTypeLayoutSync:
		return "LayoutSync"
	case ChangeTypeCleanupTables:
		return "CleanupTables"
	case ChangeTypeObjectInit:
		return "ObjectInit"
	case ChangeTypeObjectReinstall:
		return "ObjectReinstall"
	case ChangeTypeIndexing:
		return "Indexing"
	case ChangeTypeSystemObjectReviserMigration:
		return "SystemObjectReviserMigration"
	default:
		return "Unknown"
	}
}

func (c ChangeType) Raw() uint32 {
	return uint32(c)
}
