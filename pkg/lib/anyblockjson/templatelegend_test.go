package anyblockjson

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// v0.22 made `kind` the sole authority on what a template is, and refuses a
// pre-v0.22 document that carried the meaning in `type` alone. The refusal
// reads the raw spelling — but the chain it replaced read the document's own
// type_internal_keys legend first, so a document that RENAMED the template spelling
// through its own legend fell through the refusal and imported as an ordinary
// Page, losing the template. Validate said nothing, anyblockvalidate said
// nothing, and CheckTemplateTargets agreed it was not a template.
//
// These can only fail if the gate stops consulting the document's own legend:
// each asserts the VERDICT on a document whose raw spelling and legend-bound
// key disagree, so a gate reading either one alone gets exactly one of the two
// cases wrong.
func TestValidate_TheLegacyTemplateGateReadsTheDocumentsOwnLegend(t *testing.T) {
	t.Run("a legend that renames the template spelling is still a legacy template", func(t *testing.T) {
		// `tpl` resolves to the stored key `template`, so under v0.21 this WAS
		// a template. Without the legend clause it validates clean and imports
		// as a Page.
		doc := []byte(`{"version": 1, "type_internal_keys": {"tpl": "template"}, "type": "tpl"}`)

		err := Validate(doc)
		require.Error(t, err, "a pre-v0.22 template must be refused however it spelled the type")
		assert.Contains(t, err.Error(), "/kind", "and told where the repair goes")

		_, _, uerr := Unmarshal(doc, Options{})
		require.Error(t, uerr, "Validate and Unmarshal agree (§11 I2)")
	})

	t.Run("a legend that rebinds the spelling away was never a template", func(t *testing.T) {
		// the mirror case: the raw spelling IS `template`, but the document's
		// own legend binds it to `custom1`, so v0.21 read it as an ordinary
		// page. Refusing it would prescribe a repair that changes what the
		// document is.
		doc := []byte(`{"version": 1, "type_internal_keys": {"template": "custom1"}, "type": "template"}`)

		require.NoError(t, Validate(doc),
			"the document's own legend already says this is not a template")
		sbType, snap, err := Unmarshal(doc, Options{})
		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Page, sbType)
		assert.Equal(t, []string{"ot-custom1"}, snap.GetObjectTypes(),
			"and it binds to the type the legend names")
	})

	t.Run("the plain pre-v0.22 spelling is still refused", func(t *testing.T) {
		// the positive control: the case the wave-3 refusal was written for
		// must keep failing, so the clause above cannot pass by disabling it
		require.Error(t, Validate([]byte(`{"version": 1, "type": "template"}`)))
	})

	t.Run("a canonical v0.22 template still validates", func(t *testing.T) {
		require.NoError(t, Validate([]byte(
			`{"version": 1, "kind": "template", "type": "template", "template_for": "page"}`)))
	})
}
