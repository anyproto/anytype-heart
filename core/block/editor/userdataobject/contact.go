package userdataobject

import (
	"github.com/anyproto/any-store/anyenc"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type Contact struct {
	identity    string
	name        string
	icon        string
	description string
}

func NewContact(identity, name, description, icon string) *Contact {
	return &Contact{description: description, icon: icon, name: name, identity: identity}
}

func NewContactFromJson(value *anyenc.Value) *Contact {
	return &Contact{
		identity:    string(value.GetStringBytes(bundle.RelationKeyIdentity.String())),
		name:        string(value.GetStringBytes(bundle.RelationKeyName.String())),
		icon:        string(value.GetStringBytes(bundle.RelationKeyIconImage.String())),
		description: string(value.GetStringBytes(bundle.RelationKeyDescription.String())),
	}
}

func (c *Contact) Details() *types.Struct {
	return &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyIdentity.String():    pbtypes.String(c.identity),
			bundle.RelationKeyDescription.String(): pbtypes.String(c.description),
			bundle.RelationKeyIconImage.String():   pbtypes.String(c.icon),
			bundle.RelationKeyName.String():        pbtypes.String(c.name),
		},
	}
}

func (c *Contact) ToJson(arena *anyenc.Arena) *anyenc.Value {
	contact := arena.NewObject()
	contact.Set(bundle.RelationKeyIdentity.String(), arena.NewString(c.identity))
	contact.Set(bundle.RelationKeyName.String(), arena.NewString(c.name))
	contact.Set(bundle.RelationKeyIconImage.String(), arena.NewString(c.icon))
	contact.Set(bundle.RelationKeyDescription.String(), arena.NewString(c.description))
	return contact
}

func (c *Contact) Description() string {
	return c.description
}

func (c *Contact) Icon() string {
	return c.icon
}

func (c *Contact) Name() string {
	return c.name
}

func (c *Contact) Identity() string {
	return c.identity
}
