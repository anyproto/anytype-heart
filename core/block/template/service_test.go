package template

import (
	"testing"

	"go.uber.org/mock/gomock"
)

func TestService_StateFromTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	//templateName := "template"

	//t.Run("empty page name", func(t *testing.T) {
	//	tmpl, err := editor.NewTemplateTest(t, ctrl, templateName)
	//	require.NoError(t, err)
	//
	//	st, err := tmpl.GetNewPageState("")
	//	require.NoError(t, err)
	//	require.Equal(t, st.Details().Fields[bundle.RelationKeyName.String()].GetStringValue(), templateName)
	//	require.Equal(t, st.Get(template.TitleBlockId).Model().GetText().Text, templateName)
	//})
	//
	//t.Run("custom page name", func(t *testing.T) {
	//	tmpl, err := NewTemplateTest(t, ctrl, templateName)
	//	require.NoError(t, err)
	//
	//	customName := "some name"
	//	st, err := tmpl.GetNewPageState(customName)
	//	require.NoError(t, err)
	//	require.Equal(t, st.Details().Fields[bundle.RelationKeyName.String()].GetStringValue(), customName)
	//	require.Equal(t, st.Get(template.TitleBlockId).Model().GetText().Text, "")
	//})
	//
	//t.Run("object type key is set from target type", func(t *testing.T) {
	//	tmpl, err := NewTemplateTest(t, ctrl, templateName)
	//	require.NoError(t, err)
	//
	//	st, err := tmpl.GetNewPageState("")
	//	require.NoError(t, err)
	//	require.Equal(t, st.ObjectTypeKeys(), []domain.TypeKey{bundle.TypeKeyPage})
	//})
}
