package anyblockjson

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §7a lifts a transparent container everywhere a run exists. A table cell is a
// POSITION, not a run — there is nowhere to lift to — so a cell spelled as a
// container is the one shape the format cannot read back, and both sides must
// refuse it.
//
// A cell has two spellings (§6.1): the object form and the array form. The
// array form reached the refusal through checkFlatRun's `inCell` branch; the
// object form went to walkBlock, which had no such check, so Validate accepted
// a document Unmarshal hard-refused — an I2 breach, and a regression: before
// §7a both sides accepted it and minted a Layout_Div cell.
//
// This can only fail if one side stops refusing: the test asserts the two
// VERDICTS agree, not that either one is an error, so a rule that stopped
// firing on both sides at once would still have to keep them equal — and the
// separate "both refuse" assertion catches that.
func TestValidate_ACellCannotBeATransparentContainer_BothSpellings(t *testing.T) {
	for _, attrs := range []string{
		``,
		`, "background_color": "red"`,
		`, "align": "center"`,
		`, "vertical_align": "middle"`,
	} {
		cells := map[string]string{
			"object form": fmt.Sprintf(`{"type": "group"%s}`, attrs),
			"array form":  fmt.Sprintf(`[{"type": "group"%s}]`, attrs),
		}
		for form, cell := range cells {
			t.Run(form+attrs, func(t *testing.T) {
				doc := []byte(fmt.Sprintf(
					`{"version": 2, "type": "page", "blocks": [{"type": "table",
					   "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": [%s]}]}]}`, cell))

				verr := Validate(doc)
				_, _, uerr := Unmarshal(doc, Options{})

				require.Error(t, verr, "a cell cannot be a transparent container (§7a)")
				require.Error(t, uerr, "…and import refuses it too")
				assert.Equal(t, verr != nil, uerr != nil,
					"Validate and Unmarshal must agree (§11 I2)")
			})
		}
	}
}

// the control: an ordinary cell block in BOTH spellings still passes, so the
// refusal above cannot pass by rejecting every cell.
func TestValidate_AnOrdinaryCellStillPasses_BothSpellings(t *testing.T) {
	for form, cell := range map[string]string{
		"object form": `{"type": "paragraph", "text": "x"}`,
		"array form":  `[{"type": "paragraph", "text": "x"}]`,
	} {
		t.Run(form, func(t *testing.T) {
			doc := []byte(fmt.Sprintf(
				`{"version": 2, "type": "page", "blocks": [{"type": "table",
				   "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": [%s]}]}]}`, cell))
			require.NoError(t, Validate(doc))
			_, _, err := Unmarshal(doc, Options{})
			require.NoError(t, err)
		})
	}
}
