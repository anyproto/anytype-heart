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
