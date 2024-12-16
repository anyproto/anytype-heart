package userdataobject

import (
	"github.com/anyproto/any-store/anyenc"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	identityField    = "identity"
	nameField        = "name"
	iconField        = "icon"
	descriptionField = "description"
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
		identity:    string(value.GetStringBytes(identityField)),
		name:        string(value.GetStringBytes(nameField)),
		icon:        string(value.GetStringBytes(iconField)),
		description: string(value.GetStringBytes(descriptionField)),
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
	contact.Set(identityField, arena.NewString(c.identity))
	contact.Set(nameField, arena.NewString(c.name))
	contact.Set(iconField, arena.NewString(c.icon))
	contact.Set(descriptionField, arena.NewString(c.description))
	return contact
}

func ModelToJson(arena *anyenc.Arena, details *types.Struct) *anyenc.Value {
	contact := arena.NewObject()
	contact.Set(identityField, arena.NewString(pbtypes.GetString(details, bundle.RelationKeyIdentity.String())))
	contact.Set(nameField, arena.NewString(pbtypes.GetString(details, bundle.RelationKeyName.String())))
	contact.Set(iconField, arena.NewString(pbtypes.GetString(details, bundle.RelationKeyIconImage.String())))
	contact.Set(descriptionField, arena.NewString(pbtypes.GetString(details, bundle.RelationKeyDescription.String())))
	return contact
}
