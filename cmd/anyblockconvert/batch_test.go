package main

// Options sort on orderId+name concatenated (database.OrderMap.BuildOrder),
// so an option with no order id is compared by *name* against everyone
// else's order id: "Abandoned" would sort ahead of the whole declared
// vocabulary because 'A' < the lexid alphabet, while "Zebra" would sort
// behind it. Every option therefore needs an order id.

import (
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
)

// vocab is a declared vocabulary that names no colors.
func vocab(names ...string) []anyblockjson.OptionDefinition {
	out := make([]anyblockjson.OptionDefinition, 0, len(names))
	for _, n := range names {
		out = append(out, anyblockjson.OptionDefinition{Name: n})
	}
	return out
}

// mintedColors maps option name to the relationOptionColor minted for it,
// for one property.
func mintedColors(t *testing.T, b *batch, key string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, p := range b.pending {
		if p.sbType != model.SmartBlockType_STRelationOption {
			continue
		}
		d := p.snapshot.Details.Fields
		if d[detailRelationKey].GetStringValue() != key {
			continue
		}
		name := d[detailName].GetStringValue()
		out[name] = d[detailRelationOptionColor].GetStringValue()
	}
	return out
}

// mintedOptions returns option names in the order BuildOrder would compare
// them: the orderId and name concatenated.
func mintedOptions(t *testing.T, b *batch) []string {
	t.Helper()
	type opt struct{ sortKey, name string }
	var opts []opt
	for _, p := range b.pending {
		if p.sbType != model.SmartBlockType_STRelationOption {
			continue
		}
		d := p.snapshot.Details.Fields
		name := d[detailName].GetStringValue()
		order := d[detailOrderId].GetStringValue()
		require.NotEmpty(t, order, "option %q has no order id", name)
		opts = append(opts, opt{order + name, name})
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].sortKey < opts[j].sortKey })
	names := make([]string, 0, len(opts))
	for _, o := range opts {
		names = append(names, o.name)
	}
	return names
}

func TestBatch_DeclaredVocabularyKeepsDeclarationOrder(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage": {
			Format:  model.RelationFormat_status,
			Options: vocab("Backlog", "In progress", "In review", "Blocked", "Done"),
		},
	}, nil)
	assert.Equal(t,
		[]string{"Backlog", "In progress", "In review", "Blocked", "Done"},
		mintedOptions(t, b),
		"declaration order, not alphabetical")
}

// a value no vocabulary declares must land after the declared ones whatever
// its name — this is the case that used to get an empty order id
func TestBatch_UndeclaredValueSortsAfterVocabulary(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage": {
			Format:  model.RelationFormat_status,
			Options: vocab("Backlog", "In progress", "Done"),
		},
	}, nil)
	// "Abandoned" sorts first alphabetically and would jump the queue
	b.OptionId(domain.RelationKey("stage"), "Abandoned")
	b.OptionId(domain.RelationKey("stage"), "Zebra")

	assert.Equal(t,
		[]string{"Backlog", "In progress", "Done", "Abandoned", "Zebra"},
		mintedOptions(t, b))
}

// with nothing declared, discovery order still produces real order ids
func TestBatch_UndeclaredOnlyStillGetsOrderIds(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage": {Format: model.RelationFormat_status},
	}, nil)
	b.OptionId(domain.RelationKey("stage"), "Zebra")
	b.OptionId(domain.RelationKey("stage"), "Abandoned")
	assert.Equal(t, []string{"Zebra", "Abandoned"}, mintedOptions(t, b),
		"first seen first, not alphabetical")
}

// order ids are per property, so two selects do not interleave
func TestBatch_OrderIdsArePerProperty(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage":    {Format: model.RelationFormat_status, Options: vocab("A", "B")},
		"priority": {Format: model.RelationFormat_status, Options: vocab("Low", "High")},
	}, nil)
	firsts := map[string]string{}
	for _, p := range b.pending {
		if p.sbType != model.SmartBlockType_STRelationOption {
			continue
		}
		d := p.snapshot.Details.Fields
		key := d[detailRelationKey].GetStringValue()
		if _, seen := firsts[key]; !seen {
			firsts[key] = d[detailOrderId].GetStringValue()
		}
	}
	require.Len(t, firsts, 2)
	assert.Equal(t, firsts["stage"], firsts["priority"],
		"each property starts its own sequence at the same midpoint")
}

func TestBatch_DeclaredColorsReachTheOption(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage": {Format: model.RelationFormat_status, Options: []anyblockjson.OptionDefinition{
			{Name: "Backlog", Color: "grey"},
			{Name: "In progress", Color: "blue"},
			{Name: "Done", Color: "lime"},
		}},
	}, nil)
	assert.Equal(t,
		map[string]string{"Backlog": "grey", "In progress": "blue", "Done": "lime"},
		mintedColors(t, b, "stage"))
}

