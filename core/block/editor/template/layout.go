package template

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func ByLayout(layout model.ObjectTypeLayout, templates ...StateTransformer) []StateTransformer {
	// TODO: not complete, need to describe all layouts
	templates = append(templates,
		WithLayout(layout),
		WithDefaultFeaturedRelations,
		WithFeaturedRelations,
		WithRequiredRelations(),
		WithMaxCountMigration,
	)

	switch layout {
	case model.ObjectType_note:
		templates = append(templates,
			WithNoTitle,
			WithNoDescription,
		)
	case model.ObjectType_todo:
		templates = append(templates,
			WithTitle,
			WithDescription,
			WithRelations([]bundle.RelationKey{bundle.RelationKeyDone}),
		)
	default:
		templates = append(templates,
			WithTitle,
			WithDescription,
		)
	}
	return templates
}
