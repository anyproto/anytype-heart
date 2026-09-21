package v2service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	testTemplateTypeId = "type-template"
	testMemoTypeId     = "type-memo"
)

// addTemplateType registers the space's template type, without which no row
// can BE a template (its `type` names this object).
func (fx *v2Fixture) addTemplateType(t *testing.T) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String(testTemplateTypeId),
		bundle.RelationKeyName:           domain.String("Template"),
		bundle.RelationKeyUniqueKey:      domain.String(bundle.TypeKeyTemplate.URL()),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
	}})
}

// addMemoType registers a custom type `memo`, optionally carrying a
// default_template. The detail is written as a LIST, which is the spelling
// this API's own type writes take.
func (fx *v2Fixture) addMemoType(t *testing.T, defaultTemplate ...string) {
	memo := objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(testMemoTypeId),
		bundle.RelationKeyName:           domain.String("Memo"),
		bundle.RelationKeyUniqueKey:      domain.String("ot-memo"),
		bundle.RelationKeyApiObjectKey:   domain.String("memo"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
	}
	if len(defaultTemplate) > 0 {
		memo[bundle.RelationKeyDefaultTemplateId] = domain.StringList(defaultTemplate)
	}
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{memo})
}

// addTemplate registers one template object of targetTypeId.
func (fx *v2Fixture) addTemplate(t *testing.T, id, name, targetTypeId string, extra ...objectstore.TestObject) {
	row := objectstore.TestObject{
		bundle.RelationKeyId:               domain.String(id),
		bundle.RelationKeyName:             domain.String(name),
		bundle.RelationKeyType:             domain.String(testTemplateTypeId),
		bundle.RelationKeyTargetObjectType: domain.String(targetTypeId),
		bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
	}
	for _, more := range extra {
		for key, value := range more {
			row[key] = value
		}
	}
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{row})
}

func TestCreateObjectTemplate(t *testing.T) {
	t.Run("a template the body names is applied and named back", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")
		want := &v2model.AppliedTemplate{Id: "tpl-weekly", Name: "Weekly memo", Source: "request"}

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"tpl-weekly"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, result.Template)
		assert.Equal(t, "tpl-weekly", *applied, "the create itself must start from the template the result names")
		assert.Empty(t, result.Warnings)
	})

	t.Run("the type's default template applies when the body names none", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")
		want := &v2model.AppliedTemplate{Id: "tpl-weekly", Name: "Weekly memo", Source: "type_default"}

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, result.Template, "the caller did not choose it, so the result has to say who did")
		assert.Equal(t, "tpl-weekly", *applied)
	})

	t.Run("a default stored as a bare string applies too", func(t *testing.T) {
		// given — the spelling the clients write; this API writes a list
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String(testMemoTypeId),
			bundle.RelationKeyName:              domain.String("Memo"),
			bundle.RelationKeyUniqueKey:         domain.String("ot-memo"),
			bundle.RelationKeyApiObjectKey:      domain.String("memo"),
			bundle.RelationKeyResolvedLayout:    domain.Int64(int64(model.ObjectType_objectType)),
			bundle.RelationKeyDefaultTemplateId: domain.String("tpl-weekly"),
		}})
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.Equal(t, "tpl-weekly", result.Template.Id)
		assert.Equal(t, "tpl-weekly", *applied)
	})

	t.Run("none starts from nothing even when the type has a default", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"none"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Template)
		assert.Empty(t, *applied)
	})

	t.Run("an empty template member reads as absent, not as none", func(t *testing.T) {
		// given — a body generated against the schema carries every member it
		// can see; an empty one must not switch the type's default off
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":""}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.Equal(t, "type_default", result.Template.Source)
		assert.Equal(t, "tpl-weekly", *applied)
	})

	t.Run("a type with no default template applies none", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Template)
		assert.Empty(t, *applied)
		assert.Empty(t, result.Warnings)
	})

	t.Run("a stale default warns, and the object is still created", func(t *testing.T) {
		// given — the default outlives its template: deleting one clears the
		// type only when the detail was stored as a bare string
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-gone")
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "newObj", result.Id, "a type nobody repaired must not block every create of that type")
		assert.Nil(t, result.Template)
		assert.Empty(t, *applied)
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "tpl-gone")
		assert.Contains(t, result.Warnings[0].Message, "default template")
		assert.Contains(t, result.Warnings[0].Hint, "default_template")
	})

	t.Run("a deleted default warns like a missing one", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyIsDeleted: domain.Bool(true),
		})
		fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Template)
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "deleted")
	})

	t.Run("a template the body names and the space does not hold is refused", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)

		// when — no create expectation: nothing may be written
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"tpl-nope"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "tpl-nope")
		assert.Contains(t, apiErr.Issues[0].Hint, "none")
	})

	t.Run("a template of another type is refused, and the refusal names that type", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("type-task"),
			bundle.RelationKeyName:           domain.String("Task"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}})
		fx.addTemplate(t, "tpl-task", "Weekly task", "type-task")

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"tpl-task"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "task")
	})

	t.Run("an object that is not a template is refused", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("obj-plain"),
			bundle.RelationKeyName:           domain.String("Just a page"),
			bundle.RelationKeyType:           domain.String(testMemoTypeId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"obj-plain"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, "not a template")
	})

	t.Run("a non-string template member is refused with the member named", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","template":{"id":"tpl-weekly"}}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
	})

	t.Run("a dry run names the template it would apply and creates nothing", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)

		// when — no create expectation: a dry run writes nothing
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), true, false)

		// then
		require.NoError(t, err)
		assert.True(t, result.DryRun)
		require.NotNil(t, result.Template, "a dry run that does not name the template says nothing about the outcome")
		assert.Equal(t, "tpl-weekly", result.Template.Id)
		assert.Equal(t, "type_default", result.Template.Source)
	})

	t.Run("the full document form takes the same member", func(t *testing.T) {
		// given — the member is lifted before the format validation, which
		// would otherwise refuse it as an unknown envelope member
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"memo","template":"tpl-weekly","properties":{"name":"Monday"},"blocks":[{"type":"paragraph","text":"body"}]}`),
			false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.Equal(t, "tpl-weekly", result.Template.Id)
		snapshot := *captured
		require.NotNil(t, snapshot)
		for _, b := range snapshot.Blocks {
			assert.NotContains(t, b.String(), "tpl-weekly", "the directive must not reach the document")
		}
	})

	t.Run("a template document cannot itself start from a template", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","kind":"template","type":"template","template_for":"memo","template":"tpl-weekly","properties":{"name":"New"}}`),
			false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
	})

	t.Run("POST templates refuses the member as an unknown document member", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)

		// when
		_, err := fx.CreateTemplate(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","template_for":"memo","template":"tpl-weekly","properties":{"name":"New"}}`),
			false, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	})
}

