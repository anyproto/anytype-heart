package main

// Options sort on orderId+name concatenated (database.OrderMap.BuildOrder),
// so an option with no order id is compared by *name* against everyone
// else's order id: "Abandoned" would sort ahead of the whole declared
// vocabulary because 'A' < the lexid alphabet, while "Zebra" would sort
// behind it. Every option therefore needs an order id.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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

// The converter half of anyblockbatch's CheckTargetTypes ordering: a type this
// bundle DEFINES wins over the bundled type of the same name, even when its
// document carries no id — and then the target is the empty string, which
// names nothing anywhere. This is not a behaviour to keep, it is the reason
// CheckTargetTypes has to ask typeIds before it asks the bundle; the lint
// rejects such a bundle so this never runs in anger. If this assertion ever
// changes, TestCheckTargetTypes_BundledKeyDefinedLocallyWithoutIdIsReported
// has to change with it.
func TestBatch_IdlessLocalTypeYieldsAnEmptyTargetId(t *testing.T) {
	require.True(t, bundle.HasObjectTypeByKey("page"),
		"the fixture only bites while `page` really is bundled, i.e. while the two arms disagree")
	b := newBatch(nil, map[string]string{"page": ""})
	assert.Equal(t, []string{""}, b.objectTypeIds(anyblockjson.PropertyDefinition{
		Key:         "owner",
		ObjectTypes: []string{"page"},
	}), "the local arm wins and has nothing to offer — not the bundled url")
}

// End to end, over the real seam: a bundle whose property_internal_keys legend backs a
// slug (§3) must mint the property's Relation object and its declared select
// vocabulary under the STORED key, because that is the key the value's detail
// is written under. When the format table was keyed by the spelling, the
// format was never found: the value passed through raw, no Relation was minted
// at all, and the declared options sat on a relation nothing referenced.
func TestBatch_LegendBackedPropertyMintsItsRelationAndOptions(t *testing.T) {
	const storedKey = "6a32d4856761631534b22f85"
	const legend = `"property_internal_keys": {"priority": "` + storedKey + `"},`

	dir := t.TempDir()
	typeDoc := filepath.Join(dir, "task.type.json")
	require.NoError(t, os.WriteFile(typeDoc, []byte(`{"version": 1, "kind": "object_type",
	  "internal_key": "task", "id": "type-task", `+legend+`
	  "type_settings": {"property_definitions": [{"property": "priority", "name": "Priority", "format": "select",
	    "options": ["High", "Low"]}]}}`), 0o644))

	formats, err := anyblockbatch.ScanFormats([]string{typeDoc})
	require.NoError(t, err)
	b := newBatch(formats, map[string]string{"task": "type-task"})

	// types first, exactly as OrderTypesFirst arranges the walk — the type
	// document is what drives PropertyId, and so what mints the Relation
	_, _, _, err = convertFile(dir, typeDoc, b, false, nil)
	require.NoError(t, err)

	_, snap := convertDoc(t, b, "one.json", `{"version": 1, "id": "obj-1", "type": "task", `+legend+`
	  "properties": {"priority": "High"}}`)

	// the value reached the detail under the stored key, as an option id
	optionID := snap.Details.GetFields()[storedKey]
	require.NotNil(t, optionID, "the legend-backed value must land on the stored key")
	got := optionID.GetListValue().GetValues()
	require.Len(t, got, 1, "a resolved select value is an option id list, not the raw string %q",
		optionID.GetStringValue())

	// and that id is the DECLARED option, minted up front by newBatch under
	// the same stored key — not one discovered from the value with no order id
	var relations, options int
	byID := map[string]*model.SmartBlockSnapshotBase{}
	for _, p := range b.pending {
		byID[p.id] = p.snapshot
		switch p.sbType {
		case model.SmartBlockType_STRelation:
			relations++
			assert.Equal(t, storedKey, p.snapshot.Details.Fields[detailRelationKey].GetStringValue())
			assert.Equal(t, "Priority", p.snapshot.Details.Fields[detailName].GetStringValue())
		case model.SmartBlockType_STRelationOption:
			options++
			assert.Equal(t, storedKey, p.snapshot.Details.Fields[detailRelationKey].GetStringValue())
			assert.NotEmpty(t, p.snapshot.Details.Fields[detailOrderId].GetStringValue(),
				"a declared option is pre-minted with an order id; one discovered from a value is not")
		}
	}
	assert.Equal(t, 1, relations, "exactly one Relation object, for the stored key")
	assert.Equal(t, 2, options, "both declared options, and no third discovered under the spelling")

	require.Contains(t, byID, got[0].GetStringValue())
	assert.Equal(t, "High",
		byID[got[0].GetStringValue()].Details.Fields[detailName].GetStringValue())
}
