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
	UserChange ChangeType = iota
	HistoryOperation
	ActiveViewSet
	OrderOperation
	LayoutSync
	CleanupTables
	ObjectInit
	ObjectReinstall
	Indexing
	SystemObjectReviserMigration
)

func (c ChangeType) String() string {
	switch c {
	case UserChange:
		return "UserChange"
	case HistoryOperation:
		return "HistoryOperation"
	case ActiveViewSet:
		return "ActiveViewSet"
	case OrderOperation:
		return "OrderOperation"
	case LayoutSync:
		return "LayoutSync"
	case CleanupTables:
		return "CleanupTables"
	case ObjectInit:
		return "ObjectInit"
	case ObjectReinstall:
		return "ObjectReinstall"
	case Indexing:
		return "Indexing"
	case SystemObjectReviserMigration:
		return "SystemObjectReviserMigration"
	default:
		return "Unknown"
	}
}
