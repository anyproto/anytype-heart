package dependencies

import "github.com/anyproto/anytype-heart/space/clientspace"

type BuiltinTemplateService interface {
	RegisterBuiltinTemplates(space clientspace.Space) error
}