// A vocabulary that declares no colors still gets distinct ones: the palette
// is cycled in declaration order, so a five-status select does not render as
// five identical chips. Deterministic, unlike constant.RandomOptionColor.
func TestBatch_ColorlessVocabularyCyclesThePalette(t *testing.T) {
	newFixture := func() *batch {
		return newBatch(map[string]anyblockbatch.FormatInfo{
			"tag": {Format: model.RelationFormat_tag,
				Options: vocab("design", "research", "infra", "ops")},
		}, nil)
	}
	want := map[string]string{
		"design": "grey", "research": "yellow", "infra": "orange", "ops": "red",
	}

	assert.Equal(t, want, mintedColors(t, newFixture(), "tag"))
	assert.Equal(t, want, mintedColors(t, newFixture(), "tag"),
		"same input, same colors on every run")
}

// the cycle never hands out a color the vocabulary names explicitly
func TestBatch_ColorCycleSkipsDeclaredColors(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage": {Format: model.RelationFormat_status, Options: []anyblockjson.OptionDefinition{
			{Name: "Backlog"},
			{Name: "In progress", Color: "yellow"}, // second in the palette
			{Name: "Done"},
		}},
	}, nil)
	assert.Equal(t,
		map[string]string{"Backlog": "grey", "In progress": "yellow", "Done": "orange"},
		mintedColors(t, b, "stage"),
		"Done takes orange, not the claimed yellow")
}

// a vocabulary claiming the whole palette leaves the cycle nothing to pick:
// it must still terminate and produce a real color
func TestBatch_ColorCycleTerminatesWhenPaletteFullyClaimed(t *testing.T) {
	declared := make([]anyblockjson.OptionDefinition, 0, len(constant.OptionColors())+1)
	for i, c := range constant.OptionColors() {
		declared = append(declared, anyblockjson.OptionDefinition{
			Name: fmt.Sprintf("claim%d", i), Color: c.String()})
	}
	declared = append(declared, anyblockjson.OptionDefinition{Name: "uncolored"})

	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"tag": {Format: model.RelationFormat_tag, Options: declared},
	}, nil)
	assert.Contains(t, constant.OptionColors(),
		constant.OptionColor(mintedColors(t, b, "tag")["uncolored"]))
}

// a value nobody declared continues its property's cycle rather than
// restarting it, so it does not collide with the vocabulary's first color
func TestBatch_DiscoveredValueContinuesTheColorCycle(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage": {Format: model.RelationFormat_status, Options: vocab("Backlog", "Done")},
	}, nil)
	b.OptionId(domain.RelationKey("stage"), "Abandoned")

	assert.Equal(t,
		map[string]string{"Backlog": "grey", "Done": "yellow", "Abandoned": "orange"},
		mintedColors(t, b, "stage"))
}

// colors are per property: one select's palette position is not advanced by
// another's, the way order ids are already scoped (TestBatch_OrderIdsArePerProperty)
func TestBatch_ColorsArePerProperty(t *testing.T) {
	b := newBatch(map[string]anyblockbatch.FormatInfo{
		"stage":    {Format: model.RelationFormat_status, Options: vocab("A", "B")},
		"priority": {Format: model.RelationFormat_status, Options: vocab("Low", "High")},
	}, nil)
	assert.Equal(t, map[string]string{"A": "grey", "B": "yellow"}, mintedColors(t, b, "stage"))
	assert.Equal(t, map[string]string{"Low": "grey", "High": "yellow"}, mintedColors(t, b, "priority"))
}

// A property's target types are written as ids, split the same way
// PropertyId splits relations: a type this bundle defines is referenced by
// its own document id so the importer relinks it, a bundled type by its
// bundled url — the form recommendedRelations already uses for _br<key>.
func TestBatch_ObjectTypeIdsSplitLocalAndBundled(t *testing.T) {
	b := newBatch(nil, map[string]string{"wikiPerson": "type-person"})
	got := b.objectTypeIds(anyblockjson.PropertyDefinition{
		Key:         "owner",
		ObjectTypes: []string{"wikiPerson", "participant"},
	})
	assert.Equal(t, []string{"type-person", "_otparticipant"}, got)
}

func TestBatch_NoObjectTypesLeavesPropertyUntargeted(t *testing.T) {
	b := newBatch(nil, nil)
	assert.Nil(t, b.objectTypeIds(anyblockjson.PropertyDefinition{Key: "owner"}))
}
