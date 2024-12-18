package userdataobject

import (
	"github.com/anyproto/any-store/anyenc"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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

func (c *Contact) Details() *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyIdentity:    domain.String(c.identity),
		bundle.RelationKeyDescription: domain.String(c.description),
		bundle.RelationKeyIconImage:   domain.String(c.icon),
		bundle.RelationKeyName:        domain.String(c.name),
	})
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