func TestListTemplates(t *testing.T) {
	t.Run("templates list with their type and the default marked", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(200),
		})
		fx.addTemplate(t, "tpl-daily", "Daily memo", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(100),
		})
		want := []v2model.TemplateRow{
			{Id: "tpl-weekly", Name: "Weekly memo", TemplateFor: "memo", Default: true},
			{Id: "tpl-daily", Name: "Daily memo", TemplateFor: "memo"},
		}

		// when
		rows, total, hasMore, err := fx.ListTemplates(context.Background(), testSpaceId, "", 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
		assert.Equal(t, 2, total)
		assert.False(t, hasMore)
	})

	t.Run("the type filter narrows to one type's templates", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("type-task"),
			bundle.RelationKeyName:           domain.String("Task"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}})
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		fx.addTemplate(t, "tpl-task", "Weekly task", "type-task")

		// when
		rows, total, _, err := fx.ListTemplates(context.Background(), testSpaceId, "memo", 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "tpl-weekly", rows[0].Id)
		assert.Equal(t, 1, total)
	})

	t.Run("an object that is not a template does not list", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("obj-plain"),
			bundle.RelationKeyName:           domain.String("Just a page"),
			bundle.RelationKeyType:           domain.String(testMemoTypeId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})

		// when
		rows, total, _, err := fx.ListTemplates(context.Background(), testSpaceId, "", 0, 25)

		// then
		require.NoError(t, err)
		assert.Empty(t, rows)
		assert.Equal(t, 0, total)
	})

	t.Run("an unknown type key is refused with the parameter named", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)

		// when
		_, _, _, err := fx.ListTemplates(context.Background(), testSpaceId, "nosuchtype", 0, 25)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "type", apiErr.Issues[0].Path)
	})
}

