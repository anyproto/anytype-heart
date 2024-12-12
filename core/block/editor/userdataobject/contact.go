package userdataobject

import (
	"github.com/anyproto/any-store/anyenc"
)

type Contact struct {
	identity    string
	name        string
	icon        string
	description string
}

func NewJsonContact(name, identity, description, icon string) *Contact {
	return &Contact{description: description, icon: icon, name: name, identity: identity}
}

func (c *Contact) ToJson(arena *anyenc.Arena) *anyenc.Value {
	contact := arena.NewObject()
	contact.Set("identity", arena.NewString(c.identity))
	contact.Set("name", arena.NewString(c.name))
	contact.Set("icon", arena.NewString(c.icon))
	contact.Set("description", arena.NewString(c.description))
	return contact
}
