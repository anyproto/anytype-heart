package anyblockjson

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// This file used to pin a refusal: v0.22 made `kind` the sole authority on
// what a template is, and a document that carried the meaning in `type`
// alone — `{"type": "template"}` with no `kind` — was refused rather than
// migrated, because that one shape was well-formed under BOTH readings and
// would otherwise have imported as an ordinary page with nothing anywhere
// saying so. The refusal even read the document's own type_internal_keys
// legend, so a document that renamed the template spelling could not slip
// past it.
//
// The freeze deleted it (§15 #9). Every document written under the old
// reading declares version 1, and checkVersion refuses that outright before
// any semantic check runs — one gate, one verdict, instead of a byte
// comparison standing in for a version marker the format did not have.
//
// What is left is the rule the refusal was standing in front of, and these
// cases pin it: the kind decides, the legend decides only the TYPE, and the
// version gate is what answers for a pre-freeze document.
func TestValidate_ThePreFreezeTemplateShapeIsNoLongerSpecialCased(t *testing.T) {
	t.Run("a kindless document is a page, whatever its type spells", func(t *testing.T) {
		// under the old reading this WAS a template, and the deleted refusal
		// is what said so. At version 2 the kind is absent, so it is a page
		// whose object type is the one the term names — no refusal, and
		// nothing silent about it either: the type is spelled right there
		doc := []byte(`{"version": 2, "type": "template"}`)

		require.NoError(t, Validate(doc))
		sbType, snap, err := Unmarshal(doc, Options{})
		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Page, sbType)
		assert.Equal(t, []string{"ot-template"}, snap.GetObjectTypes())
	})

	t.Run("and so is one whose legend renames the template spelling", func(t *testing.T) {
		// `tpl` resolves to the stored key `template` through the document's
		// own legend. The deleted refusal read that legend; the kind gate
		// does not need to, because it reads a field no chain touches
		doc := []byte(`{"version": 2, "type_internal_keys": {"tpl": "template"}, "type": "tpl"}`)

		require.NoError(t, Validate(doc))
		sbType, snap, err := Unmarshal(doc, Options{})
		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Page, sbType)
		assert.Equal(t, []string{"ot-template"}, snap.GetObjectTypes())
	})

	t.Run("a legend that rebinds the spelling away binds the type it names", func(t *testing.T) {
		// the mirror case, and the one that never changed: the raw spelling
		// IS `template`, but the document's own legend binds it to `custom1`
		doc := []byte(`{"version": 2, "type_internal_keys": {"template": "custom1"}, "type": "template"}`)

		require.NoError(t, Validate(doc))
		sbType, snap, err := Unmarshal(doc, Options{})
		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Page, sbType)
		assert.Equal(t, []string{"ot-custom1"}, snap.GetObjectTypes(),
			"and it binds to the type the legend names")
	})

	t.Run("the version gate is what answers for a pre-freeze document", func(t *testing.T) {
		// the positive control the deleted refusal used to be: a document
		// written under the old reading is still refused, one version marker
		// earlier and for the whole grammar rather than this one shape
		err := Validate([]byte(`{"version": 1, "type": "template"}`))
		require.Error(t, err)
		var ve *ValidationError
		require.ErrorAs(t, err, &ve)
		require.Len(t, ve.Issues, 1)
		assert.Equal(t, "/version", ve.Issues[0].Path)
		assert.Contains(t, ve.Issues[0].Message, "pre-freeze")
		assert.False(t, ve.NewerFormat, "a draft is not a newer format")

		_, _, uerr := Unmarshal([]byte(`{"version": 1, "type": "template"}`), Options{})
		require.Error(t, uerr, "Validate and Unmarshal agree (§11 I2)")
	})

	t.Run("a canonical template still validates", func(t *testing.T) {
		require.NoError(t, Validate([]byte(
			`{"version": 2, "kind": "template", "type": "template", "template_for": "page"}`)))
	})
}