func TestTemplateSchemaMembers(t *testing.T) {
	t.Run("the shortcut kind declares template", func(t *testing.T) {
		// given
		var schema map[string]any

		// when
		require.NoError(t, json.Unmarshal([]byte(v2SchemaKinds["shortcut"].schema), &schema))

		// then
		props, ok := schema["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "template")
	})

	t.Run("the object kind's narrowed document schema declares template, and the template kind does not", func(t *testing.T) {
		// when
		object := apiV2KindSchema("object")
		template := apiV2KindSchema("template")

		// then
		var objectRoot, templateRoot map[string]any
		require.NoError(t, json.Unmarshal(object, &objectRoot))
		require.NoError(t, json.Unmarshal(template, &templateRoot))
		assert.Contains(t, objectRoot["properties"].(map[string]any), "template")
		assert.NotContains(t, templateRoot["properties"].(map[string]any), "template",
			"a template does not start from a template")
	})
}

func TestCreateObjectTemplateHardening(t *testing.T) {
	t.Run("a refused template mints nothing on the way", func(t *testing.T) {
		// given — an option that would be created, and a template that will
		// not resolve: the refusal has to come first, or the caller's space
		// keeps an option for an object that was never created
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addSelectProperty(t)

		// when — no ObjectCreateRelationOption expectation: minting one fails the test
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"tpl-nope","properties":{"severity":"Brand new"}}`), false, true)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
	})

	t.Run("a template of another type is refused for a type this space has not installed", func(t *testing.T) {
		// given — `task` is bundled, so it resolves as a type key while
		// holding no store row; its id is still derivable, and it is what any
		// template of that type carries as its target
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-memo", "Weekly memo", testMemoTypeId)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"task","name":"Monday","template":"tpl-memo"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "memo")
	})

	t.Run("a template of the derived type applies for a type this space has not installed", func(t *testing.T) {
		// given — the fixture derives type ids as drv-ot-<key>
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addTemplate(t, "tpl-task", "Weekly task", "drv-ot-task")
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"task","name":"Monday","template":"tpl-task"}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.Equal(t, "tpl-task", *applied)
	})

	t.Run("a default that points at another type's template warns and is not applied", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-task")
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("type-task"),
			bundle.RelationKeyName:           domain.String("Task"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}})
		fx.addTemplate(t, "tpl-task", "Weekly task", "type-task")
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Template)
		assert.Empty(t, *applied)
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "tpl-task")
	})

	t.Run("a create with no type takes the page type's default", func(t *testing.T) {
		// given — an absent type defaults to page, and the default template
		// must follow the type the object is actually created as
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String("type-page"),
			bundle.RelationKeyName:              domain.String("Page"),
			bundle.RelationKeyUniqueKey:         domain.String("ot-page"),
			bundle.RelationKeyResolvedLayout:    domain.Int64(int64(model.ObjectType_objectType)),
			bundle.RelationKeyDefaultTemplateId: domain.StringList([]string{"tpl-page"}),
		}})
		fx.addTemplate(t, "tpl-page", "Blank-ish page", "type-page")
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","properties":{"name":"Monday"}}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.Equal(t, "tpl-page", result.Template.Id)
		assert.Equal(t, "tpl-page", *applied)
	})

	t.Run("the full document form passes the template to the create, not only to the result", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		applied := fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"memo","template":"tpl-weekly","properties":{"name":"Monday"},"blocks":[{"type":"paragraph","text":"body"}]}`),
			false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "tpl-weekly", *applied)
		require.NotNil(t, result.Template)
		assert.Equal(t, "tpl-weekly", result.Template.Id)
	})

	t.Run("a template that vanishes mid-request refuses when the caller named it", func(t *testing.T) {
		// given — it passed the index check, then failed to load
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		fx.creatorMock.EXPECT().CreateObjectFromSnapshot(mock.Anything, testSpaceId, mock.Anything, "tpl-weekly").
			Return(apicore.CreateOutcome{}, apicore.ErrTemplateUnavailable)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","template":"tpl-weekly"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/template", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "deleted")
	})

	t.Run("a default that vanishes mid-request warns, and the object is created without it", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		fx.creatorMock.EXPECT().CreateObjectFromSnapshot(mock.Anything, testSpaceId, mock.Anything, "tpl-weekly").
			Return(apicore.CreateOutcome{}, apicore.ErrTemplateUnavailable)
		fx.creatorMock.EXPECT().CreateObjectFromSnapshot(mock.Anything, testSpaceId, mock.Anything, "").
			Return(apicore.CreateOutcome{Id: "newObj"}, nil)
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "newObj", result.Id)
		assert.Nil(t, result.Template, "the result must not name a template the object did not get")
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, "tpl-weekly")
	})

	t.Run("a stored type key beside the api key is refused, not silently preferred", func(t *testing.T) {
		// given — the format lets a document carry type_internal_key, and on
		// import it wins over `type`: every check this endpoint makes, the
		// template included, reads `type`
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)

		// when
		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"memo","type_internal_key":"task","properties":{"name":"X"}}`), false, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/type_internal_key", apiErr.Issues[0].Path)
	})
}

func TestListTemplatesHardening(t *testing.T) {
	t.Run("an uninstalled or hidden template does not list", func(t *testing.T) {
		// given — the create path refuses both, so offering them here would
		// hand the caller ids that cannot be used
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-live", "Weekly memo", testMemoTypeId)
		fx.addTemplate(t, "tpl-uninstalled", "Removed", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		})
		fx.addTemplate(t, "tpl-hidden", "Hidden", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyIsHidden: domain.Bool(true),
		})

		// when
		rows, total, _, err := fx.ListTemplates(context.Background(), testSpaceId, "", 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "tpl-live", rows[0].Id)
		assert.Equal(t, 1, total)
	})

	t.Run("pages carry has_more and a stable total", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.addTemplate(t, "tpl-a", "A", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(300),
		})
		fx.addTemplate(t, "tpl-b", "B", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(200),
		})
		fx.addTemplate(t, "tpl-c", "C", testMemoTypeId, objectstore.TestObject{
			bundle.RelationKeyLastModifiedDate: domain.Int64(100),
		})

		// when
		first, total, hasMore, err := fx.ListTemplates(context.Background(), testSpaceId, "", 0, 2)
		require.NoError(t, err)
		second, secondTotal, secondHasMore, err := fx.ListTemplates(context.Background(), testSpaceId, "", 2, 2)
		require.NoError(t, err)
		beyond, _, _, err := fx.ListTemplates(context.Background(), testSpaceId, "", 3, 2)
		require.NoError(t, err)

		// then
		assert.Equal(t, []string{"tpl-a", "tpl-b"}, []string{first[0].Id, first[1].Id})
		assert.True(t, hasMore)
		assert.Equal(t, 3, total)
		require.Len(t, second, 1)
		assert.Equal(t, "tpl-c", second[0].Id)
		assert.False(t, secondHasMore)
		assert.Equal(t, 3, secondTotal)
		assert.Empty(t, beyond)
	})

	t.Run("the type filter takes every spelling the type routes take", func(t *testing.T) {
		// given — a stored key that is not the served spelling
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("type-custom"),
			bundle.RelationKeyName:           domain.String("Field note"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-6a941a2861fab2a6d6059813"),
			bundle.RelationKeyApiObjectKey:   domain.String("field_note"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}})
		fx.addTemplate(t, "tpl-note", "Weekly note", "type-custom")

		for _, term := range []string{"field_note", "6a941a2861fab2a6d6059813", "Field note"} {
			// when
			rows, _, _, err := fx.ListTemplates(context.Background(), testSpaceId, term, 0, 25)

			// then
			require.NoError(t, err, term)
			require.Len(t, rows, 1, term)
			assert.Equal(t, "tpl-note", rows[0].Id, term)
			assert.Equal(t, "field_note", rows[0].TemplateFor, term, "the row spells the type the way every other response does")
		}
	})
}

func TestCreateObjectTemplateReportsComposition(t *testing.T) {
	t.Run("a request that sent content is told its body holds the template's too", func(t *testing.T) {
		// given — the object's blocks are the template's followed by these,
		// which is the one thing a caller cannot infer from its own request
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		fx.expectCreateWithTemplate("newObj", 4)
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","markdown":"# Notes\n\nfirst"}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.True(t, result.Template.Combined)
		assert.Equal(t, 4, result.Template.BlocksAdded)
	})

	t.Run("a request that sent no content is not told its body was combined", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)
		fx.expectCreateWithTemplate("newObj", 4)
		fx.expectEtagRead("newObj")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday"}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.False(t, result.Template.Combined, "nothing of the caller's went after the template's blocks")
		assert.Equal(t, 4, result.Template.BlocksAdded)
	})

	t.Run("a dry run says the body would be combined and counts nothing", func(t *testing.T) {
		// given — no template is built on a dry run, so the count is unknown;
		// what the request itself carries is not
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t, "tpl-weekly")
		fx.addTemplate(t, "tpl-weekly", "Weekly memo", testMemoTypeId)

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","markdown":"# Notes"}`), true, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Template)
		assert.True(t, result.Template.Combined)
		assert.Zero(t, result.Template.BlocksAdded)
	})

	t.Run("an object created without a template carries no composition at all", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addTemplateType(t)
		fx.addMemoType(t)
		fx.expectCreateWithTemplate("newObj")
		fx.expectEtagRead("newObj")

		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"type":"memo","name":"Monday","markdown":"# Notes"}`), false, false)

		require.NoError(t, err)
		assert.Nil(t, result.Template)
	})
}
