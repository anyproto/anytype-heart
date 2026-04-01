package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestGCEligibleLayouts_ContainsFileLayouts(t *testing.T) {
	for _, fl := range domain.FileLayouts {
		assert.Contains(t, domain.GCEligibleLayouts, fl, "FileLayouts entry %v must be in GCEligibleLayouts", fl)
	}
}

func TestGCEligibleLayouts_ExcludesSystemLayouts(t *testing.T) {
	systemLayouts := []model.ObjectTypeLayout{
		model.ObjectType_objectType,
		model.ObjectType_relation,
		model.ObjectType_relationOption,
		model.ObjectType_relationOptionsList,
		model.ObjectType_dashboard,
		model.ObjectType_space,
		model.ObjectType_spaceView,
		model.ObjectType_participant,
		model.ObjectType_date,
		model.ObjectType_chatDerived,
		model.ObjectType_discussion,
	}
	for _, sl := range systemLayouts {
		assert.NotContains(t, domain.GCEligibleLayouts, sl, "system layout %v must not be in GCEligibleLayouts", sl)
	}
}

func TestGCEligibleLayouts_ContainsUserContentLayouts(t *testing.T) {
	userLayouts := []model.ObjectTypeLayout{
		model.ObjectType_basic,
		model.ObjectType_profile,
		model.ObjectType_todo,
		model.ObjectType_set,
		model.ObjectType_note,
		model.ObjectType_bookmark,
		model.ObjectType_collection,
	}
	for _, ul := range userLayouts {
		assert.Contains(t, domain.GCEligibleLayouts, ul, "user layout %v must be in GCEligibleLayouts", ul)
	}
}
