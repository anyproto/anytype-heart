package userdataobject

import (
	"github.com/anyproto/any-store/anyenc"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

const keyField = "key"

type Contact struct {
	identity    string
	description string
	key         string
}

func NewContact(identity, key string) *Contact {
	return &Contact{identity: identity, key: key}
}

func NewContactFromJson(value *anyenc.Value) *Contact {
	return &Contact{
		identity:    string(value.GetStringBytes(bundle.RelationKeyIdentity.String())),
		description: string(value.GetStringBytes(bundle.RelationKeyDescription.String())),
		key:         string(value.GetStringBytes(keyField)),
	}
}

func (c *Contact) Details() *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyIdentity:    domain.String(c.identity),
		bundle.RelationKeyDescription: domain.String(c.description),
	})
}

func (c *Contact) ToJson(arena *anyenc.Arena) *anyenc.Value {
	contact := arena.NewObject()
	contact.Set(bundle.RelationKeyIdentity.String(), arena.NewString(c.identity))
	contact.Set(bundle.RelationKeyDescription.String(), arena.NewString(c.description))
	contact.Set(keyField, arena.NewString(c.key))
	return contact
}

func (c *Contact) Description() string {
	return c.description
}

func (c *Contact) Identity() string {
	return c.identity
}

func (c *Contact) Key() string {
	return c.key
}
